# Forge — Senior Developer Technical Review

**Reviewer:** Senior Full-Stack Software Engineer  
**Document Under Review:** `IMPLEMENTATION_PLAN.md`  
**Date:** 2025-01-07  
**Verdict:** Strong foundational vision. Needs significant hardening before implementation begins. This review identifies **37 concrete action items** across 7 categories.

---

## Table of Contents

1. [Project Structure & Architecture](#1-project-structure--architecture)
2. [API Design Deep Dive](#2-api-design-deep-dive)
3. [Edge Cases & Error Handling](#3-edge-cases--error-handling)
4. [Testing Strategy](#4-testing-strategy)
5. [Performance Considerations](#5-performance-considerations)
6. [Security Review](#6-security-review)
7. [Implementation Gaps](#7-implementation-gaps)
8. [Summary of Action Items](#8-summary-of-action-items)

---

## 1. Project Structure & Architecture

### 1.1 Proposed Go Project Layout

The plan mentions `cmd/forge` but doesn't define the full layout. Here's a concrete, idiomatic Go structure:

```
forge/
├── cmd/
│   └── forge/
│       └── main.go              # Entry point: flag parsing, DI wiring, server start
├── internal/
│   ├── server/
│   │   ├── server.go            # HTTP server setup, middleware chain, graceful shutdown
│   │   └── routes.go            # Route registration (chi mux wiring)
│   ├── api/
│   │   ├── v1/
│   │   │   ├── chat.go          # POST /v1/chat/completions handler
│   │   │   ├── models.go        # GET  /v1/models handler
│   │   │   └── embeddings.go    # POST /v1/embeddings handler (Phase 5+)
│   │   └── forge/
│   │       ├── sessions.go      # Forge-native session management API
│   │       ├── tools.go         # Forge-native tool management API
│   │       └── events.go        # WebSocket /ws event hub handler
│   ├── inference/
│   │   ├── provider.go          # Provider interface definition
│   │   ├── registry.go          # Provider registry + factory
│   │   ├── ollama.go            # Ollama provider implementation
│   │   ├── openai.go            # OpenAI API provider
│   │   ├── anthropic.go         # Anthropic API provider
│   │   ├── llamacpp.go          # Local llama.cpp server provider
│   │   └── mock.go              # Mock provider for testing
│   ├── streaming/
│   │   ├── sse.go               # SSE encoder/writer with flush control
│   │   ├── pipeline.go          # Stream pipeline: intercept, buffer, fan-out
│   │   └── backpressure.go      # Backpressure + client disconnect detection
│   ├── context/
│   │   ├── manager.go           # Context window assembly + token budget
│   │   ├── tokenizer.go         # Token counting abstraction
│   │   └── compaction.go        # Compaction algorithm
│   ├── tools/
│   │   ├── executor.go          # Tool execution orchestrator
│   │   ├── sandbox_local.go     # Local os/exec sandbox
│   │   ├── sandbox_docker.go    # Docker container sandbox
│   │   ├── manifest.go          # Tool manifest loader (JSON/YAML)
│   │   └── interceptor.go       # Stream interceptor: pause-execute-resume
│   ├── store/
│   │   ├── store.go             # Store interface (sessions, messages, etc.)
│   │   ├── sqlite.go            # SQLite implementation
│   │   ├── postgres.go          # PostgreSQL implementation
│   │   └── migrations/          # Embed-able SQL migration files
│   │       ├── 001_init.sql
│   │       └── ...
│   ├── auth/
│   │   ├── middleware.go        # API key auth middleware
│   │   └── session.go           # UI session token management
│   └── config/
│       └── config.go            # Unified configuration (env + flags + file)
├── pkg/
│   └── types/
│       ├── openai.go            # OpenAI-compatible request/response types
│       ├── events.go            # Stream event types (typed enums, not strings)
│       └── errors.go            # Structured error types
├── ui/
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── App.tsx
│   │   ├── pages/
│   │   │   ├── Chat.tsx
│   │   │   └── Inspector.tsx
│   │   ├── components/
│   │   ├── hooks/
│   │   │   ├── useSSE.ts        # SSE streaming hook
│   │   │   └── useWebSocket.ts  # WebSocket event bus hook
│   │   └── lib/
│   └── dist/                    # Build output (gitignored, embedded at compile)
├── embed.go                     # //go:embed ui/dist directive
├── go.mod
├── go.sum
├── Makefile                     # Build orchestration (ui + go)
├── Dockerfile
└── docker-compose.yml
```

**Key rationale:**
- `internal/` prevents external import — all Forge internals are private by Go convention.
- `pkg/types/` is the only public package — it holds the OpenAI-compatible types so third-party tools can import them.
- `streaming/` is separated from `api/` because the SSE pipeline is reused by both the OpenAI-compatible API and the Forge WebSocket event bus.

### 1.2 Frontend Build Pipeline Integration

The plan mentions `go:embed` but doesn't address the build choreography. This is critical — you can't `go build` unless `ui/dist/` already exists.

**Recommended approach: `Makefile` as the single entry point.**

```makefile
# Makefile
.PHONY: all build dev clean ui go test

all: build

# Build the frontend first, then compile Go with the embedded assets
build: ui go

ui:
	cd ui && npm ci && npm run build

go:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(shell git describe --tags --always)" \
		-o bin/forge ./cmd/forge

# Development: run UI dev server + Go server concurrently
dev:
	@echo "Starting dev mode..."
	cd ui && npm run dev &
	FORGE_DEV=true go run ./cmd/forge

# For CI: create a stub ui/dist if it doesn't exist (allows `go test` without node)
test:
	@mkdir -p ui/dist && touch ui/dist/index.html
	go test ./... -race -count=1

clean:
	rm -rf bin/ ui/dist/ ui/node_modules/
```

**The `embed.go` file:**

```go
package forge

import "embed"

//go:embed ui/dist/*
var UIAssets embed.FS
```

**CRITICAL: The `go test` problem.** If `ui/dist/` doesn't exist, `go build` and `go test` will fail at compile time because the `//go:embed` directive references a non-existent directory. The `Makefile` test target handles this by creating a stub. Alternatively, use a build tag:

```go
//go:build !noui

package forge

import "embed"

//go:embed ui/dist/*
var UIAssets embed.FS
```

This lets CI run `go test -tags noui ./...` without needing Node.js installed.

### 1.3 Dependency Management Strategy

**Go side:**
| Dependency | Purpose | Justification |
|---|---|---|
| `github.com/go-chi/chi/v5` | HTTP router | Lightweight, stdlib-compatible, middleware ecosystem |
| `github.com/mattn/go-sqlite3` | SQLite driver | Most mature Go SQLite driver (CGO) |
| `modernc.org/sqlite` | SQLite driver (pure Go) | **Prefer this** — enables `CGO_ENABLED=0` per the plan |
| `github.com/jackc/pgx/v5` | PostgreSQL driver | Best-in-class Go PG driver, pure Go |
| `github.com/pkoukk/tiktoken-go` | Token counting | OpenAI-compatible BPE tokenizer |
| `github.com/rs/zerolog` | Structured logging | Zero-alloc, JSON-native |
| `github.com/caarlos0/env/v11` | Config from env | Struct-tag-based, clean |

> **⚠️ CONFLICT: `CGO_ENABLED=0` vs `mattn/go-sqlite3`.**  
> The plan says `CGO_ENABLED=0` for static binaries AND SQLite. These are mutually exclusive with `mattn/go-sqlite3` (which requires CGO). **You must use `modernc.org/sqlite`** (a pure-Go SQLite implementation) or accept CGO and use musl for static linking. This is a decision that must be made before Phase 1.

**Frontend side:**
| Dependency | Purpose |
|---|---|
| React 19 | UI framework |
| Tailwind CSS v4 | Utility-first CSS |
| Lucide React | Icon library (tree-shakable) |
| Vite | Build tool (fast, ESM-native) |
| `react-markdown` + `rehype-highlight` | Markdown rendering with syntax highlighting |

---

## 2. API Design Deep Dive

### 2.1 OpenAI-Compatible Endpoints — What Exactly?

The plan only mentions `/v1/chat/completions`. For true OpenAI SDK compatibility (so users can point `openai.Client(base_url="http://localhost:8080")` at Forge), you need at minimum:

| Endpoint | Method | Priority | Notes |
|---|---|---|---|
| `/v1/chat/completions` | POST | **Phase 1** | Core. Must support `stream: true/false` |
| `/v1/models` | GET | **Phase 1** | Required — SDKs call this to validate connectivity |
| `/v1/models/{model}` | GET | Phase 2 | Optional but helps tooling |
| `/v1/embeddings` | POST | Phase 5 | Only if Forge supports embedding providers |

**Request/Response types must be exact.** Here's the critical struct:

```go
// pkg/types/openai.go

type ChatCompletionRequest struct {
    Model       string          `json:"model"`
    Messages    []ChatMessage   `json:"messages"`
    Stream      bool            `json:"stream,omitempty"`
    Temperature *float64        `json:"temperature,omitempty"` // Pointer: distinguish 0.0 from absent
    TopP        *float64        `json:"top_p,omitempty"`
    MaxTokens   *int            `json:"max_tokens,omitempty"`
    Tools       []Tool          `json:"tools,omitempty"`
    ToolChoice  any             `json:"tool_choice,omitempty"` // string | object
    Stop        any             `json:"stop,omitempty"`        // string | []string
    N           int             `json:"n,omitempty"`
    User        string          `json:"user,omitempty"`
}

type ChatMessage struct {
    Role       string      `json:"role"`                  // "system" | "user" | "assistant" | "tool"
    Content    any         `json:"content"`               // string | []ContentPart (multimodal)
    Name       string      `json:"name,omitempty"`
    ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
    ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ChatCompletionResponse struct {
    ID      string   `json:"id"`
    Object  string   `json:"object"` // "chat.completion"
    Created int64    `json:"created"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   *Usage   `json:"usage,omitempty"`
}

type ChatCompletionChunk struct {
    ID      string        `json:"id"`
    Object  string        `json:"object"` // "chat.completion.chunk"
    Created int64         `json:"created"`
    Model   string        `json:"model"`
    Choices []ChunkChoice `json:"choices"`
}
```

> **⚠️ Watch out for `Content` typing.** OpenAI's API allows `content` to be either a string or an array of content parts (for vision/multimodal). Use `any` and handle deserialization manually, or use a custom `json.Unmarshaler`.

### 2.2 Forge-Native API

**Yes, absolutely have a Forge-native API alongside the OpenAI one.** The OpenAI-compatible API is for drop-in SDK compatibility. But Forge has features (sessions, tool manifests, compaction, inspector) that don't map to OpenAI's API.

```
# Forge-Native API (suggested)
GET    /api/sessions                    # List sessions
POST   /api/sessions                    # Create session
GET    /api/sessions/{id}               # Get session (with messages)
DELETE /api/sessions/{id}               # Delete session
POST   /api/sessions/{id}/compact       # Trigger manual compaction

GET    /api/tools                       # List registered tools
POST   /api/tools                       # Register a tool dynamically
GET    /api/tools/{name}                # Get tool details
DELETE /api/tools/{name}                # Deregister a tool

GET    /api/providers                   # List configured inference providers
GET    /api/health                      # Health check (detailed)
GET    /api/config                      # Runtime config (redacted secrets)

WS     /ws                             # WebSocket event bus (inspector, live state)
```

The Chat UI talks to the Forge-native API (for session management) and opens SSE connections to `/v1/chat/completions` for streaming. The Inspector UI connects to `/ws` for real-time events.

### 2.3 WebSocket vs SSE Trade-offs

The plan uses **both**: SSE for the OpenAI-compatible streaming API and WebSocket for the event bus. This is the correct architecture. Here's why:

| Concern | SSE (`/v1/chat/completions`) | WebSocket (`/ws`) |
|---|---|---|
| **Direction** | Server → Client (unidirectional) | Bidirectional |
| **Use case** | Token streaming | Inspector events, status updates, tool progress |
| **Compatibility** | Required by OpenAI SDK | Forge-specific |
| **Reconnection** | Built into EventSource API | Manual (implement heartbeat + reconnect) |
| **Proxy compat** | Excellent (HTTP/1.1) | Can be tricky (needs Upgrade header passthrough) |

**Recommendation:** Keep this dual approach. But add these details:

1. **SSE: Use proper `text/event-stream` format with `id:` fields** so clients can resume with `Last-Event-ID` header after disconnection.
2. **WebSocket: Implement ping/pong heartbeat** (every 30s) to detect dead connections behind proxies.
3. **WebSocket: Structured message envelope:**

```go
// pkg/types/events.go

type EventType string

const (
    EventContentDelta  EventType = "content.delta"
    EventContentDone   EventType = "content.done"
    EventToolStart     EventType = "tool.start"
    EventToolProgress  EventType = "tool.progress"
    EventToolResult    EventType = "tool.result"
    EventToolError     EventType = "tool.error"
    EventSessionUpdate EventType = "session.update"
    EventTokenUsage    EventType = "token.usage"
    EventError         EventType = "error"
)

type WSEvent struct {
    Type      EventType       `json:"type"`
    SessionID string          `json:"session_id"`
    Timestamp int64           `json:"ts"`
    Data      json.RawMessage `json:"data"`
}
```

> **⚠️ The plan's event format is stringly-typed** (`"type": "content"`, `"type": "status"`). Use typed constants and a discriminated union pattern. This prevents typos and enables exhaustive matching on the frontend.

### 2.4 API Versioning Strategy

The OpenAI-compatible endpoints are already versioned (`/v1/`). For the Forge-native API:

- **Start with `/api/` (unversioned)** for the MVP. Forge is pre-1.0 — don't over-engineer versioning.
- **When breaking changes are needed**, move to `/api/v2/` and keep `/api/v1/` alive for one major version.
- **Use `Accept: application/json; version=2` header** as a secondary mechanism for minor variations.
- **Never break the OpenAI-compatible API.** If OpenAI changes their spec, add a new version (`/v2/chat/completions`) without removing `/v1/`.

---

## 3. Edge Cases & Error Handling

### 3.1 LLM Provider Goes Down Mid-Stream

This is the most critical edge case and the plan only says "cancel the LLM inference context." That's necessary but not sufficient.

**Full failure handling sequence:**

```go
// internal/streaming/pipeline.go

func (p *Pipeline) Stream(ctx context.Context, req *types.ChatCompletionRequest, w http.ResponseWriter) error {
    // 1. Set up SSE writer
    flusher, ok := w.(http.Flusher)
    if !ok {
        return errors.New("streaming not supported")
    }

    // 2. Create cancellable context tied to client connection
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    // Detect client disconnect
    go func() {
        <-w.(http.CloseNotifier).CloseNotify() // deprecated but illustrative
        cancel()
    }()
    // Better: use http.NewResponseController(w).SetWriteDeadline()

    // 3. Start inference stream
    stream, err := p.provider.StreamCompletion(ctx, req)
    if err != nil {
        // Provider refused to start — return JSON error (not SSE)
        return writeJSONError(w, http.StatusBadGateway, "provider_error", err.Error())
    }

    // 4. Stream tokens with error handling
    for {
        select {
        case <-ctx.Done():
            // Client disconnected — clean up silently
            return ctx.Err()
        case chunk, ok := <-stream:
            if !ok {
                // Stream completed normally
                fmt.Fprintf(w, "data: [DONE]\n\n")
                flusher.Flush()
                return nil
            }
            if chunk.Err != nil {
                // MID-STREAM ERROR: Provider died
                // Send error event over SSE, then close
                errEvent := types.ChatCompletionChunk{
                    // ... populate with error finish_reason
                    Choices: []types.ChunkChoice{{
                        FinishReason: ptr("error"),
                        Delta: types.Delta{Content: ptr("")},
                    }},
                }
                writeSSEEvent(w, flusher, errEvent)
                fmt.Fprintf(w, "data: [DONE]\n\n")
                flusher.Flush()
                // Save partial response to DB so it's not lost
                p.store.SavePartialResponse(ctx, req.SessionID, chunk.ContentSoFar)
                return chunk.Err
            }
            writeSSEEvent(w, flusher, chunk.Data)
            flusher.Flush()
        }
    }
}
```

**Key requirements:**
1. **Save partial responses.** If 500 tokens were streamed before failure, persist them.
2. **Send an SSE error event** before `[DONE]` so the client knows what happened.
3. **Implement retry at the provider level** with exponential backoff (configurable).
4. **Circuit breaker pattern:** If a provider fails 3 times in 60 seconds, mark it unhealthy and fall back to the next configured provider (if any).

### 3.2 Tool Execution Timeouts

The plan specifies `"timeout": 10000` (10s) in the tool contract but doesn't describe enforcement.

```go
// internal/tools/executor.go

func (e *Executor) Execute(ctx context.Context, call types.ToolCall, manifest *ToolManifest) (*ToolResult, error) {
    timeout := manifest.Timeout
    if timeout == 0 {
        timeout = 10 * time.Second // default
    }
    if timeout > e.maxTimeout {
        timeout = e.maxTimeout // hard ceiling (e.g., 60s)
    }

    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    var result ToolResult
    done := make(chan error, 1)

    go func() {
        var err error
        result, err = e.sandbox.Run(ctx, call, manifest)
        done <- err
    }()

    select {
    case err := <-done:
        if err != nil {
            return &ToolResult{
                Status: "error",
                Output: fmt.Sprintf("Tool execution failed: %v", err),
            }, nil // Return error as tool result, not Go error
        }
        // Truncate output if too large (prevent context window blowup)
        if len(result.Output) > e.maxOutputBytes {
            result.Output = result.Output[:e.maxOutputBytes] + "\n...[truncated]"
            result.Truncated = true
        }
        return &result, nil
    case <-ctx.Done():
        // Kill the process group
        e.sandbox.Kill(call.ID)
        return &ToolResult{
            Status:  "timeout",
            Output:  fmt.Sprintf("Tool execution timed out after %s", timeout),
        }, nil
    }
}
```

**Key requirements:**
1. **Timeouts must be enforced with `context.WithTimeout`, not `time.After`** — the context cancels the child process.
2. **Tool output must be truncated** before being sent back into the LLM context. A rogue `cat /dev/urandom` could produce unlimited output.
3. **Return tool errors as tool results**, not as Go errors. The LLM should see "Tool failed: X" and adjust its response. Only return Go errors for infrastructure failures.

### 3.3 Concurrent Session Management

The plan doesn't address concurrency at all. Multiple users hitting `/v1/chat/completions` simultaneously must not corrupt shared state.

```go
// internal/context/manager.go

type Manager struct {
    store    store.Store
    mu       sync.Mutex // NOT per-session — see below
    sessions map[string]*sync.Mutex // Per-session locks
}

func (m *Manager) getSessionLock(sessionID string) *sync.Mutex {
    m.mu.Lock()
    defer m.mu.Unlock()
    if _, ok := m.sessions[sessionID]; !ok {
        m.sessions[sessionID] = &sync.Mutex{}
    }
    return m.sessions[sessionID]
}

func (m *Manager) ProcessMessage(ctx context.Context, sessionID string, msg types.ChatMessage) error {
    // Per-session mutex: two requests to the same session are serialized
    // Requests to different sessions proceed concurrently
    lock := m.getSessionLock(sessionID)
    lock.Lock()
    defer lock.Unlock()

    // ... assemble context, call inference, handle tools, save result
}
```

**Alternative (better): Use a channel-based actor per session.**
Each session gets a goroutine that processes messages sequentially from a channel. This eliminates lock contention entirely and is more idiomatic Go.

### 3.4 Database Migration Strategy

The plan doesn't mention migrations at all.

```go
// internal/store/migrations.go

import "embed"

//go:embed migrations/*.sql
var migrationFS embed.FS

func (s *SQLiteStore) Migrate(ctx context.Context) error {
    entries, _ := migrationFS.ReadDir("migrations")
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Name() < entries[j].Name()
    })

    // Create migrations table if not exists
    s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )`)

    for _, entry := range entries {
        version := entry.Name()
        var exists int
        s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM _migrations WHERE version = ?", version).Scan(&exists)
        if exists > 0 {
            continue
        }

        sql, _ := migrationFS.ReadFile("migrations/" + version)
        if _, err := s.db.ExecContext(ctx, string(sql)); err != nil {
            return fmt.Errorf("migration %s failed: %w", version, err)
        }
        s.db.ExecContext(ctx, "INSERT INTO _migrations (version) VALUES (?)", version)
    }
    return nil
}
```

Alternatively, use `github.com/pressly/goose/v3` with embedded migrations. But for a project that prides itself on single-binary simplicity, a hand-rolled migrator (as above) is ~50 lines and avoids a dependency.

### 3.5 Graceful Shutdown

Not mentioned in the plan. Critical for a production server.

```go
// cmd/forge/main.go

func main() {
    cfg := config.Load()
    srv := server.New(cfg)

    // Start server in background
    go func() {
        log.Info().Str("addr", cfg.Addr).Msg("Forge started")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal().Err(err).Msg("server error")
        }
    }()

    // Wait for interrupt
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Info().Msg("Shutting down gracefully...")

    // Phase 1: Stop accepting new connections (5s deadline)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    srv.Shutdown(ctx)

    // Phase 2: Wait for in-flight streams to complete (30s deadline)
    ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel2()
    srv.DrainStreams(ctx2)

    // Phase 3: Close database connections
    srv.CloseStore()

    log.Info().Msg("Forge stopped")
}
```

**Key requirements:**
1. **Two-phase shutdown**: stop new requests, then drain in-flight streams.
2. **Active SSE streams must be signaled** to send `[DONE]` and close cleanly.
3. **Database WAL checkpoint** should be triggered before closing SQLite.

---

## 4. Testing Strategy

### 4.1 Unit Tests

Each module should have isolated unit tests. Structure mirrors the implementation:

```
internal/
├── inference/
│   ├── ollama_test.go        # Test request/response translation (mock HTTP)
│   └── registry_test.go     # Test provider selection logic
├── context/
│   ├── manager_test.go       # Test context assembly, token budgeting
│   ├── tokenizer_test.go     # Test token counts against known values
│   └── compaction_test.go    # Test summarization trigger + message preservation
├── tools/
│   ├── executor_test.go      # Test timeout, truncation, error handling
│   ├── manifest_test.go      # Test YAML/JSON loading
│   └── interceptor_test.go   # Test pause-execute-resume flow
├── store/
│   ├── sqlite_test.go        # Test CRUD + migrations (uses :memory:)
│   └── store_test.go         # Interface compliance tests (run for all impls)
└── streaming/
    ├── sse_test.go            # Test SSE encoding edge cases
    └── pipeline_test.go       # Test backpressure, disconnect, error mid-stream
```

**Pattern: Interface compliance testing.**

```go
// internal/store/store_test.go

// StoreTestSuite runs the same tests against any Store implementation
func StoreTestSuite(t *testing.T, newStore func() store.Store) {
    t.Run("CreateSession", func(t *testing.T) {
        s := newStore()
        defer s.Close()
        sess, err := s.CreateSession(context.Background(), "test-model")
        require.NoError(t, err)
        require.NotEmpty(t, sess.ID)
    })
    // ... more tests
}

func TestSQLiteStore(t *testing.T) {
    StoreTestSuite(t, func() store.Store {
        s, _ := sqlite.New(":memory:")
        return s
    })
}

func TestPostgresStore(t *testing.T) {
    if os.Getenv("TEST_PG_URL") == "" {
        t.Skip("TEST_PG_URL not set")
    }
    StoreTestSuite(t, func() store.Store {
        s, _ := postgres.New(os.Getenv("TEST_PG_URL"))
        return s
    })
}
```

### 4.2 Integration Tests for Streaming Pipeline

```go
// internal/streaming/pipeline_integration_test.go

func TestStreamingPipelineEndToEnd(t *testing.T) {
    // Set up mock provider that streams 10 tokens then completes
    mock := inference.NewMock([]string{"Hello", " ", "world", "!"})

    // Set up real HTTP server
    srv := httptest.NewServer(api.NewRouter(mock, store.NewMemory()))
    defer srv.Close()

    // Make streaming request
    body := `{"model":"mock","messages":[{"role":"user","content":"hi"}],"stream":true}`
    resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
    require.NoError(t, err)
    defer resp.Body.Close()

    require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

    // Read all SSE events
    scanner := bufio.NewScanner(resp.Body)
    var tokens []string
    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") { continue }
        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" { break }

        var chunk types.ChatCompletionChunk
        require.NoError(t, json.Unmarshal([]byte(data), &chunk))
        if chunk.Choices[0].Delta.Content != nil {
            tokens = append(tokens, *chunk.Choices[0].Delta.Content)
        }
    }

    assert.Equal(t, []string{"Hello", " ", "world", "!"}, tokens)
}
```

### 4.3 Tool Execution Integration Test

```go
func TestToolInterceptor_PauseExecuteResume(t *testing.T) {
    // Mock provider: first call returns tool_call, second call returns text
    mock := inference.NewMockWithToolCalls(
        // Response 1: tool call
        types.ChatMessage{
            Role: "assistant",
            ToolCalls: []types.ToolCall{{
                ID:       "call_1",
                Type:     "function",
                Function: types.FunctionCall{Name: "echo", Arguments: `{"text":"hello"}`},
            }},
        },
        // Response 2: final text (after tool result injected)
        types.ChatMessage{
            Role:    "assistant",
            Content: "The echo tool returned: hello",
        },
    )

    // Register a tool that just echoes
    executor := tools.NewExecutor(tools.WithLocalSandbox())
    executor.Register("echo", tools.Manifest{
        Command: "echo",
        Args:    []string{"${text}"},
        Timeout: 5 * time.Second,
    })

    // ... run through the interceptor and assert the full conversation
}
```

### 4.4 E2E Tests for UI

Use Playwright for E2E testing against a running Forge instance with the mock provider.

```typescript
// ui/e2e/chat.spec.ts
import { test, expect } from '@playwright/test';

test('sends message and receives streaming response', async ({ page }) => {
  await page.goto('/chat');
  await page.fill('[data-testid="message-input"]', 'Hello Forge');
  await page.click('[data-testid="send-button"]');

  // Wait for streaming to complete
  await expect(page.locator('[data-testid="assistant-message"]')).toContainText('Hello world!');
});

test('displays tool execution progress', async ({ page }) => {
  await page.goto('/chat');
  await page.fill('[data-testid="message-input"]', 'Search for something');
  await page.click('[data-testid="send-button"]');

  // Tool progress card should appear
  await expect(page.locator('[data-testid="tool-progress"]')).toBeVisible();
  await expect(page.locator('[data-testid="tool-progress"]')).toContainText('Executing');

  // Then disappear and final answer appears
  await expect(page.locator('[data-testid="assistant-message"]')).toBeVisible();
});
```

### 4.5 Load Testing

```go
// test/load/concurrent_test.go

func TestConcurrentStreams(t *testing.T) {
    if testing.Short() { t.Skip("load test") }

    srv := setupTestServer(t)
    defer srv.Close()

    const numStreams = 50
    var wg sync.WaitGroup
    errors := make(chan error, numStreams)

    for i := 0; i < numStreams; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            err := makeStreamingRequest(srv.URL, fmt.Sprintf("session-%d", id))
            if err != nil {
                errors <- fmt.Errorf("stream %d: %w", id, err)
            }
        }(i)
    }

    wg.Wait()
    close(errors)

    for err := range errors {
        t.Error(err)
    }
}
```

### 4.6 Mock Provider for CI/CD

The plan mentions a `MockInferenceProvider` — it should be first-class:

```go
// internal/inference/mock.go

type MockProvider struct {
    tokens   []string
    delay    time.Duration // Per-token delay (simulate real streaming)
    failAt   int           // Fail after N tokens (-1 = never)
    toolCall *types.ToolCall // If set, return a tool call
}

func NewMock(tokens []string) *MockProvider {
    return &MockProvider{tokens: tokens, delay: 10 * time.Millisecond, failAt: -1}
}

func (m *MockProvider) StreamCompletion(ctx context.Context, req *types.ChatCompletionRequest) (<-chan StreamChunk, error) {
    ch := make(chan StreamChunk)
    go func() {
        defer close(ch)
        for i, token := range m.tokens {
            if m.failAt == i {
                ch <- StreamChunk{Err: errors.New("mock provider failure")}
                return
            }
            select {
            case <-ctx.Done():
                return
            case <-time.After(m.delay):
                ch <- StreamChunk{Data: token}
            }
        }
    }()
    return ch, nil
}
```

---

## 5. Performance Considerations

### 5.1 Connection Pooling for LLM Providers

Each provider implementation should use a shared `http.Client` with connection pooling — **not** create a new client per request.

```go
// internal/inference/ollama.go

type OllamaProvider struct {
    client  *http.Client
    baseURL string
}

func NewOllama(baseURL string) *OllamaProvider {
    return &OllamaProvider{
        baseURL: baseURL,
        client: &http.Client{
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 20,
                IdleConnTimeout:     90 * time.Second,
                // Don't set a global Timeout here — we use per-request context
            },
            // No Timeout field — streaming responses can be long-lived
        },
    }
}
```

> **⚠️ Do NOT set `http.Client.Timeout` for streaming providers.** A streaming response can take minutes. Use `context.Context` for cancellation instead.

### 5.2 SQLite WAL Mode and Concurrency

SQLite in WAL mode supports concurrent reads with a single writer. Critical for Forge:

```go
// internal/store/sqlite.go

func NewSQLite(path string) (*SQLiteStore, error) {
    // Key pragmas for production use
    dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-20000&_foreign_keys=ON"

    db, err := sql.Open("sqlite", dsn) // modernc.org/sqlite driver
    if err != nil {
        return nil, err
    }

    // SQLite allows multiple readers but ONE writer.
    // Set max open connections to allow concurrent reads.
    db.SetMaxOpenConns(1)   // IMPORTANT: Only 1 writer at a time prevents SQLITE_BUSY
    // Actually — for WAL mode, we can be smarter:
    // Use separate pools for reads and writes.

    return &SQLiteStore{db: db}, nil
}
```

**Better pattern: Separate read and write connections.**

```go
type SQLiteStore struct {
    writer *sql.DB // MaxOpenConns=1
    reader *sql.DB // MaxOpenConns=runtime.NumCPU()
}
```

This is a well-known SQLite optimization. Reads never block writes (WAL), and the single-writer constraint prevents lock contention.

### 5.3 Frontend Bundle Size Optimization

The binary embeds the UI assets. Every KB counts.

1. **Vite config: enable compression and tree-shaking.**

```typescript
// ui/vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    target: 'es2020',
    minify: 'esbuild',
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom'],
          'markdown': ['react-markdown', 'rehype-highlight'],
        },
      },
    },
  },
});
```

2. **Serve with gzip/brotli compression from Go.** Embed the raw files and compress on-the-fly (or pre-compress at build time):

```go
// internal/server/static.go

func serveUI(uiFS fs.FS) http.Handler {
    fileServer := http.FileServer(http.FS(uiFS))
    return gziphandler.GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // SPA fallback: if file doesn't exist, serve index.html
        path := r.URL.Path
        if f, err := uiFS.Open(strings.TrimPrefix(path, "/")); err == nil {
            f.Close()
            fileServer.ServeHTTP(w, r)
        } else {
            r.URL.Path = "/"
            fileServer.ServeHTTP(w, r)
        }
    }))
}
```

3. **Target bundle size: < 250KB gzipped** for the chat UI. React + Tailwind + Markdown can achieve this.

### 5.4 Memory Management for Long-Running Sessions

Compaction is the plan's answer, but it's not enough. Additional safeguards:

1. **Limit in-memory session cache.** Use an LRU cache with a configurable max size.

```go
type SessionCache struct {
    cache *lru.Cache[string, *Session] // e.g., hashicorp/golang-lru
    store store.Store
}

func NewSessionCache(maxSessions int, store store.Store) *SessionCache {
    cache, _ := lru.New[string, *Session](maxSessions)
    return &SessionCache{cache: cache, store: store}
}
```

2. **Stream chunks should NOT be accumulated in memory.** Write to the client and the database simultaneously — don't buffer the full response.

3. **Set hard limits on message size** (e.g., 100KB per user message) to prevent memory bombs.

---

## 6. Security Review

### 6.1 Input Sanitization for Tool Execution (CRITICAL)

The plan's tool contract passes user-influenced arguments directly to `os/exec`. This is a **command injection vector**.

```json
{"tool": "terminal_exec", "arguments": {"command": "grep -r 'todo' ."}}
```

If the LLM is tricked into passing `"command": "grep -r 'todo' . ; rm -rf /"`, the sandbox executes it.

**Mitigations:**

```go
// internal/tools/sandbox_local.go

func (s *LocalSandbox) Run(ctx context.Context, call types.ToolCall, manifest *ToolManifest) (ToolResult, error) {
    // NEVER use shell execution. NEVER pass to sh -c.
    // Always use exec.CommandContext with explicit argv.

    args := manifest.ResolveArgs(call.Arguments)

    // Validate against allowlist
    if !s.isAllowedCommand(manifest.Command) {
        return ToolResult{Status: "error", Output: "command not in allowlist"}, nil
    }

    cmd := exec.CommandContext(ctx, manifest.Command, args...)

    // Security hardening:
    cmd.Dir = s.workDir                    // Restrict working directory
    cmd.Env = s.sanitizedEnv()             // Don't inherit all env vars
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,                     // New process group (for killing)
    }

    // Capture output with size limit
    var stdout, stderr bytes.Buffer
    cmd.Stdout = io.LimitWriter(&stdout, int64(s.maxOutput))
    cmd.Stderr = io.LimitWriter(&stderr, int64(s.maxOutput))

    err := cmd.Run()
    // ...
}
```

**Key rules:**
1. **Never use `sh -c`** — always `exec.CommandContext(ctx, binary, arg1, arg2, ...)`.
2. **Allowlist commands** in the tool manifest. Don't allow arbitrary binary execution.
3. **Restrict environment variables** — don't leak `FORGE_API_KEY` to tool subprocesses.
4. **Set `Setpgid: true`** so you can kill the entire process group on timeout.
5. **For hosted mode: Docker is mandatory.** Use `--network=none --read-only --memory=128m --cpus=0.5`.

### 6.2 CORS Configuration

```go
// internal/server/middleware.go

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
    return cors.Handler(cors.Options{
        AllowedOrigins:   allowedOrigins, // NOT "*" in production
        AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Content-Type", "Authorization", "Accept"},
        ExposedHeaders:   []string{"X-Request-ID"},
        AllowCredentials: true,
        MaxAge:           300,
    })
}
```

**For local mode:** Allow `*` (any origin) since it's a local dev tool.  
**For hosted mode:** Require explicit `FORGE_CORS_ORIGINS` configuration.  
**The plan doesn't mention CORS at all** — this is a gap.

### 6.3 Rate Limiting

Not mentioned in the plan. Essential for hosted mode.

```go
// internal/auth/ratelimit.go

func RateLimitMiddleware(rps int) func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(rps), rps*2) // burst = 2x rate
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                http.Error(w, `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`,
                    http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

For production: per-key rate limiting using a token bucket per API key.

### 6.4 Session Token Management

The plan mentions `FORGE_API_KEY` for API auth but doesn't address UI sessions.

**Recommendation for the Chat UI:**
- In **local mode**: No auth (it's a local tool on localhost).
- In **hosted mode**: Issue a session JWT on login, store in `httpOnly` cookie (not localStorage).
- **Never expose API keys to the frontend.** The Chat UI should use session cookies; only programmatic API access uses bearer tokens.

### 6.5 Content Security Policy

The embedded UI needs a strict CSP to prevent XSS:

```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; "+
            "script-src 'self'; "+
            "style-src 'self' 'unsafe-inline'; "+  // Tailwind may need inline styles
            "connect-src 'self' ws: wss:; "+        // Allow WebSocket connections
            "img-src 'self' data:; "+
            "font-src 'self'")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        next.ServeHTTP(w, r)
    })
}
```

---

## 7. Implementation Gaps

### 7.1 Logging and Observability

The plan has **zero** mention of logging. This is a significant gap.

**Recommendation: Structured logging with `zerolog`.**

```go
// internal/config/logging.go

func SetupLogger(cfg *Config) zerolog.Logger {
    var output io.Writer = os.Stderr

    if cfg.LogFormat == "pretty" {
        output = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
    }

    level, _ := zerolog.ParseLevel(cfg.LogLevel)
    return zerolog.New(output).
        Level(level).
        With().
        Timestamp().
        Str("service", "forge").
        Str("version", cfg.Version).
        Logger()
}
```

**Every HTTP request should be logged:**

```go
func requestLogger(log zerolog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            defer func() {
                log.Info().
                    Str("method", r.Method).
                    Str("path", r.URL.Path).
                    Int("status", ww.Status()).
                    Dur("duration", time.Since(start)).
                    Int("bytes", ww.BytesWritten()).
                    Msg("request")
            }()

            next.ServeHTTP(ww, r)
        })
    }
}
```

**Metrics to expose (via `/api/metrics` or Prometheus):**
- Active SSE streams count
- Token throughput (tokens/sec per stream)
- Provider latency histogram
- Tool execution duration histogram
- Context window utilization (% of max tokens used)
- Compaction events count

### 7.2 Configuration Management

The plan mentions `DATABASE_URL` and `FORGE_API_KEY` as env vars but doesn't define a configuration strategy.

**Recommendation: Layered config with env vars as primary, CLI flags as overrides.**

```go
// internal/config/config.go

