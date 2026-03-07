package types

import "time"

// Message represents a single message in a conversation session.
type Message struct {
	ID         string       `json:"id"`                    // ULID, prefixed: "msg_01J..."
	SessionID  string       `json:"session_id"`            // Parent session
	ParentID   *string      `json:"parent_id"`             // nil = root message (tree for branching)
	Role       string       `json:"role"`                  // "system" | "user" | "assistant" | "tool"
	Content    string       `json:"content"`               // Message text content
	TokenCount int          `json:"token_count"`           // Token count for this message
	IsActive   bool         `json:"is_active"`             // false after compaction
	Pinned     bool         `json:"pinned"`                // Pinned messages survive compaction
	Model      string       `json:"model,omitempty"`       // Which model generated this (assistant only)
	Metadata   *MessageMeta `json:"metadata,omitempty"`    // Tool calls, usage, etc.
	CreatedAt  time.Time    `json:"created_at"`
}

// MessageMeta holds optional structured metadata attached to a message.
type MessageMeta struct {
	ToolCalls     []ToolCall `json:"tool_calls,omitempty"`      // Tool calls made by assistant
	ToolCallID    string     `json:"tool_call_id,omitempty"`    // For role="tool" messages
	FinishReason  string     `json:"finish_reason,omitempty"`   // "stop", "tool_calls", "length", "cancelled"
	Usage         *Usage     `json:"usage,omitempty"`           // Token usage for this completion
	CompactionRef string    `json:"compaction_ref,omitempty"`  // ID of the summary message that replaced this
}

// MessageRole constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// SendMessageRequest is the request body for POST /api/sessions/{id}/messages.
type SendMessageRequest struct {
	Content     string              `json:"content"`                // Message text
	Role        string              `json:"role,omitempty"`         // Defaults to "user", server validates
	Stream      bool                `json:"stream,omitempty"`       // SSE streaming response
	ParentID    *string             `json:"parent_id,omitempty"`    // For branching (Phase 8)
	Attachments []MessageAttachment `json:"attachments,omitempty"`  // Media references (voice/video phase)
}

// SendMessageResponse is the non-streaming response for POST /api/sessions/{id}/messages.
type SendMessageResponse struct {
	UserMessage      Message `json:"user_message"`
	AssistantMessage Message `json:"assistant_message"`
}

// StopResponse is the response for POST /api/sessions/{id}/stop.
type StopResponse struct {
	Stopped          bool   `json:"stopped"`
	PartialMessageID string `json:"partial_message_id,omitempty"`
}

// RegenerateRequest is the request body for POST /api/sessions/{id}/regenerate.
type RegenerateRequest struct {
	MessageID string `json:"message_id"` // Which assistant message to regenerate
	Stream    bool   `json:"stream,omitempty"`
}

// CompactResponse is the response for POST /api/sessions/{id}/compact.
type CompactResponse struct {
	OriginalTokens   int    `json:"original_tokens"`
	CompactedTokens  int    `json:"compacted_tokens"`
	MessagesArchived int    `json:"messages_archived"`
	SummaryMessageID string `json:"summary_message_id"`
}
