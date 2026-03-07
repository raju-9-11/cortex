package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ---- Domain Models ----

// Session represents a conversation session in the store layer.
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	Status       string    `json:"status"` // "active", "idle", "archived"
	TokenCount   int       `json:"token_count"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastAccess   time.Time `json:"last_access"`
}

// Message represents a single message in a conversation.
type Message struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	ParentID   *string   `json:"parent_id,omitempty"` // nil = root message
	Role       string    `json:"role"`                // "system","user","assistant","tool"
	Content    string    `json:"content"`
	TokenCount int       `json:"token_count"`
	IsActive   bool      `json:"is_active"`       // false after compaction
	Pinned     bool      `json:"pinned"`
	Model      string    `json:"model,omitempty"` // which model generated this
	Metadata   *string   `json:"metadata,omitempty"` // JSON blob
	CreatedAt  time.Time `json:"created_at"`
}

// Provider represents a configured inference provider.
type Provider struct {
	ID        string    `json:"id"`       // "ollama", "openai", etc.
	Type      string    `json:"type"`     // "ollama","openai","anthropic","gemini","openai_compat"
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"-"` // never serialized to JSON
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- Query Parameters ----

// SessionListParams controls filtering and pagination for ListSessions.
type SessionListParams struct {
	UserID string
	Status string // "" = all statuses
	Limit  int    // 0 = default (50)
	Offset int
}

// MessageListParams controls filtering and pagination for ListMessages.
type MessageListParams struct {
	SessionID  string
	ActiveOnly bool // true = only is_active=1
	Limit      int  // 0 = all
	Offset     int
}

// ---- Store Interface ----

// Store is the persistence boundary. Every module talks to the database
// only through this interface.
type Store interface {
	// -- Lifecycle --

	// Migrate runs all pending database migrations. Called once at startup.
	Migrate(ctx context.Context) error
	// Close gracefully shuts down the database connection(s).
	Close() error
	// Ping verifies database connectivity (for health checks).
	Ping(ctx context.Context) error

	// -- Sessions --

	// CreateSession inserts a new session. The caller must set the ID.
	CreateSession(ctx context.Context, s *Session) error
	// GetSession returns a session by ID. Returns ErrNotFound if not found.
	GetSession(ctx context.Context, id string) (*Session, error)
	// ListSessions returns sessions matching the given parameters, ordered
	// by last_access DESC.
	ListSessions(ctx context.Context, params SessionListParams) ([]Session, error)
	// UpdateSession updates an existing session by ID.
	UpdateSession(ctx context.Context, s *Session) error
	// DeleteSession removes a session and its messages (via CASCADE).
	DeleteSession(ctx context.Context, id string) error

	// -- Messages --

	// CreateMessage inserts a new message. The caller must set the ID.
	CreateMessage(ctx context.Context, m *Message) error
	// GetMessage returns a message by ID. Returns ErrNotFound if not found.
	GetMessage(ctx context.Context, id string) (*Message, error)
	// ListMessages returns messages matching the given parameters, ordered
	// by created_at ASC (chronological).
	ListMessages(ctx context.Context, params MessageListParams) ([]Message, error)
	// DeactivateMessages sets is_active=0 for the given message IDs within
	// the specified session. Used by the compaction engine.
	DeactivateMessages(ctx context.Context, sessionID string, messageIDs []string) error

	// -- Providers --

	// UpsertProvider inserts or updates a provider by ID.
	UpsertProvider(ctx context.Context, p *Provider) error
	// GetProvider returns a provider by ID. Returns ErrNotFound if not found.
	GetProvider(ctx context.Context, id string) (*Provider, error)
	// ListProviders returns all providers.
	ListProviders(ctx context.Context) ([]Provider, error)
	// DeleteProvider removes a provider by ID.
	DeleteProvider(ctx context.Context, id string) error
}