type Config struct {
    // Server
    Addr     string `env:"FORGE_ADDR" envDefault:":8080"`
    DevMode  bool   `env:"FORGE_DEV" envDefault:"false"`
    Version  string // Set via ldflags

    // Database
    DatabaseURL string `env:"DATABASE_URL"` // If set → PostgreSQL; if empty → SQLite
    SQLitePath  string `env:"FORGE_DB_PATH" envDefault:"forge.db"`

    // Auth
    APIKey string `env:"FORGE_API_KEY"` // If set → require auth; if empty → no auth

    // Inference
    DefaultProvider string `env:"FORGE_PROVIDER" envDefault:"ollama"`
    OllamaURL       string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
    OpenAIKey       string `env:"OPENAI_API_KEY"`
    AnthropicKey    string `env:"ANTHROPIC_API_KEY"`
    DefaultModel    string `env:"FORGE_MODEL" envDefault:"llama3"`

    // Limits
    MaxToolTimeout  time.Duration `env:"FORGE_MAX_TOOL_TIMEOUT" envDefault:"60s"`
    MaxOutputBytes  int           `env:"FORGE_MAX_TOOL_OUTPUT" envDefault:"65536"` // 64KB
    MaxMessageSize  int           `env:"FORGE_MAX_MESSAGE_SIZE" envDefault:"102400"` // 100KB

    // Logging
    LogLevel  string `env:"FORGE_LOG_LEVEL" envDefault:"info"`
    LogFormat string `env:"FORGE_LOG_FORMAT" envDefault:"json"` // "json" | "pretty"

    // CORS
    CORSOrigins []string `env:"FORGE_CORS_ORIGINS" envSeparator:"," envDefault:"*"`
}

