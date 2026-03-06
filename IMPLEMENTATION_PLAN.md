# 🛠️ Forge: Implementation Plan (v2)

> **Synthesized from 5 expert reviews:** Product Manager, UI/UX Design, Senior Developer, LLM Engine Architect, Module Architect.
> **Last updated:** 2025-07-15

---

## 1. Vision & Differentiators

**Forge** is a unified AI backend and frontend delivered as a **single static binary** written in **Go**. It abstracts LLM inference, manages stateful conversation context, and provides a secure sandbox for tool execution.

> *Forge is the fastest way to self-host a multi-model AI chat. Download one binary. Run it. You're chatting. No Docker. No databases to manage. No YAML files. Just `./forge`.*

### The Three Pillars

1. **Zero-dependency deployment** — Single static binary, SQLite embedded, frontend embedded.
2. **Inspector UI** — See exactly what the model sees. Token counts, raw context, tool execution traces. No competitor has this.
3. **Built-in tool execution** — First-class Pause-Execute-Resume tool loop with sandboxing.

### The "Single Binary" Strategy

| Layer | Technology | Notes |
|:---|:---|:---|
| **Backend** | Go (Golang) | High-concurrency streaming, networking |
| **Frontend** | React 19 + Tailwind CSS v4 + Lucide Icons | Compiled & embedded via `go:embed` |
| **Database** | SQLite (local) / PostgreSQL (hosted) | Unified `Store` interface |
| **SQLite Driver** | **`modernc.org/sqlite`** (pure Go) | Enables `CGO_ENABLED=0` static binaries |

### Design Principles

1. **Zero Config Start** — `./forge` must work with zero flags if Ollama is running locally.
2. **No Telemetry, Ever** — Explicit promise. Self-hosters choose Forge because they value privacy.
3. **Progressive Disclosure** — Simple by default, powerful when needed.
4. **Inspector as Superpower** — First-class citizen, not an afterthought.
5. **Tools as the Only Extension Point** — One extension system, not three.

---

## 2. System Architecture (10 Modules)

### 2.1 Module Map

