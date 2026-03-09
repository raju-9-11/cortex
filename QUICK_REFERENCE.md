# Cortex Codebase - Quick Reference Guide

## Project Statistics
- **Total Go Files**: 31
- **Total Lines of Code**: 6,239
- **Go Version**: 1.24.3
- **Direct Dependencies**: 2 (chi/v5 for HTTP, env/v11 for config)

---

## Key File Locations by Feature

### 1. CORE INFERENCE LOGIC
```
internal/inference/
├── provider.go          (lines 9-29)   — InferenceProvider interface definition
├── registry.go          (lines 15-162) — ProviderRegistry for model resolution
├── openai.go            (lines 19-265) — OpenAI & OpenAI-compatible provider
├── ollama.go            (lines 108-584)— Ollama local provider
├── mock.go              (lines 11-79)  — Mock provider for testing
└── retry.go             (lines 12-50+) — HTTP retry logic
```

### 2. STREAMING & SSE
```
internal/streaming/
└── sse.go              (lines 14-189)  — SSE pipeline, event loop, timeouts

pkg/types/
└── events.go           (lines 18-33)   — StreamEvent & ToolCallEvent types
```

### 3. TYPE DEFINITIONS
```
pkg/types/
├── openai.go           (lines 3-105)   — ChatCompletionRequest/Response/Chunk
├── events.go           (lines 18-33)   — StreamEvent (8 event types)
├── ws_events.go        (lines 1-155)   — WebSocket event types (14 types, not yet implemented)
├── message.go          (lines 1-70)    — Message, MessageMeta, SendMessageRequest
├── session.go          (lines 1-69)    — Session, CreateSessionRequest
├── provider.go         (lines 1-45)    — ProviderInfo, ProviderListResponse
├── health.go           (lines 1-36)    — HealthResponse, HealthComponent
└── errors.go           (lines 1-97)    — APIError, error codes, WriteError()
```

### 4. HTTP API HANDLERS
```
internal/api/
├── handlers_chat.go    (lines 28-82)   — POST /v1/chat/completions, GET /v1/models
├── handlers_health.go  (lines 45-163)  — GET /api/health (no auth)
├── handlers_sessions.go(lines 1-100+)  — Session CRUD & messaging endpoints
└── (tests)

Routes:
- POST /v1/chat/completions (streaming & non-streaming)
- GET /v1/models
- GET /api/health (public)
- GET /api/sessions
- POST /api/sessions
- GET /api/sessions/{id}
- PATCH /api/sessions/{id}
- DELETE /api/sessions/{id}
- POST /api/sessions/{id}/messages
```

### 5. SERVER & CONFIGURATION
```
cmd/cortex/
└── main.go             (lines 18-107)  — Entry point, provider registration

internal/server/
└── server.go           (lines 22-114)  — HTTP server setup, middleware, Chi router

internal/config/
└── config.go           (lines 10-56)   — Config struct, environment variable loading
```

### 6. DATA PERSISTENCE
```
internal/store/
├── store.go            — Store interface
├── sqlite.go           — SQLite implementation
└── migrations/         — Schema files

internal/session/
└── manager.go          — Session CRUD operations
```

### 7. AUTHENTICATION
```
internal/auth/
└── middleware.go       — API key validation middleware
```

---

## InferenceProvider Interface Methods

```go
type InferenceProvider interface {
    // Streaming: provider closes channel when done
    StreamChat(ctx context.Context, req *ChatCompletionRequest, out chan<- StreamEvent) error
    
    // Non-streaming
    Complete(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
    
    // Token counting (character-based estimates)
    CountTokens(messages []ChatMessage) (int, error)
    
    // Model capabilities
    Capabilities(model string) ModelCapabilities
    
    // List available models
    ListModels(ctx context.Context) ([]ModelInfo, error)
    
    // Provider name (used in registry)
    Name() string
}
```

**Implementations**:
1. **OpenAIProvider** — SSE streaming, `POST /chat/completions`
2. **OllamaProvider** — NDJSON streaming, `POST /api/chat`, auto-detects via `Probe()`
3. **MockProvider** — Testing, configurable token delays

---

## Model Resolution Process

**File**: `/internal/inference/registry.go:96-125`

```
Input: "model_name" or "provider/model_name"
                ↓
Step 1: Check "provider/model_name" prefix
        if found → return (provider, "model_name")
                ↓
Step 2: Look up model_name in modelMap
        if found → return (mapped_provider, "model_name")
                ↓
Step 3: Use default provider
        if set → return (default_provider, "model_name")
                ↓
Step 4: Error — no provider found
```

---

## Streaming Flow

```
Client → POST /v1/chat/completions (stream: true)
              ↓
       handlers_chat.go:HandleChatCompletions
              ↓
       registry.Resolve(model) → InferenceProvider
              ↓
       streaming.NewPipeline(provider)
              ↓
       pipeline.Stream(req, responseWriter)
              ↓
    [Start provider.StreamChat() in goroutine]
              ↓
    [Event loop: listen on channel, write SSE]
              ↓
    [Timeouts: 5-min overall, 60-sec stall]
              ↓
    [Format: data: {ChatCompletionChunk JSON}\n\n]
              ↓
    [On done or error: send [DONE] sentinel]
              ↓
Client receives SSE stream
```