func Load() (*Config, error) {
    cfg := &Config{}
    if err := env.Parse(cfg); err != nil {
        return nil, fmt.Errorf("parsing config: %w", err)
    }
    return cfg, nil
}
```

### 7.3 Health Check Endpoints

```go
// GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    health := map[string]any{
        "status":  "ok",
        "version": s.cfg.Version,
        "uptime":  time.Since(s.startTime).String(),
        "checks":  map[string]any{},
    }

    checks := health["checks"].(map[string]any)

    // Database check
    if err := s.store.Ping(r.Context()); err != nil {
        checks["database"] = map[string]string{"status": "error", "error": err.Error()}
        health["status"] = "degraded"
    } else {
        checks["database"] = map[string]string{"status": "ok"}
    }

    // Provider check
    if err := s.provider.Ping(r.Context()); err != nil {
        checks["inference"] = map[string]string{"status": "error", "error": err.Error()}
        health["status"] = "degraded"
    } else {
        checks["inference"] = map[string]string{"status": "ok"}
    }

    status := http.StatusOK
    if health["status"] != "ok" {
        status = http.StatusServiceUnavailable
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(health)
}
```

### 7.4 Graceful Degradation

The plan doesn't address what happens when optional features are unavailable.

| Scenario | Behavior |
|---|---|
| No inference provider reachable | Return 503 on `/v1/chat/completions`, serve UI with error banner |
| SQLite DB locked/corrupted | Fall back to in-memory store, log CRITICAL warning |
| Token counter unavailable | Fall back to character-based estimation (1 token ≈ 4 chars) |
| Tool execution disabled | Strip `tools` from provider request, respond without tool use |
| Frontend assets missing | Serve a plain-text fallback: "Forge API is running. UI unavailable." |

---

## 8. Summary of Action Items

### 🔴 Critical (Must Fix Before Phase 1)

| # | Issue | Section |
|---|---|---|
| 1 | **CGO conflict**: `CGO_ENABLED=0` + `mattn/go-sqlite3` are incompatible. Decide on `modernc.org/sqlite` or accept CGO | §1.3 |
| 2 | **Command injection**: Tool executor must NEVER use `sh -c`. Enforce allowlist + argv-only execution | §6.1 |
| 3 | **No graceful shutdown**: Server must handle SIGTERM, drain streams, checkpoint DB | §3.5 |
| 4 | **No concurrency model**: Concurrent requests to the same session will corrupt state | §3.3 |
| 5 | **No `ui/dist` build tag**: `go test` will fail without Node.js installed | §1.2 |

### 🟡 High (Must Fix Before Phase 3)

| # | Issue | Section |
|---|---|---|
| 6 | Add `/v1/models` endpoint — OpenAI SDKs require it | §2.1 |
| 7 | Define Forge-native API alongside OpenAI-compatible API | §2.2 |
| 8 | Mid-stream error handling: save partial responses, send SSE error events | §3.1 |
| 9 | Tool output truncation to prevent context window overflow | §3.2 |
| 10 | Structured logging (zerolog or slog) — no logging = no debugging | §7.1 |
| 11 | Configuration management — define all env vars, defaults, validation | §7.2 |
| 12 | Health check endpoint with provider + DB status | §7.3 |
| 13 | CORS configuration — not mentioned, will block browser requests | §6.2 |
| 14 | CSP headers for embedded UI — prevent XSS in rendered markdown | §6.5 |
| 15 | WebSocket heartbeat — detect dead connections behind proxies | §2.3 |
| 16 | Use typed event enums, not stringly-typed event names | §2.3 |

### 🟢 Medium (Should Fix Before Phase 5)

| # | Issue | Section |
|---|---|---|
| 17 | Database migration strategy | §3.4 |
| 18 | Connection pooling for LLM providers | §5.1 |
| 19 | SQLite WAL mode + separate read/write connections | §5.2 |
| 20 | Frontend bundle size target (< 250KB gzipped) | §5.3 |
| 21 | LRU session cache to bound memory | §5.4 |
| 22 | Rate limiting middleware for hosted mode | §6.3 |
| 23 | Session token management (JWT cookies for UI) | §6.4 |
| 24 | Interface compliance tests (run same suite for SQLite + Postgres) | §4.1 |
| 25 | Load testing: 50 concurrent streams | §4.5 |
| 26 | Graceful degradation matrix | §7.4 |
| 27 | Provider circuit breaker pattern | §3.1 |
| 28 | `http.Client.Timeout` must NOT be set for streaming providers | §5.1 |
| 29 | Process group management for tool timeouts (`Setpgid`) | §6.1 |
| 30 | Restrict tool subprocess environment variables | §6.1 |

### 🔵 Low (Nice to Have)

| # | Issue | Section |
|---|---|---|
| 31 | SSE `id:` fields for resume-on-reconnect | §2.3 |
| 32 | `/api/metrics` endpoint for observability | §7.1 |
| 33 | Pre-compressed (brotli) static assets | §5.3 |
| 34 | E2E tests with Playwright | §4.4 |
| 35 | API key per-key rate limiting | §6.3 |
| 36 | Makefile dev mode (concurrent UI + Go server) | §1.2 |
| 37 | `Accept: version=N` header for Forge API versioning | §2.4 |

---

## Final Verdict

The Implementation Plan is a **solid vision document** with the right technical instincts (Go, embed, chi, SSE, SQLite). But it reads like an architecture overview, not an implementation plan. The gaps identified above — especially around concurrency safety, tool security, error handling, and observability — need to be resolved in a detailed design phase before writing production code.

**Recommended next step:** Expand Phase 1 to include items #1–5, #10–11, and #13 from the action items above. The skeleton must include graceful shutdown, structured logging, configuration management, and the concurrency model from day one. Retrofitting these is painful.

The streaming pipeline (§3.1), tool interceptor (§3.2), and the full `Provider` interface are the hardest engineering challenges. Each deserves its own focused design document with sequence diagrams before implementation begins.