```
┌─────────────────────────────────────────────────────────────────────┐
│                         FORGE BINARY                                │
│                                                                     │
│  ┌──────────┐                                                       │
│  │  Config   │◄──── env vars, forge.yaml, CLI flags                 │
│  └────┬─────┘                                                       │
│       │ injected into all modules at startup                        │
│       ▼                                                             │
│  ┌──────────┐    ┌──────────────┐    ┌───────────────┐              │
│  │   Auth    │◄───│  API Gateway  │───►│  Web Server   │              │
│  │Middleware │    │ (chi router)  │    │ (go:embed fs) │              │
│  └──────────┘    └──────┬───────┘    └───────────────┘              │
│                         │                                           │
│              ┌──────────┼──────────┐                                │
│              ▼          ▼          ▼                                │
│  ┌───────────────┐ ┌─────────┐ ┌──────────────┐                    │
│  │    Session     │ │  Model  │ │  Event Bus   │◄──── WebSocket /ws │
│  │    Manager     │ │Registry │ │  (pub/sub)   │                    │
│  └───────┬───────┘ └────┬────┘ └──────┬───────┘                    │
│          │              │             │                              │
│          ▼              ▼             │                              │
│  ┌───────────────────────────────┐    │                              │
│  │     Inference Orchestrator     │◄───┘                              │
│  │  (state machine / main loop)  │                                  │
│  └──────────┬────────────────────┘                                  │
│             │                                                       │
│    ┌────────┼────────┐                                              │
│    ▼        ▼        ▼                                              │
│ ┌───────┐ ┌──────┐ ┌──────────────┐                                │
│ │Context│ │ Tool │ │  Compaction   │                                │
│ │Window │ │Sandbox│ │   Engine     │                                │
│ └───────┘ └──────┘ └──────────────┘                                │
│                                                                     │
│  ┌──────────────────────────────────┐                               │
│  │     Storage Layer (SQLite/PG)    │                               │
│  └──────────────────────────────────┘                               │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 Module Responsibility Table

| # | Module | Responsibility | Key Technology |
|:--|:---|:---|:---|
| 1 | **Config** | Centralized configuration (env vars, `forge.yaml`, CLI flags). All modules receive config at init. | `caarlos0/env` |
| 2 | **Auth Middleware** | API key auth (bearer token), UI session management (httpOnly cookie). No-op in local mode. | `net/http` middleware |
| 3 | **API Gateway** | OpenAI-compatible `/v1/` REST API + SSE streaming. Forge-native `/api/` endpoints. | `chi` router, SSE |
| 4 | **Web Server** | Serves embedded Chat UI (`/chat`) and Inspector (`/inspector`). SPA fallback routing. | `go:embed`, `fs.FS` |
| 5 | **Session Manager** | Session CRUD lifecycle (create, list, delete, expire, archive). Owns session state transitions. | SQLite/PG |
| 6 | **Model Registry** | Discovers, catalogs, and routes to providers. Powers `/v1/models` and model selector UI. | Provider polling |
| 7 | **Event Bus** | Pub/sub for internal events. Fans out to WebSocket clients. Has no domain knowledge. | WebSocket, channels |
| 8 | **Inference Orchestrator** | The main loop state machine: validate → build context → [compact] → infer → [tools] → complete. | Go channels |
| 9 | **Context Window** | Context assembly, token counting, system prompt reservation. Per-model budget calculation. | `tiktoken-go` + fallbacks |
| 10 | **Tool Sandbox** | Receives tool calls, executes in sandbox (local `os/exec` or Docker), returns results. | `os/exec`, Docker API |
| 11 | **Compaction Engine** | Rolling summarization of conversation history when context limits are approached. | Background LLM worker |
| 12 | **Storage Layer** | Unified persistence interface. SQLite (default) or PostgreSQL (via `DATABASE_URL`). Migrations. | `modernc.org/sqlite`, `pgx` |

### 2.3 Dependency Rules

1. **No circular dependencies.** The module graph is a DAG.
2. **Config is the root.** Every module receives its config at construction time — never reads env vars directly.
3. **Storage Layer is a leaf.** Depends on nothing except Config.
4. **Event Bus has no domain knowledge.** Routes typed events, never inspects payloads.
5. **Every DB record has a `user_id` field from Day 1** (hardcoded to `"default"` in v1 — enables v2 multi-user without migration).

---

## 3. Go Project Layout

```
forge/
├── cmd/
│   └── forge/
│       └── main.go              # Entry point: flag parsing, DI wiring, graceful shutdown
├── internal/
│   ├── server/
│   │   ├── server.go            # HTTP server setup, middleware chain, graceful shutdown
│   │   └── routes.go            # Route registration (chi mux wiring)
│   ├── api/
│   │   ├── v1/
│   │   │   ├── chat.go          # POST /v1/chat/completions handler
│   │   │   └── models.go        # GET  /v1/models handler
│   │   └── forge/
│   │       ├── sessions.go      # Forge-native session CRUD API
│   │       ├── tools.go         # Forge-native tool management API
│   │       └── events.go        # WebSocket /ws event hub handler
│   ├── inference/
│   │   ├── provider.go          # InferenceProvider interface definition
│   │   ├── registry.go          # Provider registry + model catalog
│   │   ├── orchestrator.go      # Inference state machine
│   │   ├── ollama.go            # Ollama provider
│   │   ├── openai.go            # OpenAI API provider
│   │   ├── anthropic.go         # Anthropic API provider
│   │   ├── gemini.go            # Google Gemini provider
│   │   ├── openai_compat.go     # Generic OpenAI-compatible provider
│   │   └── mock.go              # Mock provider for testing
│   ├── streaming/
│   │   ├── sse.go               # SSE encoder/writer with flush control
│   │   ├── pipeline.go          # Stream pipeline: intercept, buffer, fan-out
│   │   └── backpressure.go      # Backpressure + client disconnect detection
│   ├── context/
│   │   ├── manager.go           # Context window assembly + token budget
│   │   ├── tokenizer.go         # Token counting abstraction (per-provider)
│   │   └── compaction.go        # Rolling compaction algorithm
│   ├── tools/
│   │   ├── executor.go          # Tool execution orchestrator
│   │   ├── sandbox_local.go     # Local os/exec sandbox (argv-only, no sh -c)
│   │   ├── sandbox_docker.go    # Docker container sandbox
│   │   ├── manifest.go          # Tool manifest loader (JSON/YAML)
│   │   └── interceptor.go       # Stream interceptor: pause-execute-resume
│   ├── store/
│   │   ├── store.go             # Store interface (sessions, messages)
│   │   ├── sqlite.go            # SQLite implementation (modernc.org/sqlite)
│   │   ├── postgres.go          # PostgreSQL implementation
│   │   └── migrations/
│   │       ├── 001_init.sql
│   │       └── ...
│   ├── auth/
│   │   ├── middleware.go        # API key auth middleware
│   │   └── session.go           # UI session token management
│   ├── events/
│   │   └── bus.go               # In-process pub/sub event bus
│   └── config/
│       └── config.go            # Unified configuration (env + flags + file)
├── pkg/
│   └── types/
│       ├── openai.go            # OpenAI-compatible request/response types
│       ├── events.go            # Stream event types (typed enums)
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
│   │   │   ├── chat/
│   │   │   │   ├── MessageThread.tsx
│   │   │   │   ├── StreamingRenderer.tsx
│   │   │   │   ├── CodeBlock.tsx
│   │   │   │   ├── ToolCard.tsx
│   │   │   │   ├── Composer.tsx
│   │   │   │   └── EmptyState.tsx
│   │   │   ├── inspector/
│   │   │   ├── sidebar/
│   │   │   ├── settings/
│   │   │   └── shared/
│   │   ├── hooks/
│   │   │   ├── useSSE.ts
│   │   │   └── useWebSocket.ts
│   │   └── lib/
│   └── dist/                    # Build output (gitignored, embedded at compile)
├── embed.go                     # //go:embed ui/dist directive
├── go.mod
├── go.sum
├── Makefile                     # Build orchestration (ui + go)
├── Dockerfile
└── docker-compose.yml
```

### Build Pipeline (Makefile)

```makefile
.PHONY: all build dev clean test

all: build

build: ui go

ui:
	cd ui && npm ci && npm run build

go:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$$(git describe --tags --always)" \
		-o bin/forge ./cmd/forge

dev:
	cd ui && npm run dev &
	FORGE_DEV=true go run ./cmd/forge

test:
	@mkdir -p ui/dist && touch ui/dist/index.html
	go test ./... -race -count=1

clean:
	rm -rf bin/ ui/dist/ ui/node_modules/
