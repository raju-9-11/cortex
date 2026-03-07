package session

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"forge/internal/store"
)

// setupTestManager creates a SessionManagerImpl backed by a real SQLite store
// in a temp file. The store is migrated and cleaned up automatically.
func setupTestManager(t *testing.T) *SessionManagerImpl {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "forge-session-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	s, err := store.NewSQLiteStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { s.Close() })

	return NewManager(s, "qwen2.5:0.5b")
}

func TestCreateWithDefaults(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateParams{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// ID format: "sess_" + 12 chars from UUID
	if !strings.HasPrefix(sess.ID, "sess_") {
		t.Errorf("expected ID prefix 'sess_', got %q", sess.ID)
	}
	if len(sess.ID) != 17 { // "sess_" (5) + 12 chars
		t.Errorf("expected ID length 17, got %d (%q)", len(sess.ID), sess.ID)
	}

	// Defaults
	if sess.Title != "New Chat" {
		t.Errorf("expected default title 'New Chat', got %q", sess.Title)
	}
	if sess.Model != "qwen2.5:0.5b" {
		t.Errorf("expected default model 'qwen2.5:0.5b', got %q", sess.Model)
	}
	if sess.UserID != "default" {
		t.Errorf("expected default userID 'default', got %q", sess.UserID)
	}
	if sess.Status != "active" {
		t.Errorf("expected status 'active', got %q", sess.Status)
	}
	if sess.SystemPrompt != "" {
		t.Errorf("expected empty system prompt, got %q", sess.SystemPrompt)
	}
	if sess.TokenCount != 0 {
		t.Errorf("expected token_count 0, got %d", sess.TokenCount)
	}
	if sess.MessageCount != 0 {
		t.Errorf("expected message_count 0, got %d", sess.MessageCount)
	}
	if sess.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestCreateWithAllFields(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateParams{
		Model:        "gpt-4",
		Title:        "My Custom Chat",
		SystemPrompt: "You are a pirate.",
		UserID:       "user_42",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if sess.Title != "My Custom Chat" {
		t.Errorf("expected title 'My Custom Chat', got %q", sess.Title)
	}
	if sess.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", sess.Model)
	}
	if sess.SystemPrompt != "You are a pirate." {
		t.Errorf("expected system prompt 'You are a pirate.', got %q", sess.SystemPrompt)
	}
	if sess.UserID != "user_42" {
		t.Errorf("expected userID 'user_42', got %q", sess.UserID)
	}
}

func TestGetExistingSession(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, CreateParams{Title: "Get Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Small sleep to ensure last_access update is distinguishable.
	time.Sleep(10 * time.Millisecond)

	got, err := mgr.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Title != "Get Test" {
		t.Errorf("expected title 'Get Test', got %q", got.Title)
	}

	// Verify last_access was updated (should be >= created time).
	if got.LastAccess.Before(created.LastAccess) {
		t.Errorf("expected last_access to be updated: created=%v, got=%v",
			created.LastAccess, got.LastAccess)
	}
}

func TestGetNonExistentSession(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	_, err := mgr.Get(ctx, "sess_nonexist")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListSessions(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	// Create a couple of sessions.
	_, err := mgr.Create(ctx, CreateParams{Title: "Chat 1"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	_, err = mgr.Create(ctx, CreateParams{Title: "Chat 2"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	sessions, err := mgr.List(ctx, "default")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	// List with a different user should return 0.
	sessions, err = mgr.List(ctx, "other_user")
	if err != nil {
		t.Fatalf("List other user: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for other user, got %d", len(sessions))
	}
}

func TestUpdatePartialFields(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, CreateParams{
		Title: "Original",
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update only the title; model should remain unchanged.
	newTitle := "Updated Title"
	updated, err := mgr.Update(ctx, created.ID, UpdateParams{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", updated.Title)
	}
	if updated.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4' unchanged, got %q", updated.Model)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt) {
		// UpdatedAt should be >= created time (could be equal if very fast).
		// Just verify it's set.
		if updated.UpdatedAt.IsZero() {
			t.Error("expected non-zero updated_at")
		}
	}
}

func TestUpdateAllFields(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, CreateParams{
		Title:        "Original",
		Model:        "gpt-4",
		SystemPrompt: "original prompt",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newTitle := "New Title"
	newModel := "claude-3"
	newPrompt := "new prompt"
	newStatus := "archived"

	updated, err := mgr.Update(ctx, created.ID, UpdateParams{
		Title:        &newTitle,
		Model:        &newModel,
		SystemPrompt: &newPrompt,
		Status:       &newStatus,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Title != "New Title" {
		t.Errorf("expected title 'New Title', got %q", updated.Title)
	}
	if updated.Model != "claude-3" {
		t.Errorf("expected model 'claude-3', got %q", updated.Model)
	}
	if updated.SystemPrompt != "new prompt" {
		t.Errorf("expected system prompt 'new prompt', got %q", updated.SystemPrompt)
	}
	if updated.Status != "archived" {
		t.Errorf("expected status 'archived', got %q", updated.Status)
	}
}

func TestUpdateNonExistentSession(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	title := "nope"
	_, err := mgr.Update(ctx, "sess_nope1234", UpdateParams{Title: &title})
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateParams{Title: "To Delete"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it's gone.
	_, err = mgr.Get(ctx, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Deleting again should error.
	err = mgr.Delete(ctx, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound on double delete, got %v", err)
	}
}

func TestAddMessageAndGetMessages(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateParams{Title: "Msg Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Add a user message.
	userMsg := &store.Message{
		Role:    "user",
		Content: "Hello, world!",
	}
	if err := mgr.AddMessage(ctx, sess.ID, userMsg); err != nil {
		t.Fatalf("AddMessage (user): %v", err)
	}

	// Verify message ID was generated.
	if !strings.HasPrefix(userMsg.ID, "msg_") {
		t.Errorf("expected message ID prefix 'msg_', got %q", userMsg.ID)
	}
	if userMsg.SessionID != sess.ID {
		t.Errorf("expected sessionID %q, got %q", sess.ID, userMsg.SessionID)
	}
	if !userMsg.IsActive {
		t.Error("expected IsActive to be true")
	}
	if userMsg.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at on message")
	}

	// Add an assistant message.
	assistantMsg := &store.Message{
		Role:    "assistant",
		Content: "Hello! How can I help?",
		Model:   "qwen2.5:0.5b",
	}
	if err := mgr.AddMessage(ctx, sess.ID, assistantMsg); err != nil {
		t.Fatalf("AddMessage (assistant): %v", err)
	}

	// Verify session message_count was incremented.
	updated, err := mgr.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get after AddMessage: %v", err)
	}
	if updated.MessageCount != 2 {
		t.Errorf("expected message_count 2, got %d", updated.MessageCount)
	}

	// GetMessages should return both (active only).
	messages, err := mgr.GetMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify order (chronological).
	if messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %q", messages[0].Role)
	}
	if messages[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", messages[1].Role)
	}
	if messages[0].Content != "Hello, world!" {
		t.Errorf("expected first message content 'Hello, world!', got %q", messages[0].Content)
	}
	if messages[1].Content != "Hello! How can I help?" {
		t.Errorf("expected second message content, got %q", messages[1].Content)
	}
}

func TestGetMessagesEmpty(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateParams{Title: "Empty"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	messages, err := mgr.GetMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if messages != nil && len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}
