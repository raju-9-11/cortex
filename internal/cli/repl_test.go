package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cortex/internal/inference"
	"cortex/internal/session"
	"cortex/internal/store"
)

// testSetup creates an in-memory SQLite store, session manager, and a provider
// registry with a mock provider. Returns all dependencies and a cleanup func.
func testSetup(t *testing.T) (*inference.ProviderRegistry, session.Manager, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	sessionMgr := session.NewManager(db, "mock-model")

	registry := inference.NewProviderRegistry()
	mock := inference.NewMockProvider("test-provider", []string{"Hello", " from", " mock", "!"})
	registry.Register(mock)
	registry.SetDefault("test-provider")

	cleanup := func() {
		db.Close()
	}

	return registry, sessionMgr, cleanup
}

func TestRunREPL_BasicExchange(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Send "hello" followed by blank line, then /exit.
	input := "hello\n\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()

	// Should contain the welcome banner.
	if !strings.Contains(output, "Cortex Chat") {
		t.Error("output missing welcome banner")
	}

	// Should contain the mock response.
	if !strings.Contains(output, "Hello from mock!") {
		t.Errorf("output missing mock response, got:\n%s", output)
	}

	// Should contain the goodbye message.
	if !strings.Contains(output, "Goodbye!") {
		t.Error("output missing goodbye message")
	}
}

func TestRunREPL_SlashExit(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("output missing goodbye message on /exit")
	}
}

func TestRunREPL_SlashQuit(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/quit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("output missing goodbye message on /quit")
	}
}

func TestRunREPL_SlashHelp(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/help\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Available commands") {
		t.Error("output missing help text")
	}
	if !strings.Contains(output, "/exit") {
		t.Error("help text missing /exit command")
	}
	if !strings.Contains(output, "/model") {
		t.Error("help text missing /model command")
	}
}

func TestRunREPL_MultiTurn(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Two messages followed by /exit.
	input := "first message\n\nsecond message\n\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()

	// The mock response should appear twice (once per turn).
	count := strings.Count(output, "Hello from mock!")
	if count != 2 {
		t.Errorf("expected 2 mock responses, got %d; output:\n%s", count, output)
	}
}

func TestRunREPL_EmptyInput(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Empty input (just blank line) should re-prompt, then exit.
	input := "\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()

	// Should NOT contain the mock response (no message sent).
	if strings.Contains(output, "Hello from mock!") {
		t.Error("empty input should not trigger provider call")
	}

	// Should have prompted at least twice (once for empty, once for /exit).
	promptCount := strings.Count(output, "cortex [mock-model]>")
	if promptCount < 2 {
		t.Errorf("expected at least 2 prompts, got %d", promptCount)
	}
}

func TestRunREPL_EOF(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Empty reader → EOF immediately.
	in := strings.NewReader("")
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("output missing goodbye message on EOF")
	}
}

func TestRunREPL_SlashModel(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/model new-model\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Model switched to: new-model") {
		t.Errorf("expected model switch confirmation, got:\n%s", output)
	}

	// The prompt after model switch should show the new model.
	if !strings.Contains(output, "cortex [new-model]>") {
		t.Errorf("expected prompt to show new model, got:\n%s", output)
	}
}

func TestRunREPL_SlashNew(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/new\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "New session created:") {
		t.Errorf("expected new session message, got:\n%s", output)
	}
}

func TestRunREPL_SlashSessions(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Create a session first so /sessions has something to list.
	_, err := sessionMgr.Create(context.Background(), session.CreateParams{
		Model:  "mock-model",
		Title:  "Test Session",
		UserID: "default",
	})
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	input := "/sessions\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err = RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	// Should show both the pre-created session and the REPL's own session.
	if !strings.Contains(output, "Test Session") {
		t.Errorf("expected session listing to contain 'Test Session', got:\n%s", output)
	}
}

func TestRunREPL_SlashLoad(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Create a session to load later.
	sess, err := sessionMgr.Create(context.Background(), session.CreateParams{
		Model:  "mock-model",
		Title:  "Loadable Session",
		UserID: "default",
	})
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Add a message to the loadable session.
	err = sessionMgr.AddMessage(context.Background(), sess.ID, &store.Message{
		Role:    "user",
		Content: "previous question",
	})
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	input := "/load " + sess.ID + "\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err = RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Switched to session "+sess.ID) {
		t.Errorf("expected session switch message, got:\n%s", output)
	}
	if !strings.Contains(output, "previous question") {
		t.Errorf("expected loaded session history, got:\n%s", output)
	}
}

func TestRunREPL_ResumeSession(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Create a session to resume.
	sess, err := sessionMgr.Create(context.Background(), session.CreateParams{
		Model:  "mock-model",
		Title:  "Resume Session",
		UserID: "default",
	})
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Add messages.
	err = sessionMgr.AddMessage(context.Background(), sess.ID, &store.Message{
		Role:    "user",
		Content: "hello from history",
	})
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	input := "/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err = RunREPL(context.Background(), registry, sessionMgr, "mock-model", sess.ID, "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Resuming session") {
		t.Errorf("expected resume header, got:\n%s", output)
	}
	if !strings.Contains(output, "hello from history") {
		t.Errorf("expected history message, got:\n%s", output)
	}
}

func TestRunREPL_MultilineInput(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Multiline input: two non-blank lines followed by blank line.
	input := "line one\nline two\n\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	// Verify the mock was called (response present).
	output := out.String()
	if !strings.Contains(output, "Hello from mock!") {
		t.Errorf("expected mock response for multiline input, got:\n%s", output)
	}
}

func TestRunREPL_UnknownSlashCommand(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/unknown\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Unknown command: /unknown") {
		t.Errorf("expected unknown command message, got:\n%s", output)
	}
}

func TestRunREPL_SlashModelNoArg(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/model\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Current model: mock-model") {
		t.Errorf("expected current model info, got:\n%s", output)
	}
}

func TestRunREPL_SlashLoadNoArg(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	input := "/load\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Usage: /load <session-id>") {
		t.Errorf("expected usage hint for /load, got:\n%s", output)
	}
}

func TestRunREPL_MessagesPersisted(t *testing.T) {
	registry, sessionMgr, cleanup := testSetup(t)
	defer cleanup()

	// Send a message, then exit.
	input := "test persistence\n\n/exit\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunREPL(context.Background(), registry, sessionMgr, "mock-model", "", "", in, &out)
	if err != nil {
		t.Fatalf("RunREPL returned error: %v", err)
	}

	// Find the session ID from output.
	output := out.String()
	idx := strings.Index(output, "Session: ")
	if idx < 0 {
		t.Fatal("could not find session ID in output")
	}
	sessLine := output[idx+len("Session: "):]
	sessID := strings.TrimSpace(sessLine[:strings.Index(sessLine, "\n")])

	// Verify messages were persisted.
	msgs, err := sessionMgr.GetMessages(context.Background(), sessID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	// Should have user message + assistant message.
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	foundUser := false
	foundAssistant := false
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "test persistence" {
			foundUser = true
		}
		if m.Role == "assistant" && strings.Contains(m.Content, "Hello from mock!") {
			foundAssistant = true
		}
	}

	if !foundUser {
		t.Error("user message not persisted")
	}
	if !foundAssistant {
		t.Error("assistant message not persisted")
	}
}