```

---

## 4. Provider Architecture

### 4.1 The InferenceProvider Interface

```go
type InferenceProvider interface {
    StreamChat(ctx context.Context, req *ChatRequest, out chan<- StreamEvent) error
    Complete(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    CountTokens(messages []Message) (int, error)
    Capabilities(model string) ModelCapabilities
    ListModels(ctx context.Context) ([]ModelInfo, error)
    Name() string
}
```

### 4.2 StreamEvent Types (Discriminated Union)

```go
type StreamEventType int

const (
    EventContentDelta StreamEventType = iota
    EventToolCallStart
    EventToolCallDelta
    EventToolCallComplete
    EventError
    EventDone
    EventStatus
)

type StreamEvent struct {
    Type         StreamEventType
    Delta        string
    ToolCall     *ToolCallEvent
    Error        error
    FinishReason string          // "stop", "tool_calls", "length"
    Raw          json.RawMessage // original provider payload for debugging
}
```

### 4.3 Provider Support Plan

| Provider | Type | Phase | Notes |
|:---|:---|:---|:---|
| **Ollama** | Auto-detect dedicated | Phase 2 | Probe `localhost:11434` at startup. Primary local provider. |
| **OpenAI** | Dedicated | Phase 3 | GPT-4o, o3. Most users have a key. |
| **Anthropic** | Dedicated | Phase 3 | Claude. Different API format — needs dedicated provider. |
| **Google Gemini** | Dedicated | Phase 3 | Gemini 2.5 Pro. Different API format. |
| **Generic OpenAI-compatible** | Generic | Phase 3 | Covers Groq, Together, Mistral, OpenRouter, LM Studio, vLLM, LocalAI. |
| **llama.cpp** | Dedicated | Phase 5 | Direct llama-server without Ollama. |

**Key insight:** Only Anthropic and Gemini have meaningfully different APIs. Everyone else is OpenAI-compatible. The generic provider covers 80% of the landscape.

### 4.4 Streaming Format Normalization

Each provider has a different SSE format. Every provider's stream reader normalizes to the canonical `StreamEvent` type. The consumer (SSE writer) never knows which provider generated the event.

```
OpenAI:    data: {"choices":[{"delta":{"content":"Hello"}}]}
Anthropic: event: content_block_delta  →  data: {"delta":{"text":"Hello"}}
Ollama:    {"message":{"content":"Hello"},"done":false}    (NDJSON, not SSE!)
Gemini:    data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}
```

### 4.5 Per-Provider Infrastructure

Each provider gets: dedicated `http.Client` with connection pooling (no global `Timeout`), rate limiter (`rate.NewLimiter`), circuit breaker (3 failures → open for 30s), and retry with exponential backoff on `{429, 500, 502, 503, 504}`.

---

## 5. Orchestrator State Machine

The inference loop is formalized as a 6-state FSM with explicit transitions, timeouts, and error recovery.

```
                         ┌───────────────────┐
    Request arrives      │                   │
    ────────────────────►│   VALIDATING      │
                         │                   │
                         └────────┬──────────┘
                                  │
                         ┌────────▼──────────┐
                         │                   │
                         │  BUILDING_CONTEXT  │
                         │                   │
                         └────────┬──────────┘
                                  │
                          ┌───────┴───────┐
                          │               │
                 tokens < threshold    tokens >= threshold
                          │               │
                          │      ┌────────▼──────────┐
                          │      │    COMPACTING      │
                          │      └────────┬──────────┘
                          │               │
                          └───────┬───────┘
                                  │
                         ┌────────▼──────────┐
                         │                   │
                         │    INFERRING      │◄────────────────────┐
                         │                   │                     │
                         └────────┬──────────┘                     │
                                  │                                │
                          ┌───────┴───────┐                       │
                          │               │                       │
                   finish_reason     finish_reason               │
                     = "stop"       = "tool_calls"               │
                          │               │                       │
                          │      ┌────────▼──────────┐            │
                          │      │ EXECUTING_TOOLS   │────────────┘
                          │      └───────────────────┘  (loop back)
                          │
                 ┌────────▼──────────┐
                 │   COMPLETING      │
                 └───────────────────┘
```

| State | Max Duration | On Timeout | Recovery |
|:---|:---|:---|:---|
| `VALIDATING` | 1s | 400 Bad Request | Fatal |
| `BUILDING_CONTEXT` | 5s | 500 Internal | Fatal |
| `COMPACTING` | 60s | Skip compaction, truncate instead | Recoverable |
| `INFERRING` | 120s (configurable) | 504 Gateway Timeout, cancel LLM | Fatal — persist partial response |
| `EXECUTING_TOOLS` | Per-tool timeout (10s default) | Inject error as tool result | Recoverable — re-infer with error |
| `COMPLETING` | 5s | Log warning (response already sent) | Non-fatal |

**Safety limits:** `max_tool_rounds=10`, `max_tool_calls_per_round=5`, `max_total_tool_calls=25`. On limit exceeded, inject system message forcing a final answer.

---

## 6. Compaction Algorithm

### 6.1 Rolling Summary (Not Single-Shot)

The original plan's "summarize everything into one message" is too risky. Use an incremental rolling summary:

```
┌─────────────────────────────────────────────────────┐
│                    Context Window                    │
├─────────────┬──────────────────┬────────────────────┤
│ System      │ Rolling Summary  │ Recent Messages    │
│ Prompt      │ (compacted)      │ (sliding window)   │
│             │                  │                    │
│ FIXED       │ GROWS, then      │ FIFO — newest N    │
│ never       │ gets re-compacted│ messages, where N  │
│ compacted   │ into itself      │ is dynamically     │
│             │                  │ calculated         │
├─────────────┼──────────────────┼────────────────────┤
│ ~500 tokens │ ~1000-2000 tokens│ remainder of budget│
└─────────────┴──────────────────┴────────────────────┘
```

### 6.2 Key Design Decisions

| Decision | Value | Rationale |
|:---|:---|:---|
| **Trigger threshold** | 80% of (MaxContext − OutputReserve) | 90% is too tight — leaves no room for output tokens |
| **Summary model** | Cheap/fast model (`gpt-4o-mini` or local 8B) | Compaction is frequent and doesn't need creative intelligence |
| **Recent message count** | Dynamic (all pinned + last 3 turn pairs minimum) | Fixed-5 is fragile — may preserve garbage and discard important context |
| **Message pinning** | Auto-pin: tool results, user corrections, first user message, messages with code blocks | Prevents compaction of factual anchors |
| **Re-compaction** | When summary itself > 2000 tokens, summarize the summary | Enables theoretically infinite conversations |
| **Failure mode** | Non-fatal — continue with full context, try again next turn | Never lose a conversation due to compaction failure |
| **Archival** | Original messages get `is_active=FALSE` in DB, never deleted | Inspector can always show full uncompacted history |

### 6.3 Compaction System Prompt

```
You are a conversation summarizer. RULES:
1. NEVER invent information not in the conversation.
2. Preserve ALL: user preferences, decisions, factual corrections, code snippets,
   file paths, error messages, configuration values.
