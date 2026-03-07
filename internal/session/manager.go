package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"forge/internal/store"

	"github.com/google/uuid"
)

// Manager handles session lifecycle and business logic.
// API handlers call Manager, NEVER the Store directly.
type Manager interface {
	Create(ctx context.Context, params CreateParams) (*store.Session, error)
	Get(ctx context.Context, id string) (*store.Session, error)
	List(ctx context.Context, userID string) ([]store.Session, error)
	Update(ctx context.Context, id string, params UpdateParams) (*store.Session, error)
	Delete(ctx context.Context, id string) error
	AddMessage(ctx context.Context, sessionID string, msg *store.Message) error
	GetMessages(ctx context.Context, sessionID string) ([]store.Message, error)
}

// CreateParams holds the parameters for creating a new session.
type CreateParams struct {
	Model        string // Required — falls back to config default
	Title        string // Optional — defaults to "New Chat"
	SystemPrompt string // Optional — defaults to ""
	UserID       string // "default" in v1
}

// UpdateParams holds the parameters for updating a session.
// nil fields are not updated (merge-patch semantics).
type UpdateParams struct {
	Title        *string // nil = don't update
	Model        *string
	SystemPrompt *string
	Status       *string
}

// SessionManagerImpl is the concrete implementation of Manager.
type SessionManagerImpl struct {
	store        store.Store
	defaultModel string
}

// Verify interface compliance at compile time.
var _ Manager = (*SessionManagerImpl)(nil)

// NewManager creates a new SessionManagerImpl with the given store and default model.
func NewManager(s store.Store, defaultModel string) *SessionManagerImpl {
	return &SessionManagerImpl{
		store:        s,
		defaultModel: defaultModel,
	}
}

// Create creates a new session with sensible defaults applied.
func (m *SessionManagerImpl) Create(ctx context.Context, params CreateParams) (*store.Session, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Apply defaults.
	model := params.Model
	if model == "" {
		model = m.defaultModel
	}

	title := params.Title
	if title == "" {
		title = "New Chat"
	}

	userID := params.UserID
	if userID == "" {
		userID = "default"
	}

	sess := &store.Session{
		ID:           "sess_" + uuid.NewString()[:8],
		UserID:       userID,
		Title:        title,
		Model:        model,
		SystemPrompt: params.SystemPrompt,
		Status:       "active",
		TokenCount:   0,
		MessageCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastAccess:   now,
	}

	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("session.Create: %w", err)
	}

	return sess, nil
}

// Get retrieves a session by ID and updates its last_access timestamp.
func (m *SessionManagerImpl) Get(ctx context.Context, id string) (*store.Session, error) {
	sess, err := m.store.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("session %q: %w", id, store.ErrNotFound)
		}
		return nil, fmt.Errorf("session.Get: %w", err)
	}

	// Update last_access timestamp.
	sess.LastAccess = time.Now().UTC().Truncate(time.Millisecond)
	if err := m.store.UpdateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("session.Get update last_access: %w", err)
	}

	return sess, nil
}

// List returns all sessions for the given user.
func (m *SessionManagerImpl) List(ctx context.Context, userID string) ([]store.Session, error) {
	sessions, err := m.store.ListSessions(ctx, store.SessionListParams{
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("session.List: %w", err)
	}
	return sessions, nil
}

// Update applies partial updates to an existing session.
func (m *SessionManagerImpl) Update(ctx context.Context, id string, params UpdateParams) (*store.Session, error) {
	sess, err := m.store.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("session %q: %w", id, store.ErrNotFound)
		}
		return nil, fmt.Errorf("session.Update: %w", err)
	}

	// Apply non-nil fields.
	if params.Title != nil {
		sess.Title = *params.Title
	}
	if params.Model != nil {
		sess.Model = *params.Model
	}
	if params.SystemPrompt != nil {
		sess.SystemPrompt = *params.SystemPrompt
	}
	if params.Status != nil {
		sess.Status = *params.Status
	}

	sess.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)

	if err := m.store.UpdateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("session.Update: %w", err)
	}

	return sess, nil
}

// Delete removes a session by ID.
func (m *SessionManagerImpl) Delete(ctx context.Context, id string) error {
	if err := m.store.DeleteSession(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("session %q: %w", id, store.ErrNotFound)
		}
		return fmt.Errorf("session.Delete: %w", err)
	}
	return nil
}

// AddMessage adds a message to a session, auto-generating the message ID
// and updating the session's message_count and updated_at.
func (m *SessionManagerImpl) AddMessage(ctx context.Context, sessionID string, msg *store.Message) error {
	// Generate message ID and set fields.
	msg.ID = "msg_" + uuid.NewString()[:8]
	msg.SessionID = sessionID
	msg.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	msg.IsActive = true

	if err := m.store.CreateMessage(ctx, msg); err != nil {
		return fmt.Errorf("session.AddMessage: %w", err)
	}

	// Increment session's message_count and updated_at.
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session.AddMessage get session: %w", err)
	}
	sess.MessageCount++
	sess.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	if err := m.store.UpdateSession(ctx, sess); err != nil {
		return fmt.Errorf("session.AddMessage update session: %w", err)
	}

	return nil
}

// GetMessages returns all active messages for a session.
func (m *SessionManagerImpl) GetMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	messages, err := m.store.ListMessages(ctx, store.MessageListParams{
		SessionID:  sessionID,
		ActiveOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("session.GetMessages: %w", err)
	}
	return messages, nil
}
