package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"forge/internal/session"
	"forge/internal/store"
)

// setupTestSessionMgr creates a SessionManagerImpl backed by a real SQLite store
// in a temp directory. The store is migrated and cleaned up automatically.
func setupTestSessionMgr(t *testing.T) session.Manager {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "forge-cli-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	db, err := store.NewSQLiteStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return session.NewManager(db, "test-model")
}

func TestRunSessions_ListEmpty(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	var buf bytes.Buffer

	err := RunSessions(context.Background(), mgr, "list", "", 0, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "No sessions found.") {
		t.Errorf("expected 'No sessions found.' in output, got %q", buf.String())
	}
}

func TestRunSessions_ListWithSessions(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	// Create 3 sessions
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		sess, err := mgr.Create(ctx, session.CreateParams{
			Title: "Test Chat",
		})
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
		ids[i] = sess.ID
	}

	var buf bytes.Buffer
	err := RunSessions(ctx, mgr, "", "", 0, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify header
	if !strings.Contains(output, "ID") || !strings.Contains(output, "TITLE") {
		t.Errorf("expected table header in output, got %q", output)
	}

	// Verify all session IDs appear
	for _, id := range ids {
		if !strings.Contains(output, id) {
			t.Errorf("expected session ID %q in output", id)
		}
	}

	// Verify count
	if !strings.Contains(output, "3 session(s)") {
		t.Errorf("expected '3 session(s)' in output, got %q", output)
	}
}

func TestRunSessions_ListWithLimit(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		_, err := mgr.Create(ctx, session.CreateParams{
			Title: "Limit Test",
		})
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	var buf bytes.Buffer
	err := RunSessions(ctx, mgr, "list", "", 2, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Should show "2 session(s)" not "5 session(s)"
	if !strings.Contains(output, "2 session(s)") {
		t.Errorf("expected '2 session(s)' in output, got %q", output)
	}
}

func TestRunSessions_ListJSON(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	// Create 2 sessions
	for i := 0; i < 2; i++ {
		_, err := mgr.Create(ctx, session.CreateParams{
			Title: "JSON Test",
		})
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	var buf bytes.Buffer
	err := RunSessions(ctx, mgr, "list", "", 0, true, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify valid JSON
	var sessions []store.Session
	if err := json.Unmarshal(buf.Bytes(), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions in JSON, got %d", len(sessions))
	}
}

func TestRunSessions_ShowSession(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, session.CreateParams{
		Title: "Show Test",
		Model: "test-model",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Add messages
	err = mgr.AddMessage(ctx, sess.ID, &store.Message{
		Role:    "user",
		Content: "Hello there",
	})
	if err != nil {
		t.Fatalf("add user message: %v", err)
	}

	err = mgr.AddMessage(ctx, sess.ID, &store.Message{
		Role:    "assistant",
		Content: "Hi! How can I help?",
	})
	if err != nil {
		t.Fatalf("add assistant message: %v", err)
	}

	var buf bytes.Buffer
	err = RunSessions(ctx, mgr, "show", sess.ID, 0, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Check session details
	if !strings.Contains(output, sess.ID) {
		t.Errorf("expected session ID in output")
	}
	if !strings.Contains(output, "Show Test") {
		t.Errorf("expected title in output")
	}
	if !strings.Contains(output, "test-model") {
		t.Errorf("expected model in output")
	}

	// Check messages
	if !strings.Contains(output, "[user] Hello there") {
		t.Errorf("expected user message in output, got %q", output)
	}
	if !strings.Contains(output, "[assistant] Hi! How can I help?") {
		t.Errorf("expected assistant message in output, got %q", output)
	}
}

func TestRunSessions_ShowSessionJSON(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, session.CreateParams{
		Title: "JSON Show Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = mgr.AddMessage(ctx, sess.ID, &store.Message{
		Role:    "user",
		Content: "Test message",
	})
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	var buf bytes.Buffer
	err = RunSessions(ctx, mgr, "show", sess.ID, 0, true, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify valid JSON with session and messages keys
	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}

	if _, ok := result["session"]; !ok {
		t.Error("expected 'session' key in JSON output")
	}
	if _, ok := result["messages"]; !ok {
		t.Error("expected 'messages' key in JSON output")
	}
}

func TestRunSessions_ShowNotFound(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	var buf bytes.Buffer

	err := RunSessions(context.Background(), mgr, "show", "sess_nonexist", 0, false, &buf)
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestRunSessions_DeleteSession(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, session.CreateParams{
		Title: "To Delete",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var buf bytes.Buffer
	err = RunSessions(ctx, mgr, "delete", sess.ID, 0, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", buf.String())
	}

	// Verify actually deleted by trying to get it
	_, err = mgr.Get(ctx, sess.ID)
	if err == nil {
		t.Error("expected error when getting deleted session")
	}
}

func TestRunSessions_DeleteNotFound(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	var buf bytes.Buffer

	err := RunSessions(context.Background(), mgr, "delete", "sess_nonexist", 0, false, &buf)
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestRunSessions_UnknownAction(t *testing.T) {
	mgr := setupTestSessionMgr(t)
	var buf bytes.Buffer

	err := RunSessions(context.Background(), mgr, "foo", "", 0, false, &buf)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}

	if !strings.Contains(err.Error(), "unknown sessions action") {
		t.Errorf("expected 'unknown sessions action' in error, got %q", err.Error())
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