3. Preserve chronological order. Use bullet points for discrete facts.
4. If the user corrected the assistant, note the correction explicitly.
5. Keep proper nouns, variable names, and technical terms EXACT.
FORMAT: Start with "## Conversation Summary". Group by topic. End with
"## Key Decisions" listing user decisions/preferences.
```

---

## 7. Chat UI Specifications

### 7.1 Three-Column Layout

```
┌─────────────────────────────────────────────────────────┐
│  Topbar: Model Selector │ Session Title │ Theme Toggle  │
├──────────┬──────────────────────────────┬───────────────┤
│          │                              │               │
│ History  │     Message Thread           │  Inspector    │
│ Sidebar  │                              │  Panel        │
│ (260px)  │     (flex-1, min 400px)      │  (320px)      │
│          │                              │  (collapsible │
│          │                              │   default off)│
│          ├──────────────────────────────┤               │
│          │  Composer Bar                │               │
└──────────┴──────────────────────────────┴───────────────┘
```

| Breakpoint | Behavior |
|:---|:---|
| `< 640px` | Sidebar hidden (hamburger). Inspector as bottom sheet. Full-bleed messages. |
| `640–1024px` | Sidebar as overlay drawer. Inspector as slide-over. |
| `≥ 1024px` | Full three-column. Sidebar persistent. Inspector toggle via `Cmd+Shift+I`. |

### 7.2 Message Rendering — Flat Messages (Not Bubbles)

Full-width message blocks with subtle sender differentiation. Max prose width: `65ch`.

| Element | Light | Dark |
|:---|:---|:---|
| User messages | `bg-zinc-50` | `bg-zinc-900` |
| Assistant messages | `bg-white` (canvas) | `bg-zinc-950` (canvas) |
| Avatar | 28×28 rounded square | User: initials on `bg-blue-600`. Assistant: Forge icon. |
| Timestamp | Relative ("2m ago"), absolute on hover | `text-xs text-zinc-400`, right-aligned |
| Token count | Per assistant message, right-aligned | `text-xs text-zinc-400` |
| Action buttons | Appear on hover (desktop) or always (mobile) | Copy, Regenerate |

### 7.3 Streaming Text Renderer

**Strategy: Chunked DOM Append with CSS Cursor** — do NOT re-render the entire message on each token.

1. Append text nodes to the current element via a `ref`.
2. Blinking cursor via `.streaming` CSS `::after` pseudo-element.
3. Markdown: buffer tokens until block boundary detected (double newline, code fence). Parse completed blocks. Trailing incomplete block as plain text.
4. Code blocks: render with plain `<pre>` during stream, apply Shiki highlighting on close fence.
5. Auto-scroll only if user is at bottom (100px threshold). Show "↓ New content below" pill if user scrolled up.

### 7.4 Keyboard Shortcuts

| Shortcut | Action |
|:---|:---|
| `Cmd/Ctrl+N` | New conversation |
| `Cmd/Ctrl+M` | Open model selector |
| `Cmd/Ctrl+K` | Focus sidebar search |
| `Cmd/Ctrl+Shift+I` | Toggle inspector panel |
| `Cmd/Ctrl+Shift+D` | Toggle dark/light theme |
| `Enter` | Send message |
| `Shift+Enter` | New line in composer |
| `Escape` | Stop generation / close panel |
| `Cmd/Ctrl+/` | Show keyboard shortcut overlay |

### 7.5 Conversation Sidebar

- Grouped by time: Today / Yesterday / Previous 7 Days / Older.
- Auto-titling via background LLM call after first exchange.
- Context menu: Rename, Pin, Export as Markdown, Delete (popover confirmation, no modal).
- Client-side fuzzy search on titles. Server-side FTS5 on message content.

### 7.6 Model Selector (Topbar)

- Grouped: Local Models first, then Remote Providers.
- Real-time health badges: 🟢 Connected, 🔴 Offline, ⚪ Not configured.
- Keyboard: `Cmd+M` opens, arrows navigate, Enter selects, Escape closes.
- Per-conversation persistence. Global default in settings.

### 7.7 Settings Page

- Provider management: Add/edit/remove. API keys stored encrypted in DB, redacted in UI.
- "Test Connection" button validates provider connectivity.
- Defaults: model, system prompt, theme, send-on-enter.
- Env vars take precedence over UI values (show "Set via environment variable" badge).

### 7.8 Accessibility (WCAG AA)

- **Streaming `aria-live`:** Debounced updates every 3 seconds (not per token).
- **Keyboard navigation:** Full tab order through sidebar → messages → composer → inspector.
- **Focus management:** Never steal focus from composer on new message arrival.
- **Color contrast:** All pairs ≥ 4.5:1. Non-color indicators on all status elements.
- **Reduced motion:** Respect `prefers-reduced-motion` — disable all animations.
- **Skip navigation:** Visually-hidden "Skip to main content" link.

---

## 8. Technical Roadmap (8 Phases)

### Phase 1: Skeleton & Streaming

**Objective:** Establish project structure, streaming API, and build pipeline.

| # | Task | Details |
|:--|:---|:---|
| 1.1 | Initialize Go project | `github.com/user/forge`, `go.mod`, project layout per §3 |
| 1.2 | Config module | Layered config: env vars → `forge.yaml` → CLI flags → defaults |
| 1.3 | HTTP server with `chi` | Middleware chain: logging, CORS, request ID, recovery |
| 1.4 | Graceful shutdown | SIGTERM handling: stop new connections → drain streams → checkpoint DB |
| 1.5 | OpenAI-compatible router | `POST /v1/chat/completions` (stream + non-stream), `GET /v1/models` |
| 1.6 | SSE streaming pipeline | Bounded channel (32), backpressure with 5s slow-consumer timeout |
| 1.7 | MockInferenceProvider | Configurable token list, per-token delay, failure injection |
| 1.8 | Scaffold React frontend | Vite + React 19 + Tailwind v4 + Lucide. `go:embed` serves at `/chat` |
| 1.9 | Makefile & build tags | `make build`, `make test` (stub `ui/dist`), `//go:build !noui` tag |
| 1.10 | SQLite storage layer | `modernc.org/sqlite`, WAL mode, embedded migrations, `Store` interface |
| 1.11 | Structured logging | `zerolog` or `slog`. Request logging middleware. |
| 1.12 | Auth middleware (no-op) | Interface wired from Day 1, passthrough in local mode |

