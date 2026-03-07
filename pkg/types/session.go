package types

import "time"

// Session represents a conversation session with full details.
// Used when loading a specific session (GET /api/sessions/{id}).
type Session struct {
	ID           string    `json:"id"`            // ULID, prefixed: "ses_01J..."
	UserID       string    `json:"user_id"`       // "default" in v1 (multi-user prep)
	Title        string    `json:"title"`         // Auto-generated or user-set
	Model        string    `json:"model"`         // Active model ID for this session
	SystemPrompt string    `json:"system_prompt"` // Per-session system prompt override
	Status       string    `json:"status"`        // "active" | "idle" | "archived"
	TokenCount   int       `json:"token_count"`   // Current context window usage
	MessageCount int       `json:"message_count"` // Total messages (including compacted)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastAccess   time.Time `json:"last_access"`
}

// SessionListItem is a lightweight projection for sidebar listing.
// Excludes system_prompt and token_count to reduce payload for large lists.
type SessionListItem struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastAccess   time.Time `json:"last_access"`
}

// SessionStatus constants
const (
	SessionStatusActive   = "active"
	SessionStatusIdle     = "idle"
	SessionStatusArchived = "archived"
)

// CreateSessionRequest is the request body for POST /api/sessions.
type CreateSessionRequest struct {
	Model        string `json:"model,omitempty"`         // Defaults to config.DefaultModel
	Title        string `json:"title,omitempty"`         // Defaults to "New Chat"
	SystemPrompt string `json:"system_prompt,omitempty"` // Defaults to ""
}

// UpdateSessionRequest is the request body for PATCH /api/sessions/{id}.
// All fields are optional (merge-patch semantics).
type UpdateSessionRequest struct {
	Title        *string `json:"title,omitempty"`
	Model        *string `json:"model,omitempty"`
	SystemPrompt *string `json:"system_prompt,omitempty"`
	Status       *string `json:"status,omitempty"`
}

// SessionListResponse is the response for GET /api/sessions.
type SessionListResponse struct {
	Data    []SessionListItem `json:"data"`
	Total   int               `json:"total"`
	HasMore bool              `json:"has_more"`
}

// SessionDetailResponse is the response for GET /api/sessions/{id}.
type SessionDetailResponse struct {
	Session  Session   `json:"session"`
	Messages []Message `json:"messages"`
}
