# Cortex — Backend API Specification for Frontend

> **Author:** UI/UX Design Architect (review pass)
> **Date:** 2025-07-15
> **Status:** DRAFT — ready for backend implementation
> **Scope:** Defines every REST endpoint, WebSocket event, data model, and error shape
> that the planned React frontend will consume.

---

## 0. Executive Summary: What Exists vs. What's Needed

### ✅ What exists today

| Artifact | Location | Status |
|:---|:---|:---|
| `POST /v1/chat/completions` | `internal/api/handlers_chat.go` | Works (stateless passthrough, streaming + non-streaming) |
| `GET /v1/models` | `internal/api/handlers_chat.go` | Works (aggregates from all providers) |
| `types.ChatCompletionRequest` | `pkg/types/openai.go` | Complete OpenAI-compatible shape |
| `types.ChatCompletionChunk` | `pkg/types/openai.go` | SSE chunk shape |
| `types.StreamEvent` | `pkg/types/events.go` | Internal event with typed enum |
| `InferenceProvider` interface | `internal/inference/provider.go` | 6-method contract |
| Config module | `internal/config/config.go` | Env-var driven, multi-provider keys |
| SSE pipeline | `internal/streaming/sse.go` | Channel-based, backpressure-aware |

### ❌ What's missing (required before any frontend work)

| Gap | Severity | Why the frontend needs it |
|:---|:---|:---|
| **No database / Store layer** | 🔴 Blocker | Sessions, messages, settings — nothing persists |
| **No Session CRUD API** | 🔴 Blocker | Sidebar, conversation switching, history |
| **No structured error envelope** | 🔴 Blocker | Frontend can't parse `http.Error` plain-text strings |
| **No health check endpoint** | 🟡 High | Connection status indicator in topbar |
| **No WebSocket event bus** | 🟡 High | Inspector, real-time status, model status changes |
| **No provider management API** | 🟡 High | Settings page, "Test Connection" button |
| **No message persistence** | 🔴 Blocker | Messages vanish on reload — no chat history |
| **No `user_id` threading** | 🟢 Medium | Multi-user prep (hardcode `"default"` for v1) |
| **No `embed.go` / static file serving** | 🟡 High | Frontend assets can't be served |
| **Inconsistent error responses** | 🔴 Blocker | Some return plain text, some JSON, no error codes |

---

## 1. Standard Error Response Format

**Every** API endpoint MUST return errors in this shape. No plain-text `http.Error()`.

### 1.1 Error Envelope

```json
{
  "error": {
    "code":    "session_not_found",
    "message": "Session with ID 'abc-123' does not exist.",
    "type":    "not_found",
    "param":   "session_id",
    "details": null
  }
}
```

### 1.2 Go Type Definition

```go
// pkg/types/errors.go

package types

import (
    "encoding/json"
    "net/http"
)

// APIError is the canonical error envelope returned by all endpoints.
type APIError struct {
    Code    string `json:"code"`              // Machine-readable: "session_not_found"
    Message string `json:"message"`           // Human-readable: "Session with ID..."
    Type    string `json:"type"`              // Category: see ErrorType constants
    Param   string `json:"param,omitempty"`   // Which parameter caused the error
    Details any    `json:"details,omitempty"` // Optional structured details
}

type APIErrorResponse struct {
    Error APIError `json:"error"`
}

// Error type categories (aligns with OpenAI convention)
const (
    ErrorTypeInvalidRequest  = "invalid_request_error"
    ErrorTypeNotFound        = "not_found"
    ErrorTypeAuthentication  = "authentication_error"
    ErrorTypeRateLimit       = "rate_limit_error"
    ErrorTypeServer          = "server_error"
    ErrorTypeProvider        = "provider_error"
    ErrorTypeTimeout         = "timeout_error"
    ErrorTypeConflict        = "conflict_error"
)

// Standard error codes (machine-readable)
const (
    ErrCodeValidation       = "validation_error"
    ErrCodeMalformedJSON    = "malformed_json"
    ErrCodeSessionNotFound  = "session_not_found"
    ErrCodeModelNotFound    = "model_not_found"
    ErrCodeProviderNotFound = "provider_not_found"
    ErrCodeProviderOffline  = "provider_offline"
    ErrCodeStreamFailed     = "stream_failed"
    ErrCodeUnauthorized     = "unauthorized"
    ErrCodeRateLimited      = "rate_limited"
    ErrCodeInternalError    = "internal_error"
    ErrCodeConflict         = "session_busy"
)

// WriteError writes a structured error response.
func WriteError(w http.ResponseWriter, status int, code, errType, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(APIErrorResponse{
        Error: APIError{
            Code:    code,
            Message: message,
            Type:    errType,
        },
    })
}
```

