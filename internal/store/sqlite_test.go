package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "forge-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	s, err := NewSQLiteStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrate(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Running Migrate again should be idempotent
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}
}

func TestPing(t *testing.T) {
	s := setupTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestSessionCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	sess := &Session{
		ID:           "ses_test1",
		UserID:       "default",
		Title:        "Test Chat",
		Model:        "qwen2.5:0.5b",
		SystemPrompt: "You are helpful.",
		Status:       "active",
		TokenCount:   0,
		MessageCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastAccess:   now,
	}

	// Create
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Get
	got, err := s.GetSession(ctx, "ses_test1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Title != "Test Chat" {
		t.Errorf("expected title 'Test Chat', got '%s'", got.Title)
	}
	if got.Model != "qwen2.5:0.5b" {
		t.Errorf("expected model 'qwen2.5:0.5b', got '%s'", got.Model)
	}

	// Get not found
	_, err = s.GetSession(ctx, "ses_nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Update
	got.Title = "Updated Chat"
	got.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	if err := s.UpdateSession(ctx, got); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	updated, _ := s.GetSession(ctx, "ses_test1")
	if updated.Title != "Updated Chat" {
		t.Errorf("expected updated title, got '%s'", updated.Title)
	}

	// List
	sessions, err := s.ListSessions(ctx, SessionListParams{UserID: "default"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// List with status filter
	sessions, err = s.ListSessions(ctx, SessionListParams{UserID: "default", Status: "archived"})
	if err != nil {
		t.Fatalf("ListSessions (archived): %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 archived sessions, got %d", len(sessions))
	}

	// Delete
	if err := s.DeleteSession(ctx, "ses_test1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = s.GetSession(ctx, "ses_test1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMessageCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Need a session first
	sess := &Session{
		ID: "ses_msg_test", UserID: "default", Title: "Msg Test",
		Model: "test-model", Status: "active",
		CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	msg1 := &Message{
		ID: "msg_1", SessionID: "ses_msg_test", Role: "user",
		Content: "Hello", TokenCount: 5, IsActive: true,
		CreatedAt: now,
	}
	msg2 := &Message{
		ID: "msg_2", SessionID: "ses_msg_test", Role: "assistant",
		Content: "Hi there!", TokenCount: 8, IsActive: true,
		Model: "test-model",
		CreatedAt: now.Add(time.Second),
	}

	// Create
	if err := s.CreateMessage(ctx, msg1); err != nil {
		t.Fatalf("CreateMessage msg1: %v", err)
	}
	if err := s.CreateMessage(ctx, msg2); err != nil {
		t.Fatalf("CreateMessage msg2: %v", err)
	}

	// Get
	got, err := s.GetMessage(ctx, "msg_1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Content != "Hello" {
		t.Errorf("expected content 'Hello', got '%s'", got.Content)
	}
	if !got.IsActive {
		t.Error("expected IsActive to be true")
	}

	// Get not found
	_, err = s.GetMessage(ctx, "msg_nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// List (chronological)
	messages, err := s.ListMessages(ctx, MessageListParams{SessionID: "ses_msg_test"})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].ID != "msg_1" || messages[1].ID != "msg_2" {
		t.Errorf("messages not in chronological order: %s, %s", messages[0].ID, messages[1].ID)
	}

	// Deactivate
	if err := s.DeactivateMessages(ctx, "ses_msg_test", []string{"msg_1"}); err != nil {
		t.Fatalf("DeactivateMessages: %v", err)
	}

	// List active only
	active, err := s.ListMessages(ctx, MessageListParams{SessionID: "ses_msg_test", ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListMessages (active): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active message, got %d", len(active))
	}

	// Deactivate empty list (should be no-op)
	if err := s.DeactivateMessages(ctx, "ses_msg_test", []string{}); err != nil {
		t.Fatalf("DeactivateMessages (empty): %v", err)
	}

	// Delete session should cascade
	if err := s.DeleteSession(ctx, "ses_msg_test"); err != nil {
		t.Fatal(err)
	}
	msgs, _ := s.ListMessages(ctx, MessageListParams{SessionID: "ses_msg_test"})
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", len(msgs))
	}
}

func TestProviderCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	p := &Provider{
		ID: "ollama", Type: "ollama", BaseURL: "http://localhost:11434",
		Enabled: true, CreatedAt: now,
	}

	// Upsert (insert)
	if err := s.UpsertProvider(ctx, p); err != nil {
		t.Fatalf("UpsertProvider (insert): %v", err)
	}

	// Get
	got, err := s.GetProvider(ctx, "ollama")
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.BaseURL != "http://localhost:11434" {
		t.Errorf("expected base_url, got '%s'", got.BaseURL)
	}
	if !got.Enabled {
		t.Error("expected enabled to be true")
	}
	if got.APIKey != "" {
		t.Errorf("expected empty API key, got '%s'", got.APIKey)
	}

	// Upsert (update)
	p.BaseURL = "http://localhost:11435"
	p.APIKey = "secret-key"
	if err := s.UpsertProvider(ctx, p); err != nil {
		t.Fatalf("UpsertProvider (update): %v", err)
	}
	got, _ = s.GetProvider(ctx, "ollama")
	if got.BaseURL != "http://localhost:11435" {
		t.Errorf("expected updated base_url, got '%s'", got.BaseURL)
	}
	if got.APIKey != "secret-key" {
		t.Errorf("expected 'secret-key', got '%s'", got.APIKey)
	}

	// List
	providers, err := s.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(providers))
	}

	// Delete
	if err := s.DeleteProvider(ctx, "ollama"); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	_, err = s.GetProvider(ctx, "ollama")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Delete not found
	err = s.DeleteProvider(ctx, "ollama")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound on double delete, got %v", err)
	}
}

func TestMessageWithParentIDAndMetadata(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	sess := &Session{
		ID: "ses_meta", UserID: "default", Title: "Meta Test",
		Model: "m", Status: "active",
		CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}
	s.CreateSession(ctx, sess)

	parentID := "msg_parent"
	metadata := `{"tool_calls":[]}`
	msg := &Message{
		ID: "msg_child", SessionID: "ses_meta", ParentID: &parentID,
		Role: "assistant", Content: "Response",
		IsActive: true, Model: "gpt-4",
		Metadata:  &metadata,
		CreatedAt: now,
	}
	if err := s.CreateMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMessage(ctx, "msg_child")
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentID == nil || *got.ParentID != "msg_parent" {
		t.Error("expected parent_id 'msg_parent'")
	}
	if got.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", got.Model)
	}
	if got.Metadata == nil || *got.Metadata != `{"tool_calls":[]}` {
		t.Error("expected metadata")
	}
}

// Verify the Store interface is satisfied.
var _ Store = (*SQLiteStore)(nil)