**Exit criteria:** `./forge` starts, `GET /v1/models` returns mock model, `POST /v1/chat/completions` streams SSE tokens, `/chat` serves a placeholder page.

---

### Phase 2: Chat UI MVP

**Objective:** A user can `./forge` and have a working chat with a real model.

| # | Task | Details |
|:--|:---|:---|
| 2.1 | Ollama provider | Auto-detect at `localhost:11434`. Implement `InferenceProvider`. NDJSON stream parsing. |
| 2.2 | Model Registry | Query Ollama for available models. Power `/v1/models`. |
| 2.3 | Session Manager | Session CRUD: create, list, get (with messages), delete. Forge-native `/api/sessions` API. |
| 2.4 | Message persistence | SQLite schema: `sessions` + `messages` tables. Tree-based message schema (`parent_id`) for future branching. |
| 2.5 | Chat message thread | Flat message layout per §7.2. Streaming renderer per §7.3. |
| 2.6 | Composer bar | Text input, send button, `Enter` to send, `Shift+Enter` for newline. |
| 2.7 | Stop generation button | Send → Stop (square icon) during streaming. Cancel inference context. Keep partial response. |
| 2.8 | Conversation sidebar | List, create, rename, delete. Auto-title. Time grouping. |
| 2.9 | Model selector dropdown | Topbar. Grouped by provider. Health status badges. |
| 2.10 | Markdown rendering | Code blocks with Shiki highlighting and Copy button. |
| 2.11 | Dark/light theme toggle | `<html class="dark">`. Blocking `<script>` in `<head>` to prevent FOUC. |
| 2.12 | Per-session concurrency | Per-session mutex (or channel-based actor) to prevent state corruption from concurrent requests. |
| 2.13 | First-run onboarding | Welcome → Connect a model (auto-detect Ollama) → Ready. Suggested prompts. |
| 2.14 | Connection status indicator | Topbar colored dot: green/amber/red. |

**Exit criteria:** User runs `./forge`, sees onboarding, connects to Ollama, chats with streaming responses, manages conversations, switches models. Total time from binary to first response: < 60 seconds.

---

### Phase 3: Multi-Provider Support

**Objective:** Support cloud providers alongside local models.

| # | Task | Details |
|:--|:---|:---|
| 3.1 | OpenAI provider | Dedicated. `http.NewRequestWithContext` for cancellation. SSE stream parsing. |
| 3.2 | Anthropic provider | Dedicated. System prompt as top-level param. Strictly alternating messages. `content_block_delta` parsing. |
| 3.3 | Google Gemini provider | Dedicated. `system_instruction` param. Different streaming format. |
| 3.4 | Generic OpenAI-compatible | Configurable `base_url` + `api_key`. Covers Groq, Together, Mistral, OpenRouter, LM Studio, vLLM. |
| 3.5 | Settings UI | Provider management: add/edit/remove providers, API keys (encrypted storage), "Test Connection" button. |
| 3.6 | Model capabilities detection | `ModelCapabilities` struct: context window, supports_tools, supports_vision, tokenizer ID. Static registry + runtime detection. |
| 3.7 | Role normalization layer | Canonical `Message` format → provider-specific formatting (Anthropic: merge consecutive roles, Gemini: no system role). |
| 3.8 | Circuit breaker + retry | Per-provider. 3 failures → open for 30s. Exponential backoff on `{429, 500, 502, 503}`. Respect `Retry-After`. |
| 3.9 | Rate limiting feedback UI | Show countdown on 429. Auto-retry on expiry. |
| 3.10 | System prompt editor | Collapsible panel above messages. Token count display. Presets. Per-conversation override. |