### 1.3 HTTP Status Code Mapping

| Scenario | HTTP Status | `type` | `code` |
|:---|:---|:---|:---|
| Malformed JSON body | `400` | `invalid_request_error` | `malformed_json` |
| Missing required field | `400` | `invalid_request_error` | `validation_error` |
| Bad API key | `401` | `authentication_error` | `unauthorized` |
| Session not found | `404` | `not_found` | `session_not_found` |
| Model not found | `404` | `not_found` | `model_not_found` |
| Session busy (concurrent inference) | `409` | `conflict_error` | `session_busy` |
| Rate limited | `429` | `rate_limit_error` | `rate_limited` |
| Provider upstream error | `502` | `provider_error` | `provider_offline` |
| Provider timeout | `504` | `timeout_error` | `stream_failed` |
| Internal server error | `500` | `server_error` | `internal_error` |

### 1.4 Refactoring Required in Existing Code

The current `handlers_chat.go` uses bare `http.Error()` in 4 places. These MUST all be
replaced with `types.WriteError()`:

```go
// BEFORE (current code, line 29):
http.Error(w, err.Error(), http.StatusBadRequest)

// AFTER:
types.WriteError(w, http.StatusBadRequest, types.ErrCodeMalformedJSON,
    types.ErrorTypeInvalidRequest, "Invalid JSON in request body: "+err.Error())
```

---

## 2. Backend Data Models for UI

These are the Go structs and SQL tables that must exist to support the planned three-column
UI. They map 1:1 to the schema in `IMPLEMENTATION_PLAN.md` §10 with additional annotations
for the frontend contract.

### 2.1 Session

```go
// pkg/types/session.go

package types

import "time"

type Session struct {
    ID           string    `json:"id"`             // ULID or UUID
    UserID       string    `json:"user_id"`        // "default" in v1
    Title        string    `json:"title"`           // Auto-generated or user-set
    Model        string    `json:"model"`           // Active model ID for this session
    SystemPrompt string    `json:"system_prompt"`   // Per-session system prompt override
    Status       string    `json:"status"`          // "active" | "idle" | "archived"
    TokenCount   int       `json:"token_count"`     // Current context window usage
    MessageCount int       `json:"message_count"`   // Total messages (including inactive)
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    LastAccess   time.Time `json:"last_access"`
}

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
```

**Design rationale:** `SessionListItem` is a lightweight projection used by the sidebar.
The full `Session` (with `system_prompt`, `token_count`) is only loaded when a session is
opened. This prevents over-fetching for a sidebar that may have hundreds of entries.

### 2.2 Message

```go
// pkg/types/message.go

package types

import "time"

type Message struct {
    ID         string          `json:"id"`              // ULID
    SessionID  string          `json:"session_id"`
    ParentID   *string         `json:"parent_id"`       // nil = root message (for branching)
    Role       string          `json:"role"`            // "system" | "user" | "assistant" | "tool"
    Content    string          `json:"content"`
    TokenCount int             `json:"token_count"`
    IsActive   bool            `json:"is_active"`       // false after compaction
    Pinned     bool            `json:"pinned"`
    Model      string          `json:"model,omitempty"` // Which model generated this
    Metadata   *MessageMeta    `json:"metadata,omitempty"`
    CreatedAt  time.Time       `json:"created_at"`
}

type MessageMeta struct {
    ToolCalls     []ToolCall     `json:"tool_calls,omitempty"`
    ToolCallID    string         `json:"tool_call_id,omitempty"`
    FinishReason  string         `json:"finish_reason,omitempty"`
    Usage         *Usage         `json:"usage,omitempty"`
    CompactionRef string         `json:"compaction_ref,omitempty"` // ID of summary msg
}
```

### 2.3 Provider Status (for Model Selector + Settings)

```go
// pkg/types/provider.go

package types

type ProviderInfo struct {
    ID        string   `json:"id"`         // "ollama", "openai", etc.
    Type      string   `json:"type"`       // "ollama" | "openai" | "anthropic" | "openai_compat"
    BaseURL   string   `json:"base_url"`
    Enabled   bool     `json:"enabled"`
    Status    string   `json:"status"`     // "connected" | "offline" | "error" | "unconfigured"
    Models    []string `json:"models"`     // Model IDs available from this provider
    HasAPIKey bool     `json:"has_api_key"` // true/false — NEVER expose the actual key
    IsEnvVar  bool     `json:"is_env_var"`  // true if configured via env var (not editable in UI)
}
```

**Security note:** The `api_key` field is NEVER included in API responses. The frontend
only sees `has_api_key: true` to show a "configured" badge.

### 2.4 Health Status