---

## StreamEvent Types

| Event Type | Use Case | Field Used |
|-----------|----------|-----------|
| `content.delta` | Text chunk | `.Delta` (string) |
| `tool.start` | Tool call begins | `.ToolCall.ID`, `.ToolCall.Name` |
| `tool.progress` | Tool arguments | `.ToolCall.ID`, `.ToolCall.Arguments` |
| `tool.complete` | Tool call finishes | `.ToolCall.ID` |
| `tool.result` | Tool execution result | `.ToolCall` + metadata |
| `content.done` | Completion finished | `.FinishReason`, `.Usage` |
| `status` | Status update | `.Usage` |
| `error` | Error occurred | `.Error`, `.ErrorMessage` |

---

## Configuration (Environment Variables)

### Server
- `CORTEX_ADDR` (default: `:8080`)
- `CORTEX_DEV` (default: `false`)

### Database
- `DATABASE_URL` (if set → PostgreSQL; else → SQLite)
- `CORTEX_DB_PATH` (default: `cortex.db`)

### Authentication
- `CORTEX_API_KEY` (if set → require auth)

### Providers
- `CORTEX_PROVIDER` (default: `qwen`)
- `CORTEX_MODEL` (default: `qwen2.5:0.5b`)

### Ollama
- `OLLAMA_URL` (default: `http://localhost:11434`)

### OpenAI-Compatible (Qwen, Llama, Minimax, OSS, OpenAI)
- `{PROVIDER}_BASE_URL`
- `{PROVIDER}_API_KEY`

### Limits
- `CORTEX_MAX_TOOL_TIMEOUT` (default: `60s`)
- `CORTEX_MAX_TOOL_OUTPUT` (default: `65536` bytes)
- `CORTEX_MAX_MESSAGE_SIZE` (default: `102400` bytes)

### Logging & CORS
- `CORTEX_LOG_LEVEL` (default: `info`)
- `CORTEX_LOG_FORMAT` (default: `json`)
- `CORTEX_CORS_ORIGINS` (default: `*`)

---

## Error Response Format

```json
{
  "error": {
    "code": "model_not_found",
    "message": "No provider found for model: 'xyz'",
    "type": "not_found",
    "param": "model",
    "details": null
  }
}
```

**Error Types**: `invalid_request_error`, `not_found`, `authentication_error`, `rate_limit_error`, `server_error`, `provider_error`, `timeout_error`, `conflict_error`

---

## Dependency Tree

```
cortex
├── github.com/go-chi/chi/v5 v5.2.5         (HTTP router)
├── github.com/caarlos0/env/v11 v11.4.0     (Env var parsing)
└── modernc.org/sqlite v1.46.1 (indirect)   (SQLite driver)
    ├── modernc.org/libc
    ├── modernc.org/mathutil
    ├── modernc.org/memory
    ├── github.com/google/uuid
    └── [other transitive deps]
```

---

## WebSocket Support (Not Yet Implemented)

**Defined Event Types** (14 types in `/pkg/types/ws_events.go`):

**Server → Client**:
- `inference.started`, `inference.token`, `inference.tool_call`, `inference.tool_result`, `inference.completed`, `inference.error`
- `compaction.started`, `compaction.completed`
- `session.created`, `session.updated`, `session.deleted`
- `model.status_changed`

**Bidirectional**: `ping`, `pong`

**Client → Server**: `subscribe`, `unsubscribe`

**Status**: Types fully defined but no HTTP upgrade handler or WebSocket implementation yet.

---

## Provider Registration Order (from `main.go`)

1. **Ollama** — Auto-detects at `OLLAMA_URL` via `Probe()`
2. **OpenAI** — If `OPENAI_API_KEY` set
3. **Qwen** — If `QWEN_API_KEY` set
4. **Llama** — If `LLAMA_API_KEY` set
5. **Minimax** — If `MINIMAX_API_KEY` set
6. **OSS** — If `OSS_API_KEY` set
7. **Mock Providers** — Fallback if no real providers configured

**First registered becomes default** (unless overridden by `CORTEX_PROVIDER`)

---

## Testing Files

- `/internal/auth/middleware_test.go`
- `/internal/inference/ollama_test.go`
- `/internal/api/handlers_health_test.go`
- `/internal/api/handlers_sessions_test.go`
- `/internal/store/sqlite_test.go`
- `/internal/session/manager_test.go`

---

## Entry Point Flow

**`cmd/cortex/main.go`**:

1. Load config from env vars
2. Initialize SQLite store
3. Run database migrations
4. Create provider registry
5. Register providers (auto-detect Ollama, check env vars for API keys)
6. Refresh model map
7. Create session manager
8. Create auth middleware
9. Print startup banner
10. Create HTTP server with all dependencies
11. Start listening on configured address

