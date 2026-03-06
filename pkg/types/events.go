package types

import "encoding/json"

type EventType string

const (
	EventContentDelta     EventType = "content.delta"
	EventToolCallStart    EventType = "tool.start"
	EventToolCallDelta    EventType = "tool.progress"
	EventToolCallComplete EventType = "tool.complete"
	EventToolResult       EventType = "tool.result"
	EventContentDone      EventType = "content.done"
	EventStatus           EventType = "status"
	EventError            EventType = "error"
)

type StreamEvent struct {
	Type         EventType       `json:"type"`
	Delta        string          `json:"delta,omitempty"`
	ToolCall     *ToolCallEvent  `json:"tool_call,omitempty"`
	Error        error           `json:"-"` // Not serialized directly
	ErrorMessage string          `json:"error,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	Raw          json.RawMessage `json:"raw,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
}

type ToolCallEvent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