```go
// pkg/types/health.go

package types

type HealthResponse struct {
    Status    string                    `json:"status"`    // "ok" | "degraded" | "error"
    Version   string                    `json:"version"`
    Uptime    int64                     `json:"uptime_seconds"`
    Database  HealthComponent           `json:"database"`
    Providers map[string]HealthComponent `json:"providers"`
}

type HealthComponent struct {
    Status  string `json:"status"`            // "ok" | "error"
    Latency int64  `json:"latency_ms"`
    Error   string `json:"error,omitempty"`
}
```

---

## 3. Store Interface

The storage layer must implement this interface. SQLite first (Phase 1), PostgreSQL later
(Phase 7). Both implementations run the same compliance test suite.

```go
// internal/store/store.go

package store

import (
    "context"
    "cortex/pkg/types"
)

type Store interface {
    // Sessions
    CreateSession(ctx context.Context, session *types.Session) error
    GetSession(ctx context.Context, id string) (*types.Session, error)
    ListSessions(ctx context.Context, userID string, opts ListOptions) ([]types.SessionListItem, error)
    UpdateSession(ctx context.Context, id string, patch SessionPatch) error
    DeleteSession(ctx context.Context, id string) error

    // Messages
    CreateMessage(ctx context.Context, msg *types.Message) error
    GetMessages(ctx context.Context, sessionID string, opts MessageListOptions) ([]types.Message, error)
    UpdateMessage(ctx context.Context, id string, patch MessagePatch) error
    // No DeleteMessage — messages are soft-deleted via is_active=false

    // Migrations
    Migrate(ctx context.Context) error

    // Health
    Ping(ctx context.Context) error

    Close() error
}

type ListOptions struct {
    Status string // Filter by status. Empty = all.
    Limit  int    // Default 50, max 200
    Offset int
    Search string // FTS query (Phase 8)
}

type MessageListOptions struct {
    ActiveOnly bool // If true, only is_active=true messages
    Limit      int
    Offset     int
}

type SessionPatch struct {
    Title        *string `json:"title,omitempty"`
    Model        *string `json:"model,omitempty"`
    SystemPrompt *string `json:"system_prompt,omitempty"`
    Status       *string `json:"status,omitempty"`
    TokenCount   *int    `json:"token_count,omitempty"`
    MessageCount *int    `json:"message_count,omitempty"`
}

type MessagePatch struct {
    IsActive *bool `json:"is_active,omitempty"`
    Pinned   *bool `json:"pinned,omitempty"`
}
```

---

## 4. Complete REST API Specification

### 4.1 OpenAI-Compatible Endpoints (existing — needs fixes)

---

#### `POST /v1/chat/completions`

**Status:** ✅ Exists — needs error envelope + session awareness

The frontend will call this for all inference. In Phase 2+, the handler should optionally
accept a `session_id` header/field to persist messages.

**Request:**
```jsonc
// Existing shape — no changes needed
{
  "model": "llama3.2:1b",
  "messages": [
    {"role": "system", "content": "You are helpful."},
    {"role": "user", "content": "Hello"}
  ],
  "stream": true,
  "temperature": 0.7,
  "max_tokens": 2048,
  // NEW (Phase 2): Cortex extension fields
  "session_id": "ses_01J...",     // Optional. If set, persist messages.
  "system_prompt": "..."          // Optional override, takes precedence over session default
}
```

**Response (non-streaming):** No change — standard `ChatCompletionResponse`.

**Response (streaming SSE):** No change to SSE format — standard OpenAI chunks.

**Error responses to add:**

| Scenario | Status | Error code |
|:---|:---|:---|
| Invalid JSON body | 400 | `malformed_json` |
| Model not found | 404 | `model_not_found` |
| Provider offline | 502 | `provider_offline` |
| Session busy | 409 | `session_busy` |

---

#### `GET /v1/models`