**Exit criteria:** User can chat with Ollama, OpenAI, Anthropic, Gemini, and any OpenAI-compatible endpoint. API keys configurable from Settings UI.

---

### Phase 4: Context Management & Compaction

**Objective:** Handle long conversations without errors or degraded quality.

| # | Task | Details |
|:--|:---|:---|
| 4.1 | Token counting abstraction | Per-provider: `tiktoken-go` for OpenAI, `chars×0.32` for Anthropic, Ollama `/api/show`, Gemini `countTokens` API. Fallback: `bytes/3.5`. |
| 4.2 | Context window assembly | System prompt reservation → output budget → fill history newest-first. Assert invariant: `system + history + output ≤ max`. |
| 4.3 | Rolling compaction engine | Per §6. Trigger at 80% of usable context. Cheap summarization model. Auto-pin important messages. |
| 4.4 | Summary re-compaction | When summary > 2000 tokens, condense to ~1000. |
| 4.5 | Compaction validation | Verify named entities from original appear in summary. Reject bad summaries silently. |
| 4.6 | Token usage UI | Segmented bar: system (purple), history (blue), current (green), remaining (gray). Warning at 80%, red at 95%. Numeric breakdown below bar. |
| 4.7 | Compaction notification | Subtle toast: "Context compacted: 6,240 → 2,100 tokens". Timeline event in Inspector. |
| 4.8 | Message archival | Compacted messages get `is_active=FALSE`. Original messages always accessible in Inspector. |

**Exit criteria:** Conversations of 100+ messages work without context overflow errors. Compaction fires transparently. Token usage visible in UI.

---

### Phase 5: Tool Execution

**Objective:** Implement the Pause-Execute-Resume tool loop with security.

| # | Task | Details |
|:--|:---|:---|
| 5.1 | Tool manifest system | JSON manifests in `~/.forge/tools/`. Discovery at startup. JSON Schema for parameters. |
| 5.2 | Tool executor (local sandbox) | `exec.CommandContext` with explicit argv — **never `sh -c`**. Allowlisted commands. Restricted env vars. `Setpgid` for process group killing. |
| 5.3 | Tool output truncation | Max 64KB output. Truncate before injecting into LLM context. |
| 5.4 | Stream interceptor | Pause-Execute-Resume loop per §5. Tool call accumulator for streamed arguments. |
| 5.5 | Parallel tool execution | Multiple tool calls in one response execute concurrently via `sync.WaitGroup`. |
| 5.6 | Tool execution cards (UI) | Inline status cards: Pending → Running (elapsed timer) → Success (collapsed output) / Error. |
| 5.7 | Tool approval mode | Configurable: auto-approve, always-ask, deny-all. User confirmation dialog. |
| 5.8 | Tool call validation | Reject unknown tools. Validate JSON arguments. Sanitize shell-adjacent inputs. |
| 5.9 | Docker sandbox (hosted mode) | `--network=none --read-only --memory=128m --cpus=0.5`. |
| 5.10 | llama.cpp direct provider | For users running llama-server without Ollama. |

**Exit criteria:** Model can call registered tools. Execution shows progress in chat. Tool output feeds back into inference. Security: no shell injection, command allowlisting enforced.

---

### Phase 6: Inspector & Observability

**Objective:** Deliver the Inspector UI and event-driven architecture.

| # | Task | Details |
|:--|:---|:---|
| 6.1 | Event Bus (in-process pub/sub) | Typed events. Fan-out to WebSocket clients. Ping/pong heartbeat every 30s. |
| 6.2 | WebSocket `/ws` endpoint | Structured event envelope: `{type, session_id, timestamp, payload}`. |
| 6.3 | Inspector: token usage bar | Segmented horizontal bar with hover tooltips. Per-segment breakdown. |
| 6.4 | Inspector: context viewer | Accordion list of messages. Role icon, truncated content, token count. "Raw JSON" tab showing exact payload sent to provider. |
| 6.5 | Inspector: event stream | Log-style scrolling feed. Filterable by type. Pause/resume. Export as JSONL. Max 1000 events buffer. |
| 6.6 | Inspector: tool timeline | Vertical timeline of tool executions. Color-coded by status. Click for detail slide-over. |
| 6.7 | Inspector inline panel | Toggle via `Cmd+Shift+I` within chat view (right panel). Also standalone at `/inspector`. |
| 6.8 | Compaction diff view | Before/after toggle showing what was removed/summarized. |

**Exit criteria:** Inspector shows real-time token usage, raw context window, event stream, and tool timeline. Developers can debug prompt issues by seeing exactly what the model sees.

---

### Phase 7: Production Hardening

**Objective:** Enable secure, deployable self-hosting.

