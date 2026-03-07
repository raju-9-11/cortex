package types

import "time"

// WSEventType identifies the type of a WebSocket event.
type WSEventType string

// --- Server → Client: Inference lifecycle ---

const (
	WSEventInferenceStarted    WSEventType = "inference.started"
	WSEventInferenceToken      WSEventType = "inference.token"
	WSEventInferenceToolCall   WSEventType = "inference.tool_call"
	WSEventInferenceToolResult WSEventType = "inference.tool_result"
	WSEventInferenceCompleted  WSEventType = "inference.completed"
	WSEventInferenceError      WSEventType = "inference.error"
)

// --- Server → Client: Compaction lifecycle ---

const (
	WSEventCompactionStarted   WSEventType = "compaction.started"
	WSEventCompactionCompleted WSEventType = "compaction.completed"
)

// --- Server → Client: Session lifecycle ---

const (
	WSEventSessionCreated WSEventType = "session.created"
	WSEventSessionUpdated WSEventType = "session.updated"
	WSEventSessionDeleted WSEventType = "session.deleted"
)

// --- Server → Client: Model/provider status ---

const (
	WSEventModelStatusChanged WSEventType = "model.status_changed"
)

// --- Bidirectional: Keepalive ---

const (
	WSEventPing WSEventType = "ping"
	WSEventPong WSEventType = "pong"
)

// --- Client → Server: Subscription management ---

const (
	WSEventSubscribe   WSEventType = "subscribe"
	WSEventUnsubscribe WSEventType = "unsubscribe"
)

// WSEvent is the canonical envelope for all WebSocket messages (both directions).
//
// Example:
//
//	{
//	  "type": "inference.token",
//	  "session_id": "ses_01JDEFGH",
//	  "timestamp": "2025-07-15T12:01:00.050Z",
//	  "payload": { "delta": "Hello", "message_id": "msg_01JPQRST" }
//	}
type WSEvent struct {
	Type      WSEventType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   any         `json:"payload"`
}

// --- Typed payloads for each event type ---

// InferenceStartedPayload is sent when inference begins.
type InferenceStartedPayload struct {
	Model         string `json:"model"`
	MessageID     string `json:"message_id"`
	ContextTokens int    `json:"context_tokens"`
}

// InferenceTokenPayload is sent for each streamed token.
type InferenceTokenPayload struct {
	Delta     string `json:"delta"`
	MessageID string `json:"message_id"`
}

// InferenceToolCallPayload is sent when the model invokes a tool.
type InferenceToolCallPayload struct {
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	MessageID string `json:"message_id"`
}

// InferenceToolResultPayload is sent when a tool execution completes.
type InferenceToolResultPayload struct {
	CallID     string `json:"call_id"`
	Status     string `json:"status"` // "success" | "error" | "timeout"
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// InferenceCompletedPayload is sent when inference finishes.
type InferenceCompletedPayload struct {
	MessageID    string `json:"message_id"`
	FinishReason string `json:"finish_reason"` // "stop", "tool_calls", "length", "cancelled"
	Usage        *Usage `json:"usage,omitempty"`
}

// InferenceErrorPayload is sent when inference encounters an error.
type InferenceErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	MessageID string `json:"message_id,omitempty"`
}

// CompactionStartedPayload is sent when context compaction begins.
type CompactionStartedPayload struct {
	OriginalTokens int `json:"original_tokens"`
	Threshold      int `json:"threshold"`
}

// CompactionCompletedPayload is sent when context compaction finishes.
type CompactionCompletedPayload struct {
	OriginalTokens   int `json:"original_tokens"`
	CompactedTokens  int `json:"compacted_tokens"`
	MessagesArchived int `json:"messages_archived"`
}

// SessionPayload is sent for session.created / session.updated events.
type SessionPayload struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	Status       string `json:"status"`
	MessageCount int    `json:"message_count"`
}

// SessionDeletedPayload is sent for session.deleted events.
type SessionDeletedPayload struct {
	ID string `json:"id"`
}

// ModelStatusChangedPayload is sent when a provider's connectivity changes.
type ModelStatusChangedPayload struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	OldStatus  string `json:"old_status"`
	NewStatus  string `json:"new_status"`
}

// SubscribePayload is sent by the client to subscribe to session events.
type SubscribePayload struct {
	SessionID string `json:"session_id"`
}