**Status:** ✅ Exists — works correctly

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3.2:1b",
      "object": "model",
      "created": 1700000000,
      "owned_by": "meta",
      "provider": "ollama",
      "context_window": 8192,
      "capabilities": ["chat", "streaming"],
      "status": "ready"
    }
  ]
}
```

---

#### `GET /v1/models/{model_id}` (NEW — Phase 3)

**Response:** Single `ModelInfo` object or 404.

---

### 4.2 Cortex-Native Endpoints (ALL NEW)

These power the Chat UI's sidebar, session management, settings, and inspector.
All prefixed with `/api/` to separate from the OpenAI-compatible surface.

---

#### `GET /api/health`

Health check for the topbar status indicator.

**Response `200`:**
```json
{
  "status": "ok",
  "version": "0.1.0-dev",
  "uptime_seconds": 3600,
  "database": {
    "status": "ok",
    "latency_ms": 1
  },
  "providers": {
    "ollama": {
      "status": "ok",
      "latency_ms": 12
    },
    "openai": {
      "status": "error",
      "latency_ms": 0,
      "error": "connection refused"
    }
  }
}
```

**Frontend usage:** Polled every 30s OR replaced by WebSocket `model.status_changed` events.
The topbar renders: 🟢 if all `"ok"`, 🟡 if some `"error"`, 🔴 if DB or all providers down.

---

#### `GET /api/sessions`

Lists sessions for the sidebar. Lightweight projection (no messages, no system_prompt).

**Query params:**
- `status` — Filter: `active`, `archived`, `all` (default: `active`)
- `limit` — Default `50`, max `200`
- `offset` — For pagination
- `q` — Search query (Phase 8, FTS5)

**Response `200`:**
```json
{
  "data": [
    {
      "id": "ses_01JDEFGH",
      "title": "Debugging Go concurrency",
      "model": "llama3.2:1b",
      "status": "active",
      "message_count": 24,
      "created_at": "2025-07-15T10:30:00Z",
      "updated_at": "2025-07-15T11:45:00Z",
      "last_access": "2025-07-15T11:45:00Z"
    }
  ],
  "total": 142,
  "has_more": true
}
```

**Frontend usage:** Sidebar groups by relative time (Today / Yesterday / Previous 7 Days / Older)
using `updated_at`. Client-side grouping — no server-side grouping needed.

---

#### `POST /api/sessions`

Creates a new session. Called when user clicks "New Chat" or sends first message.

**Request:**
```json
{
  "model": "llama3.2:1b",
  "title": "",
  "system_prompt": ""
}
```

All fields optional. Defaults: `model` from config `CORTEX_MODEL`, `title` = `"New Chat"`,
`system_prompt` = `""`.

**Response `201`:**
```json
{
  "id": "ses_01JDEFGH",
  "user_id": "default",
  "title": "New Chat",
  "model": "llama3.2:1b",
  "system_prompt": "",
  "status": "active",
  "token_count": 0,
  "message_count": 0,
  "created_at": "2025-07-15T12:00:00Z",
  "updated_at": "2025-07-15T12:00:00Z",
  "last_access": "2025-07-15T12:00:00Z"
}
```

---

#### `GET /api/sessions/{id}`

Get full session with messages. This is what loads when user clicks a session in the sidebar.

**Query params:**
- `active_only` — `true` (default) returns only `is_active=true` messages. `false` includes
  compacted messages (for Inspector).

**Response `200`:**
```json
{
  "session": {
    "id": "ses_01JDEFGH",
    "user_id": "default",
    "title": "Debugging Go concurrency",
    "model": "llama3.2:1b",
    "system_prompt": "You are a Go expert.",
    "status": "active",
    "token_count": 3200,
    "message_count": 8,
    "created_at": "2025-07-15T10:30:00Z",
    "updated_at": "2025-07-15T11:45:00Z",
    "last_access": "2025-07-15T11:45:00Z"
  },
  "messages": [
    {
      "id": "msg_01JABCDE",
      "session_id": "ses_01JDEFGH",
      "parent_id": null,
      "role": "user",
      "content": "How do I use sync.WaitGroup?",
      "token_count": 12,
      "is_active": true,
      "pinned": false,
      "model": null,
      "metadata": null,
      "created_at": "2025-07-15T10:30:05Z"
    },
    {
      "id": "msg_01JFGHIJ",
      "session_id": "ses_01JDEFGH",
      "parent_id": "msg_01JABCDE",
      "role": "assistant",
      "content": "Here's how sync.WaitGroup works...",
      "token_count": 245,
      "is_active": true,
      "pinned": false,
      "model": "llama3.2:1b",
      "metadata": {
        "finish_reason": "stop",
        "usage": {
          "prompt_tokens": 42,
          "completion_tokens": 245,
          "total_tokens": 287
        }
      },
      "created_at": "2025-07-15T10:30:08Z"
    }
  ]
}
```

**Error `404`:**
```json
{
  "error": {
    "code": "session_not_found",
    "message": "Session 'ses_invalid' does not exist.",
    "type": "not_found"
  }
}
```

---

#### `PATCH /api/sessions/{id}`

Update session metadata. Used for: rename, change model, update system prompt.

**Request (all fields optional, merge-patch semantics):**
```json
{
  "title": "My Go concurrency notes",
  "model": "gpt-4o",
  "system_prompt": "You are an expert Go developer."
}
```

**Response `200`:** Full updated `Session` object.

**Error `404`:** Standard session_not_found.

---

#### `DELETE /api/sessions/{id}`

Permanently deletes session and all its messages (CASCADE).

**Response `204`:** No body.

**Error `404`:** Standard session_not_found.

---

#### `POST /api/sessions/{id}/messages`

Append a user message to a session. This is the primary "send message" endpoint that the
Composer bar calls. The backend:
1. Persists the user message
2. Builds context from session history
3. Calls the provider (streaming or non-streaming)
4. Persists the assistant response
5. Updates session token_count and message_count

**Request:**
```json
{
  "content": "How do I use channels in Go?",
  "role": "user",
  "stream": true,
  "parent_id": null
}
```

- `role` is always `"user"` from the frontend (server validates)
- `stream` controls SSE vs. JSON response
- `parent_id` enables branching (Phase 8) — `null` = append to linear thread

**Response (stream=false) `200`:**
```json
{
  "user_message": {
    "id": "msg_01JKLMNO",
    "role": "user",
    "content": "How do I use channels in Go?",
    "token_count": 10,
    "created_at": "2025-07-15T12:01:00Z"
  },
  "assistant_message": {
    "id": "msg_01JPQRST",
    "role": "assistant",
    "content": "Channels in Go are...",
    "token_count": 180,
    "model": "llama3.2:1b",
    "metadata": {
      "finish_reason": "stop",
      "usage": { "prompt_tokens": 50, "completion_tokens": 180, "total_tokens": 230 }
    },
    "created_at": "2025-07-15T12:01:03Z"
  }
}
```

**Response (stream=true):** SSE stream in OpenAI chunk format (same as `/v1/chat/completions`),
but with two additional Cortex-specific SSE events at boundaries:

```
event: cortex.message_created
data: {"id":"msg_01JKLMNO","role":"user","token_count":10}