| # | Task | Details |
|:--|:---|:---|
| 7.1 | API key authentication | Bearer token middleware. `FORGE_API_KEY` env var. |
| 7.2 | UI session management | httpOnly JWT cookie for Chat UI. Never expose API keys to frontend. |
| 7.3 | PostgreSQL adapter | Implement `Store` interface for PG via `pgx`. Enabled via `DATABASE_URL`. |
| 7.4 | Interface compliance tests | Same test suite runs against SQLite and PostgreSQL implementations. |
| 7.5 | Rate limiting middleware | Per-key token bucket for hosted mode. |
| 7.6 | CORS configuration | `*` in local mode. Explicit `FORGE_CORS_ORIGINS` in hosted mode. |
| 7.7 | CSP headers | `default-src 'self'`. Prevent XSS in rendered markdown. |
| 7.8 | Health check endpoint | `GET /api/health` — DB status, provider status, uptime, version. |
| 7.9 | Dockerfile | Multi-stage: build UI, build Go binary, final ~20MB image. |
| 7.10 | docker-compose.yml | One-command deployment with optional PG. |
| 7.11 | Database migration system | Embedded SQL files via `go:embed`. Auto-migrate on startup. |
| 7.12 | Message editing & regeneration | Edit user message → fork conversation. Regenerate → carousel (← 1/3 →). |
| 7.13 | Conversation export | Download as Markdown or JSON. |
| 7.14 | Keyboard shortcuts overlay | `Cmd+/` shows full shortcut reference. |

**Exit criteria:** `docker run -p 8080:8080 forge` works with auth enabled. PostgreSQL supported. Security headers set.

---

### Phase 8: Power Features

**Objective:** Features that make Forge special for power users.

| # | Task | Details |
|:--|:---|:---|
| 8.1 | Conversation branching UI | Tree navigation when editing past messages. Branch indicator in sidebar. |
| 8.2 | Prompt templates library | Built-in presets + user-created templates. Include system prompt, model, temperature. |
| 8.3 | Image upload (vision) | Paste/drag-drop. Thumbnail preview. Only shown if model supports vision. Base64 multimodal format. |
| 8.4 | Thinking/reasoning blocks | Display extended thinking from Claude/o3 in collapsible blocks. |
| 8.5 | Conversation search (FTS) | SQLite FTS5 full-text search across all message content. |
| 8.6 | Conversation import | Import from ChatGPT/Claude export formats. |
| 8.7 | Cost estimation | Per-request and per-session cost based on model registry pricing data. |
| 8.8 | `--ui-dir` flag | Optionally serve frontend from local directory instead of embedded. |

**Exit criteria:** Power users can branch conversations, use templates, upload images, and search across all conversations.

---

## 9. API Surface

### 9.1 OpenAI-Compatible (Drop-in SDK Compatibility)

| Endpoint | Method | Phase |
|:---|:---|:---|
| `/v1/chat/completions` | POST | Phase 1 |
| `/v1/models` | GET | Phase 1 |
| `/v1/models/{model}` | GET | Phase 3 |

### 9.2 Forge-Native API

```
GET    /api/sessions                    # List sessions
POST   /api/sessions                    # Create session
GET    /api/sessions/{id}               # Get session (with messages)
PATCH  /api/sessions/{id}               # Update title, model, system prompt
DELETE /api/sessions/{id}               # Delete session
POST   /api/sessions/{id}/compact       # Trigger manual compaction

GET    /api/providers                   # List configured providers
GET    /api/tools                       # List registered tools
GET    /api/health                      # Health check

WS     /ws                             # WebSocket event bus
```

### 9.3 WebSocket Event Types

| Event Type | Direction | Payload |
|:---|:---|:---|
| `inference.started` | Server → Client | `{model, token_count}` |
| `inference.token` | Server → Client | `{delta}` |
| `inference.tool_call` | Server → Client | `{call_id, tool, arguments}` |
| `inference.tool_result` | Server → Client | `{call_id, status, output}` |
| `inference.completed` | Server → Client | `{finish_reason, usage}` |
| `inference.error` | Server → Client | `{code, message}` |
| `compaction.started` | Server → Client | `{original_tokens}` |
| `compaction.completed` | Server → Client | `{new_tokens, removed_messages}` |
| `session.created` | Server → Client | `{session}` |
| `session.updated` | Server → Client | `{session}` |
| `model.status_changed` | Server → Client | `{model_id, status}` |
| `ping` / `pong` | Bidirectional | `{}` (keepalive, 30s interval) |

---

## 10. Database Schema (Day 1)

```sql
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL DEFAULT 'default',
    title        TEXT NOT NULL DEFAULT 'New Chat',
    model        TEXT NOT NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'active',  -- active|idle|expired|archived
    token_count  INTEGER NOT NULL DEFAULT 0,
    message_count INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_access  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE messages (
    id             TEXT PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    parent_id      TEXT,               -- nullable: null = root (tree for branching)
    role           TEXT NOT NULL,       -- system|user|assistant|tool
    content        TEXT NOT NULL,
    token_count    INTEGER NOT NULL DEFAULT 0,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,  -- FALSE after compaction
    pinned         BOOLEAN NOT NULL DEFAULT FALSE,
    model          TEXT,               -- which model generated this
    metadata       TEXT,               -- JSON: tool_calls, compaction info, etc.
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE providers (
    id         TEXT PRIMARY KEY,       -- ollama, openai, anthropic, etc.
    type       TEXT NOT NULL,          -- ollama|openai|anthropic|gemini|openai_compat
    base_url   TEXT NOT NULL,
    api_key    TEXT,                   -- encrypted at rest
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE _migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_messages_session ON messages(session_id, is_active);
CREATE INDEX idx_sessions_user    ON sessions(user_id, status);
```

---

## 11. Testing Strategy

### 11.1 Test Pyramid