data: {"id":"chatcmpl-...","choices":[{"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-...","choices":[{"delta":{"content":"Channels "}}]}

data: {"id":"chatcmpl-...","choices":[{"delta":{"content":"in Go "}}]}

data: {"id":"chatcmpl-...","choices":[{"delta":{"content":"are..."}}]}

data: {"id":"chatcmpl-...","choices":[{"finish_reason":"stop"}]}

event: cortex.message_created
data: {"id":"msg_01JPQRST","role":"assistant","token_count":180,"model":"llama3.2:1b"}

data: [DONE]
```

This allows the frontend to update its local message store with persisted IDs without a
separate fetch.

---

#### `POST /api/sessions/{id}/stop`

Cancel an in-flight inference. Called when user clicks the Stop button.

**Response `200`:**
```json
{
  "stopped": true,
  "partial_message_id": "msg_01JPQRST"
}
```

The partial response is persisted with `finish_reason: "cancelled"`.

---

#### `POST /api/sessions/{id}/regenerate`

Regenerate the last assistant response. Deletes (or branches from) the last assistant
message and re-infers.

**Request:**
```json
{
  "message_id": "msg_01JPQRST",
  "stream": true
}
```

**Response:** Same as `POST /api/sessions/{id}/messages` (streaming or non-streaming).

---

#### `POST /api/sessions/{id}/compact`

Trigger manual compaction of session history.

**Response `200`:**
```json
{
  "original_tokens": 6240,
  "compacted_tokens": 2100,
  "messages_archived": 12,
  "summary_message_id": "msg_01JSUMMARY"
}
```

---

#### `GET /api/providers`

List configured providers for Settings page and model selector grouping.

**Response `200`:**
```json
{
  "data": [
    {
      "id": "ollama",
      "type": "ollama",
      "base_url": "http://localhost:11434",
      "enabled": true,
      "status": "connected",
      "models": ["llama3.2:1b", "codellama:7b"],
      "has_api_key": false,
      "is_env_var": false
    },
    {
      "id": "openai",
      "type": "openai",
      "base_url": "https://api.openai.com/v1",
      "enabled": true,
      "status": "connected",
      "models": ["gpt-4o", "gpt-4o-mini"],
      "has_api_key": true,
      "is_env_var": true
    }
  ]
}
```

---

#### `POST /api/providers/{id}/test`

Test provider connectivity. Settings "Test Connection" button.

**Response `200`:**
```json
{
  "status": "ok",
  "latency_ms": 45,
  "models_found": 3
}
```

**Response `502`:**
```json
{
  "error": {
    "code": "provider_offline",
    "message": "Connection to ollama at http://localhost:11434 refused.",
    "type": "provider_error"
  }
}
```

---

#### `GET /api/config`

Returns non-sensitive runtime configuration for the frontend. Powers the Settings page
"Set via environment variable" badges and default values.

**Response `200`:**
```json
{
  "default_model": "llama3.2:1b",
  "default_provider": "ollama",
  "auth_enabled": false,
  "dev_mode": true,
  "version": "0.1.0-dev",
  "env_overrides": ["CORTEX_MODEL", "OLLAMA_URL"]
}
```

`env_overrides` lists which config keys are set via environment variables (and thus
read-only in the UI).

---

## 5. WebSocket Event Specification

### 5.1 Connection

```
GET /ws?session_id=ses_01JDEFGH
Upgrade: websocket
Connection: Upgrade
```

- `session_id` is optional. If set, the client receives events for that session only.
- If omitted, the client receives global events (model status, session lifecycle).
- A client can subscribe to multiple sessions by sending a subscribe message.

### 5.2 Event Envelope

Every WebSocket message (both directions) uses this envelope:

```json
{
  "type": "inference.token",
  "session_id": "ses_01JDEFGH",
  "timestamp": "2025-07-15T12:01:03.456Z",
  "payload": { }
}
```

### 5.3 Go Type Definition

```go
// pkg/types/ws_events.go

package types

import "time"

type WSEventType string

const (
    // Inference lifecycle
    WSEventInferenceStarted   WSEventType = "inference.started"
    WSEventInferenceToken     WSEventType = "inference.token"
    WSEventInferenceToolCall  WSEventType = "inference.tool_call"
    WSEventInferenceToolResult WSEventType = "inference.tool_result"
    WSEventInferenceCompleted WSEventType = "inference.completed"
    WSEventInferenceError     WSEventType = "inference.error"

    // Compaction
    WSEventCompactionStarted   WSEventType = "compaction.started"
    WSEventCompactionCompleted WSEventType = "compaction.completed"

    // Session lifecycle
    WSEventSessionCreated WSEventType = "session.created"
    WSEventSessionUpdated WSEventType = "session.updated"
    WSEventSessionDeleted WSEventType = "session.deleted"

    // Model/provider status
    WSEventModelStatusChanged WSEventType = "model.status_changed"

    // Keepalive
    WSEventPing WSEventType = "ping"
    WSEventPong WSEventType = "pong"

    // Client → Server commands
    WSEventSubscribe   WSEventType = "subscribe"
    WSEventUnsubscribe WSEventType = "unsubscribe"
)

type WSEvent struct {
    Type      WSEventType `json:"type"`
    SessionID string      `json:"session_id,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
    Payload   any         `json:"payload"`
}
```

### 5.4 Event Payloads

#### `inference.started`
```json
{
  "type": "inference.started",
  "session_id": "ses_01JDEFGH",
  "timestamp": "2025-07-15T12:01:00.000Z",
  "payload": {
    "model": "llama3.2:1b",
    "message_id": "msg_01JPQRST",
    "context_tokens": 1200
  }
}
```

#### `inference.token`
```json
{
  "type": "inference.token",
  "session_id": "ses_01JDEFGH",
  "timestamp": "2025-07-15T12:01:00.050Z",
  "payload": {
    "delta": "Hello",
    "message_id": "msg_01JPQRST"
  }
}
```

**Note:** For streaming, the frontend primarily consumes SSE from the
`POST /api/sessions/{id}/messages` response. WebSocket `inference.token` events exist
for the Inspector panel and for other tabs/windows observing the same session.

#### `inference.tool_call`
```json
{
  "payload": {
    "call_id": "call_abc123",
    "tool_name": "read_file",
    "arguments": "{\"path\": \"/etc/hosts\"}",
    "message_id": "msg_01JPQRST"
  }
}
```

#### `inference.tool_result`
```json
{
  "payload": {
    "call_id": "call_abc123",
    "status": "success",
    "output": "127.0.0.1 localhost\n...",
    "duration_ms": 45
  }
}
```

#### `inference.completed`
```json
{
  "payload": {
    "message_id": "msg_01JPQRST",
    "finish_reason": "stop",
    "usage": {
      "prompt_tokens": 1200,
      "completion_tokens": 180,
      "total_tokens": 1380
    }
  }
}
```

#### `inference.error`
```json
{
  "payload": {
    "code": "provider_offline",
    "message": "Ollama connection refused",
    "message_id": "msg_01JPQRST"
  }
}
```

#### `compaction.started`
```json
{
  "payload": {
    "original_tokens": 6240,
    "threshold": 6553
  }
}
```

#### `compaction.completed`
```json
{
  "payload": {
    "original_tokens": 6240,
    "compacted_tokens": 2100,
    "messages_archived": 12
  }
}
```

#### `session.created` / `session.updated`
```json
{
  "payload": {
    "id": "ses_01JDEFGH",
    "title": "Debugging Go concurrency",
    "model": "llama3.2:1b",
    "status": "active",
    "message_count": 24
  }
}
```

#### `session.deleted`
```json
{
  "payload": {
    "id": "ses_01JDEFGH"
  }
}
```

#### `model.status_changed`
```json
{
  "payload": {
    "provider_id": "ollama",
    "model_id": "llama3.2:1b",
    "old_status": "connected",
    "new_status": "offline"
  }
}
```

#### Client → Server: `subscribe` / `unsubscribe`
```json
{
  "type": "subscribe",
  "payload": {
    "session_id": "ses_01JDEFGH"
  }
}
```

#### Keepalive: `ping` / `pong`
Server sends `ping` every 30s. Client must reply with `pong` within 10s or connection
is terminated.

```json
{"type": "ping", "timestamp": "2025-07-15T12:05:00Z", "payload": {}}
```

---

## 6. API Implementation Priority

Based on what the frontend needs at each phase:

### Phase 1 (Skeleton) — Backend-only, no UI

| Priority | Endpoint | Why |
|:---|:---|:---|
| P0 | `pkg/types/errors.go` + refactor existing handlers | Foundation for every endpoint |
| P0 | `internal/store/store.go` interface | Foundation for persistence |
| P0 | `internal/store/sqlite.go` + migrations | Enables session/message persistence |
| P0 | `GET /api/health` | Simplest endpoint, validates wiring |

### Phase 2 (Chat UI MVP) — First usable frontend

| Priority | Endpoint | Why |
|:---|:---|:---|
| P0 | `POST /api/sessions` | "New Chat" button |
| P0 | `GET /api/sessions` | Sidebar listing |
| P0 | `GET /api/sessions/{id}` | Load conversation |
| P0 | `POST /api/sessions/{id}/messages` | Send message (core interaction) |
| P0 | `PATCH /api/sessions/{id}` | Rename, change model |
| P0 | `DELETE /api/sessions/{id}` | Delete from sidebar |
| P0 | `POST /api/sessions/{id}/stop` | Stop button |
| P1 | `embed.go` + static file server | Serve the React build |
| P1 | Auto-title via background LLM call | Sidebar shows meaningful titles |

### Phase 3+ (Multi-provider, Inspector)

| Priority | Endpoint | Why |
|:---|:---|:---|
| P1 | `GET /api/providers` | Settings page |
| P1 | `POST /api/providers/{id}/test` | "Test Connection" button |
| P1 | `GET /api/config` | Settings env-var badges |
| P1 | `GET /v1/models/{model_id}` | Model detail view |
| P2 | `WS /ws` | Inspector real-time feed |
| P2 | `POST /api/sessions/{id}/regenerate` | Regenerate button |
| P2 | `POST /api/sessions/{id}/compact` | Manual compaction |

---

## 7. Immediate Refactoring Checklist

These are concrete changes to the existing codebase that should be made before adding
new endpoints. Each item references the exact file and line.

### 7.1 Create `pkg/types/errors.go`

New file. Contains `APIError`, `APIErrorResponse`, `WriteError()`, and all error
code/type constants. (See §1.2 above for the full implementation.)

### 7.2 Refactor `internal/api/handlers_chat.go`

**4 changes required:**

1. **Line 29** — Replace `http.Error(w, err.Error(), http.StatusBadRequest)` with
   `types.WriteError(w, 400, types.ErrCodeMalformedJSON, types.ErrorTypeInvalidRequest, ...)`

2. **Line 35** — Replace `http.Error(w, "Provider not found for model: "+req.Model, http.StatusNotFound)`
   with `types.WriteError(w, 404, types.ErrCodeModelNotFound, types.ErrorTypeNotFound, ...)`

3. **Line 44** — Replace `http.Error(w, err.Error(), http.StatusInternalServerError)` with
   `types.WriteError(w, 500, types.ErrCodeStreamFailed, types.ErrorTypeServer, ...)`

4. **Line 50** — Replace `http.Error(w, err.Error(), http.StatusInternalServerError)` with
   `types.WriteError(w, 500, types.ErrCodeInternalError, types.ErrorTypeServer, ...)`

### 7.3 Refactor `internal/streaming/sse.go`

**Line 113** — `writeSSEError` manually formats JSON. Replace with:

```go
func writeSSEError(w http.ResponseWriter, err error) {
    errResp := types.APIErrorResponse{
        Error: types.APIError{
            Code:    types.ErrCodeStreamFailed,
            Message: err.Error(),
            Type:    types.ErrorTypeServer,
        },
    }
    data, _ := json.Marshal(errResp)
    fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
}
```

### 7.4 Fix Model Routing in `handlers_chat.go`

**Lines 76–98** — `getProviderForModel()` has hardcoded model-to-provider mappings. This
must be replaced with a proper model registry lookup before Phase 2. Intermediate fix:

```go
func (router *Router) getProviderForModel(model string) inference.InferenceProvider {
    // 1. Check if model contains provider prefix: "openai/gpt-4o" → provider="openai"
    if idx := strings.Index(model, "/"); idx > 0 {
        providerName := model[:idx]
        if p, ok := router.providers[providerName]; ok {
            return p
        }
    }
    // 2. Ask each provider if they own this model
    for _, p := range router.providers {
        models, err := p.ListModels(context.Background())
        if err != nil {
            continue
        }
        for _, m := range models {
            if m.ID == model {
                return p
            }
        }
    }
    // 3. Fall back to default provider
    if p, ok := router.providers[router.defaultProvider]; ok {
        return p
    }
    // 4. Fall back to first available
    for _, p := range router.providers {
        return p
    }
    return nil
}
```

### 7.5 Add CORS `DELETE` and `PATCH` Methods

**`internal/server/server.go` line 38** — Current CORS only allows `GET, POST, OPTIONS`.
The session API requires `PATCH` and `DELETE`:

```go
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
```

### 7.6 Add `Content-Type` Validation Middleware

Currently the `POST /v1/chat/completions` handler will silently fail on non-JSON bodies.
Add a middleware or check:

```go
if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
    types.WriteError(w, 415, "unsupported_media_type",
        types.ErrorTypeInvalidRequest, "Content-Type must be application/json")
    return
}
```

---

## 8. Frontend-Backend Contract Summary

This table is the definitive reference a frontend developer uses to build each UI component:

| UI Component | Primary Endpoint | WebSocket Event | Polling Fallback |
|:---|:---|:---|:---|
| **Topbar: Status Indicator** | `GET /api/health` | `model.status_changed` | Poll every 30s |
| **Topbar: Model Selector** | `GET /v1/models` | — | Fetch on open |
| **Sidebar: Session List** | `GET /api/sessions` | `session.created`, `session.updated`, `session.deleted` | Fetch on focus |
| **Sidebar: New Chat** | `POST /api/sessions` | — | — |
| **Sidebar: Rename** | `PATCH /api/sessions/{id}` | `session.updated` | — |
| **Sidebar: Delete** | `DELETE /api/sessions/{id}` | `session.deleted` | — |
| **Messages: Load** | `GET /api/sessions/{id}` | — | — |
| **Messages: Send** | `POST /api/sessions/{id}/messages` | `inference.*` | — |
| **Messages: Stream** | SSE from `POST .../messages?stream=true` | `inference.token` (mirror) | — |
| **Messages: Stop** | `POST /api/sessions/{id}/stop` | `inference.completed` | — |
| **Messages: Regenerate** | `POST /api/sessions/{id}/regenerate` | `inference.*` | — |
| **Inspector: Token Usage** | `GET /api/sessions/{id}` (token_count) | `inference.completed` | — |
| **Inspector: Event Stream** | — | All `inference.*`, `compaction.*` | — |
| **Settings: Providers** | `GET /api/providers` | `model.status_changed` | — |
| **Settings: Test Connection** | `POST /api/providers/{id}/test` | — | — |
| **Settings: Config Info** | `GET /api/config` | — | — |

---

## 9. ID Generation Strategy

All IDs should use **ULIDs** (Universally Unique Lexicographically Sortable Identifiers):

- Sortable by creation time (no need for `ORDER BY created_at` in most queries)
- URL-safe, no special characters
- 26 characters, case-insensitive

Prefixed for clarity:
- Sessions: `ses_01J5K3Y8...`
- Messages: `msg_01J5K3Y9...`
- Providers: human-readable strings (`"ollama"`, `"openai"`)

**Go library:** `github.com/oklog/ulid/v2`

---

## 10. Summary of New Files to Create

| File | Purpose |
|:---|:---|
| `pkg/types/errors.go` | Error envelope, WriteError helper, error constants |
| `pkg/types/session.go` | Session, SessionListItem structs |
| `pkg/types/message.go` | Message, MessageMeta structs |
| `pkg/types/provider.go` | ProviderInfo struct |
| `pkg/types/health.go` | HealthResponse, HealthComponent structs |
| `pkg/types/ws_events.go` | WebSocket event envelope, event type constants |
| `internal/store/store.go` | Store interface + ListOptions, Patch types |
| `internal/store/sqlite.go` | SQLite implementation |
| `internal/store/migrations/001_init.sql` | Initial schema (sessions, messages, providers, _migrations) |
| `internal/api/handlers_sessions.go` | Session CRUD handlers |
| `internal/api/handlers_health.go` | Health check handler |
| `internal/api/handlers_providers.go` | Provider listing + test handler |
| `internal/api/handlers_config.go` | Config info handler |
| `internal/events/bus.go` | In-process pub/sub event bus |
| `internal/api/ws_handler.go` | WebSocket upgrade + event fan-out |
| `embed.go` | `//go:embed ui/dist` directive |

---

*This specification is ready for direct implementation. Each endpoint has concrete
request/response shapes. Each data model has Go struct definitions. Each error case
has a status code and machine-readable error code. A developer can pick up any section
and implement it without ambiguity.*