| Layer | Tool | Scope |
|:---|:---|:---|
| **Unit tests** | `go test` | Each module in isolation. Mocked dependencies. |
| **Interface compliance** | `go test` | Same test suite runs against SQLite `:memory:` and PostgreSQL (if `TEST_PG_URL` set). |
| **Integration tests** | `go test` + `httptest` | Full streaming pipeline with mock provider, real HTTP server, real SSE parsing. |
| **E2E tests** | Playwright | Chat flow, model switching, conversation management against real Forge + mock provider. |
| **Load tests** | `go test -run Load` | 50 concurrent streams. Guard against data races with `-race`. |

### 11.2 Mock Provider (First-Class)

Configurable token list, per-token delay, failure injection at token N, tool call responses. Used by all automated tests.

### 11.3 CI Requirements

- `go test -race ./...` on every PR.
- Frontend: `npm run lint && npm run build` on every PR.
- E2E: Playwright against `./forge` with mock provider.
- Bundle size check: `< 250KB gzipped` for chat UI.

---

## 12. Risk Mitigation

### 12.1 Technical Risks

| Risk | Severity | Mitigation |
|:---|:---|:---|
| **CGO conflict** (`CGO_ENABLED=0` vs `mattn/go-sqlite3`) | 🔴 Critical | Use `modernc.org/sqlite` (pure Go). Decision made. |
| **Command injection** in tool execution | 🔴 Critical | Never use `sh -c`. Argv-only `exec.CommandContext`. Command allowlisting. Restricted env vars. |
| **No graceful shutdown** | 🔴 Critical | Two-phase: stop new connections → drain in-flight streams → WAL checkpoint → close DB. |
| **Concurrent session state corruption** | 🔴 Critical | Per-session mutex or channel-based actor. Never mutate context during streaming. |
| **Mid-stream provider failure** | 🟡 High | Save partial response. Send SSE error event before `[DONE]`. Circuit breaker. |
| **SQLite concurrent writes** | 🟡 High | WAL mode. Separate read/write connection pools. `SetMaxOpenConns(1)` for writer. |
| **Frontend bundle bloat** | 🟢 Medium | Budget: < 250KB gzipped. Manual chunks in Vite config. Tree-shake Lucide icons. |
| **`go test` fails without Node.js** | 🟢 Medium | `//go:build !noui` tag. Stub `ui/dist` in Makefile test target. |

### 12.2 LLM-Specific Risks

| Risk | Severity | Mitigation |
|:---|:---|:---|
| **Compaction hallucination** | 🟡 High | Structured compaction format. Diff-based verification of named entities. Always archive originals. Non-fatal failure. |
| **Prompt injection via tool results** | 🟡 High | Truncate oversized results (4096 token max). Strip role-injection patterns. Sanitize before context injection. |
| **Token counting inaccuracy** | 🟡 High | Per-provider tokenizer. Conservative estimate fallback (`bytes/3.5`, round UP). Better to compact early than overflow. |
| **Infinite tool loop** | 🟢 Medium | `max_tool_rounds=10`, `max_total_tool_calls=25`. Force final answer on limit. Telemetry on deep loops. |
| **Context poisoning from bad summaries** | 🟢 Medium | Validation prompt (optional for critical sessions). Reject summaries that fail entity count check. |
| **Provider API format changes** | 🟢 Medium | Isolated provider code. Integration tests against real APIs (nightly). Pin API versions. |

### 12.3 Product Risks

| Risk | Severity | Mitigation |
|:---|:---|:---|
| **Scope creep into RAG** | 🟡 High | Explicitly deferred. RAG is a separate product. |
| **Trying to out-feature Open WebUI** | 🟡 High | Compete on simplicity and DX, not feature count. Be ruthlessly selective. |
| **Community adoption** | 🟢 Medium | Focus on small, polished MVP. Get 100 happy users before adding features. |
| **Streaming reliability** | 🟢 Medium | WebSocket heartbeat. SSE `id:` fields for `Last-Event-ID` resume. Auto-reconnect with exponential backoff. |

---

## 13. Dependencies

### Go

| Dependency | Purpose |
|:---|:---|
| `github.com/go-chi/chi/v5` | HTTP router (lightweight, stdlib-compatible) |
| `modernc.org/sqlite` | SQLite driver (pure Go, enables `CGO_ENABLED=0`) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver (pure Go, Phase 7) |
| `github.com/pkoukk/tiktoken-go` | OpenAI-compatible BPE tokenizer |
| `github.com/rs/zerolog` | Zero-alloc structured logging |
| `github.com/caarlos0/env/v11` | Struct-tag config from env vars |

### Frontend

| Dependency | Purpose |
|:---|:---|
| React 19 | UI framework |
| Tailwind CSS v4 | Utility-first CSS |
| Lucide React | Tree-shakable icons |
| Vite | Build tool |
| Radix UI Primitives | Accessible dropdown, dialog, popover, tooltip, accordion, tabs |
| Shiki | VS Code-quality syntax highlighting |

---

## 14. Distribution

| Channel | Priority | Phase |
|:---|:---|:---|
| GitHub Releases (binary) | P0 | Phase 2 |
| `curl` one-liner install script | P0 | Phase 2 |
| Docker Hub image | P0 | Phase 7 |
| Homebrew tap | P1 | Phase 7 |
| AUR package | P2 | Post-launch |

**Supported platforms:** Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64).

---

*Review documents archived in `reviews/`. This plan incorporates feedback from: Product Manager, UI/UX Design Architect, Senior Developer, LLM Engine Architect, and Module Architect.*
