# Forge: Prioritized Implementation Plan

> **Author:** Product Manager (TPM Agent)
> **Date:** 2025-07-15
> **Status:** ACTIVE
> **Based on:** Code audit of 18 Go files (~1,400 LOC), IMPLEMENTATION_PLAN.md, 5 expert reviews

---

## 0. Current State Audit

### What Actually Exists (More Than Stated)

| Component | Status | Files | Notes |
|:---|:---|:---|:---|
| **Config** | ✅ Complete | `internal/config/config.go` | 30+ env vars, all defaults set |
| **HTTP Server** | ✅ Complete | `internal/server/server.go` | Chi + middleware chain + graceful shutdown |
| **SSE Pipeline** | ✅ Complete | `internal/streaming/sse.go` | Bounded channel (32), CloseNotifier, backpressure |
| **OpenAI Types** | ✅ Complete | `pkg/types/openai.go` | 14 structs: request, response, chunk, delta, tools |
| **Stream Events** | ✅ Complete | `pkg/types/events.go` | 8 event types, ToolCallEvent struct |
| **Structured Errors** | ✅ Complete | `pkg/types/errors.go` | OpenAI-format errors, WriteError(), 12 error codes |
| **Session Types** | ✅ Types only | `pkg/types/session.go` | Session, SessionListItem, CRUD request/response types |
| **Message Types** | ✅ Types only | `pkg/types/message.go` | Message, MessageMeta, SendMessageRequest/Response |
| **Provider Types** | ✅ Types only | `pkg/types/provider.go` | ProviderInfo, TestProviderResponse |
| **WS Event Types** | ✅ Types only | `pkg/types/ws_events.go` | 12 WS event types, all typed payloads |
| **Health Types** | ✅ Types only | `pkg/types/health.go` | HealthResponse, ConfigResponse |
| **Provider Interface** | ✅ Complete | `internal/inference/provider.go` | 6-method interface, ModelCapabilities struct |
| **OpenAI Provider** | ✅ Complete | `internal/inference/openai.go` | SSE parsing, streaming + non-streaming |
| **Mock Provider** | ✅ Complete | `internal/inference/mock.go` | Configurable tokens, delay, failure injection |
| **Provider Registry** | ✅ Complete | `internal/inference/registry.go` | Thread-safe, 4-step model resolution, RefreshModelMap |
| **Retry Logic** | ✅ Complete | `internal/inference/retry.go` | Exponential backoff, retryable status detection |
| **API Handlers** | ✅ Partial | `internal/api/handlers_chat.go` | 2 endpoints, model resolution inline (doesn't use Registry) |
| **Entry Point** | ✅ Working | `cmd/forge/main.go` | Hardcoded provider map, doesn't use Registry |

### What Is Truly Missing (No Implementation)

| Component | Impact | Blocking? |
|:---|:---|:---|
| **SQLite Store** | No persistence. Conversations vanish on restart. | 🔴 Yes — blocks sessions |
| **Session Manager** | No conversation lifecycle. Every request is stateless. | 🔴 Yes — blocks chat UX |
| **Ollama Provider** | The "zero-config" promise is broken. Need API keys to use anything. | 🔴 Yes — blocks MVP |
| **Frontend** | No UI at all. Users must use `curl` to chat. | 🔴 Yes — blocks MVP |
| **Forge API routes** | No `/api/sessions/*`, no `/api/health`, no `/api/providers` | 🔴 Yes — blocks frontend |
| **Auth Middleware** | `FORGE_API_KEY` config exists but is never checked | 🟡 No — acceptable for local-only MVP |
| **Token Counting** | Returns hardcoded `100` (OpenAI) or `10` (Mock) | 🟡 No — works until long conversations |
| **Structured Logging** | Uses `log.Printf` everywhere, no levels, no JSON output | 🟡 No — cosmetic for MVP |
| **Tests** | Zero test files in the project | 🟡 No — but high debt risk |
| **main.go wiring** | Doesn't use ProviderRegistry, hardcoded provider map | 🟡 No — tech debt |

### Key Architectural Observations

1. **Types layer is 80% done.** Session, Message, Provider, WS Event, Health, Error types are all defined and well-designed. The data model is ready — it just needs implementation behind it.
2. **ProviderRegistry exists but isn't wired.** `main.go` builds a raw `map[string]InferenceProvider` and passes it to the server. The thread-safe `ProviderRegistry` with proper model resolution sits unused.
3. **API handler does inline model resolution** via `getProviderForModel()` which duplicates the Registry's `Resolve()` logic, but worse (calls `ListModels` on every request, iterates all providers).
4. **No `internal/store` package exists** despite the plan defining it clearly. This is the biggest gap.
5. **`http.CloseNotifier` is deprecated** in the SSE pipeline — should use `r.Context().Done()` instead.

---

## 1. MVP Definition

### Core User Problem

> *"I want to run a local AI chat with zero setup. I should be able to download one file, run it, and be chatting with my local Ollama models within 60 seconds."*

### Minimum Feature Set for MVP

A user downloads the binary, runs `./forge`, and:

1. **Sees a web UI** at `http://localhost:8080` with a chat interface
2. **Auto-detects Ollama** running locally and lists available models
3. **Can send messages** and receive streaming responses
4. **Conversations persist** across browser refreshes (SQLite storage)
5. **Can manage conversations** (create, switch, delete)
6. **Can select models** from available Ollama models

### Explicitly NOT in MVP

- ❌ No cloud providers (OpenAI, Anthropic, Gemini) — Ollama only
- ❌ No context compaction — just truncate if context overflows
- ❌ No tool execution — pure chat only
- ❌ No Inspector panel — ship later as differentiator
- ❌ No authentication — local mode only
- ❌ No settings UI — env vars only for configuration
- ❌ No conversation branching, regeneration, or editing
- ❌ No image/vision support
- ❌ No PostgreSQL — SQLite only
- ❌ No Docker image
- ❌ No WebSocket event bus — SSE only for streaming

---

## 2. Phase Breakdown

### Phase A: Backend Foundation (No UI Required)
**Goal:** Wire storage + sessions + Ollama so the API is fully functional via `curl`.
**Duration estimate:** 3-4 days
**Dependency:** None — starts from current state

```
                    ┌─────────────┐
                    │   Config    │ (exists)
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
        ┌─────▼─────┐ ┌───▼────┐ ┌────▼──────┐
        │  Store     │ │ Ollama │ │  Registry │
        │ (SQLite)   │ │Provider│ │  Wiring   │
        └─────┬──────┘ └───┬────┘ └────┬──────┘
              │            │            │
              └────────────┼────────────┘
                           │
                    ┌──────▼──────┐
                    │  Session    │
                    │  Manager    │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Forge API  │
                    │  Endpoints  │
                    └─────────────┘
```

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **a-1** | SQLite store interface | Define `Store` interface in `internal/store/store.go`: `CreateSession`, `GetSession`, `ListSessions`, `DeleteSession`, `SaveMessage`, `GetMessages`, `UpdateSession`. Use the types already defined in `pkg/types/session.go` and `pkg/types/message.go`. | — | 0.5d |
| **a-2** | SQLite implementation | Implement `Store` with `modernc.org/sqlite`. WAL mode. Embedded migration SQL (`001_init.sql` from §10 of IMPL_PLAN). Auto-migrate on startup. Connection pooling: single writer, multiple readers. ULID generation for IDs (prefix `ses_` / `msg_`). | a-1 | 1.5d |
| **a-3** | Ollama provider | Implement `InferenceProvider` for Ollama. NDJSON stream parsing (not SSE). Auto-detect at startup by probing `GET http://localhost:11434/api/tags`. `ListModels` via `/api/tags`. `StreamChat` via `POST /api/chat`. Map Ollama response format to canonical `StreamEvent`. | — | 1d |
| **a-4** | Wire ProviderRegistry | Refactor `main.go` to use the existing `ProviderRegistry` instead of raw map. Register Ollama (auto-detected), OpenAI-compat providers (if configured). Call `RefreshModelMap()` at startup. Pass registry to API router instead of raw map. | a-3 | 0.5d |
| **a-5** | Session manager | Create `internal/session/manager.go`. Business logic layer between API handlers and Store: create session (with default model), send message (persist user msg → call inference → persist assistant msg), list/get/delete sessions. Per-session mutex to prevent concurrent inference corruption. | a-1, a-2, a-4 | 1d |
| **a-6** | Forge API endpoints | Implement handlers in `internal/api/forge/sessions.go`: `GET /api/sessions`, `POST /api/sessions`, `GET /api/sessions/{id}`, `PATCH /api/sessions/{id}`, `DELETE /api/sessions/{id}`, `POST /api/sessions/{id}/messages` (the main chat endpoint — saves messages, calls inference, streams response). `GET /api/health` (basic). | a-5 | 1d |
| **a-7** | Structured logging | Replace all `log.Printf` with `log/slog` (stdlib, no dependency). Add request logging middleware with request_id, method, path, status, duration. Configurable level via `FORGE_LOG_LEVEL`. | — | 0.5d |

**Exit Criteria (Phase A):**
```bash
# 1. Start Forge with Ollama running locally
./forge

# 2. Verify Ollama auto-detected
curl http://localhost:8080/v1/models
# → Returns real Ollama models (llama3.2, etc.)

# 3. Create a session
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3.2:1b"}'
# → Returns session with ID

# 4. Send a message and get streaming response
curl -X POST http://localhost:8080/api/sessions/{id}/messages \
  -H "Content-Type: application/json" \
  -d '{"content": "Hello!", "stream": true}'
# → Streams SSE tokens from Ollama

# 5. Messages persisted in SQLite
curl http://localhost:8080/api/sessions/{id}
# → Returns session with message history

# 6. Restart forge, sessions still there
pkill forge && ./forge
curl http://localhost:8080/api/sessions
# → Previous sessions still listed
```

---

### Phase B: Chat UI MVP (The "Download & Chat" Experience)
**Goal:** A user runs `./forge`, opens browser, and chats immediately.
**Duration estimate:** 4-5 days
**Dependency:** Phase A complete

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **b-1** | React scaffold | Vite + React 19 + TypeScript + Tailwind CSS v4. Basic project structure: `ui/src/App.tsx`, layout shell, API client module. Build outputs to `ui/dist/`. | — | 0.5d |
| **b-2** | `go:embed` wiring | Create `embed.go` at project root with `//go:embed ui/dist` directive. Serve at `/` with SPA fallback (serve `index.html` for all non-API, non-asset routes). `//go:build !noui` tag for test builds without Node.js. Add Makefile targets. | b-1 | 0.5d |
| **b-3** | API client module | TypeScript module (`ui/src/lib/api.ts`): typed fetch wrappers for all Forge API endpoints. SSE helper for streaming responses. Error handling that maps to `APIErrorResponse` type. | b-1 | 0.5d |
| **b-4** | Message thread | `MessageThread.tsx`: Flat message layout (not bubbles). Role icons (user initials, Forge icon). Markdown rendering with `react-markdown` + `remark-gfm`. Code blocks with syntax highlighting and copy button. Auto-scroll with "scroll to bottom" pill. | b-3 | 1.5d |
| **b-5** | Streaming renderer | `StreamingRenderer.tsx`: Append-only DOM updates (not full re-render per token). CSS blinking cursor during streaming. Buffer markdown tokens until block boundary before parsing. | b-4 | 1d |
| **b-6** | Composer bar | `Composer.tsx`: Text input (auto-resize textarea), Send button. `Enter` to send, `Shift+Enter` for newline. Send → Stop (square icon) toggle during streaming. Cancel inference on stop (sends abort to server). | b-3 | 0.5d |
| **b-7** | Conversation sidebar | `Sidebar.tsx`: List sessions from API. Create new conversation button. Click to switch. Delete with confirmation. Time grouping (Today / Yesterday / Older). Auto-title via first user message (truncated, client-side). | b-3 | 1d |
| **b-8** | Model selector | `ModelSelector.tsx`: Topbar dropdown. Fetches from `/v1/models`. Grouped by provider. Shows connection status. Per-session model selection via `PATCH /api/sessions/{id}`. | b-3 | 0.5d |
| **b-9** | Dark/light theme | System preference detection via `prefers-color-scheme`. Manual toggle in topbar. Persist preference in `localStorage`. Blocking `<script>` in `<head>` to prevent FOUC. | b-1 | 0.25d |
| **b-10** | Empty state & onboarding | First-run detection (no sessions). Welcome message with model status. "Start chatting" CTA. Suggested prompts. Connection status indicator (green/red dot). | b-7, b-8 | 0.5d |

**Exit Criteria (Phase B):**
```
1. User runs `./forge` with Ollama running
2. Opens http://localhost:8080 in browser
3. Sees welcome screen with detected Ollama models
4. Selects a model, types a message, gets streaming response
5. Can create, switch, and delete conversations
6. Conversations persist across page refreshes
7. Dark/light mode works
8. Time from binary download to first AI response: < 60 seconds
```

---

### Phase C: Test Infrastructure & Stability
**Goal:** Establish testing foundation and fix known tech debt before adding features.
**Duration estimate:** 2-3 days
**Dependency:** Phase B complete (stable API surface to test against)

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **c-1** | Store tests | `internal/store/sqlite_test.go`: Full CRUD coverage with `:memory:` SQLite. Test all session and message operations. Test migration idempotency. Test concurrent read/write. | — | 0.5d |
| **c-2** | Provider tests | `internal/inference/mock_test.go`: Test streaming, cancellation, failure injection. `internal/inference/registry_test.go`: Test Resolve() with prefixes, model map, defaults. `internal/inference/retry_test.go`: Test backoff, retryable status codes. | — | 0.5d |
| **c-3** | API integration tests | `internal/api/handlers_test.go`: Use `httptest.Server` + mock provider + `:memory:` SQLite. Test full request→response cycle for all endpoints. Test SSE streaming parse. Test error responses match OpenAI format. | c-1 | 1d |
| **c-4** | Session manager tests | `internal/session/manager_test.go`: Test send-message flow end-to-end with mock provider. Test per-session mutex prevents concurrent inference. Test message persistence. | c-1, c-2 | 0.5d |
| **c-5** | Fix deprecated CloseNotifier | Replace `http.CloseNotifier` in `sse.go` with `r.Context().Done()` pattern. `CloseNotifier` was deprecated in Go 1.11. | — | 0.25d |
| **c-6** | CI pipeline | GitHub Actions: `go test -race ./...`, `go vet ./...`, frontend `npm run build` + `npm run lint`. Run on every PR. | c-1, c-2, c-3 | 0.25d |
| **c-7** | Makefile | Complete Makefile with targets: `build`, `dev` (concurrent Go + Vite), `test`, `lint`, `clean`. Stub `ui/dist/index.html` for Go-only test builds. | — | 0.25d |

**Exit Criteria (Phase C):**
```
1. `go test -race ./...` passes with ≥ 70% coverage on store, session, API packages
2. CI runs on every PR and blocks merge on failure
3. `make build` produces working binary
4. `make test` works without Node.js installed (noui build tag)
5. No deprecated API usage warnings
```

---

### Phase D: Multi-Provider Support
**Goal:** Support cloud LLMs alongside local Ollama models.
**Duration estimate:** 3-4 days
**Dependency:** Phase C complete

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **d-1** | Anthropic provider | Dedicated provider. System prompt as top-level param (not in messages). Strictly alternating user/assistant roles (merge consecutive same-role). `content_block_delta` SSE parsing. Map to canonical `StreamEvent`. | — | 1d |
| **d-2** | Google Gemini provider | Dedicated provider. `system_instruction` param. Different streaming format (`candidates[].content.parts[].text`). `countTokens` API for token counting. | — | 1d |
| **d-3** | Role normalization | `internal/inference/normalize.go`: Canonical `[]ChatMessage` → provider-specific formatting. Anthropic: merge consecutive roles, extract system prompt. Gemini: convert system role to `system_instruction`. Called by each provider before sending to API. | — | 0.5d |
| **d-4** | Settings UI | `ui/src/pages/Settings.tsx`: Provider management — list configured providers, add new (type + base_url + API key), "Test Connection" button, enable/disable. API keys stored encrypted in SQLite (AES-GCM with key derived from machine ID). Show "Set via environment variable" badge for env-configured providers. | — | 1d |
| **d-5** | Provider CRUD API | `GET /api/providers`, `POST /api/providers`, `PATCH /api/providers/{id}`, `DELETE /api/providers/{id}`, `POST /api/providers/{id}/test`. Store in `providers` table. API key encryption at rest. Never expose API keys to frontend (only `has_api_key: true`). | — | 0.5d |
| **d-6** | Circuit breaker | Per-provider circuit breaker: 3 consecutive failures → open for 30s → half-open (single probe request). Track in `ProviderRegistry`. Surface status via `/api/providers` and model selector health badges. | — | 0.5d |

**Exit Criteria (Phase D):**
```
1. User can chat with Ollama, OpenAI, Anthropic, and Gemini from same UI
2. User can add a new provider via Settings UI
3. "Test Connection" validates connectivity and shows latency
4. API keys never appear in API responses or browser devtools
5. Circuit breaker prevents cascading failures when a provider is down
6. Model selector shows health status badges (green/red/gray)
```

---

### Phase E: Context Management
**Goal:** Long conversations work without context overflow errors.
**Duration estimate:** 3-4 days
**Dependency:** Phase D complete (need multiple providers for per-provider tokenizers)

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **e-1** | Token counting | `internal/context/tokenizer.go`: Per-provider token counting. OpenAI: `tiktoken-go`. Anthropic: `chars × 0.32`. Ollama: `/api/show` endpoint. Gemini: `countTokens` API. Fallback: `bytes / 3.5` (round UP). Replace all hardcoded `return 100`. | — | 1d |
| **e-2** | Context window assembly | `internal/context/manager.go`: Build context array for inference. System prompt reservation → output budget → fill history newest-first. Assert invariant: `system + history + output ≤ model.MaxContextTokens`. Token count each message on insert (store in DB). | e-1 | 1d |
| **e-3** | Simple truncation | When context exceeds budget, drop oldest messages (not system prompt). Log warning. No compaction yet — just truncation. This is the safe, dumb fallback. | e-2 | 0.5d |
| **e-4** | Rolling compaction | `internal/context/compaction.go`: Per §6 of IMPL_PLAN. Trigger at 80% of usable context. Use cheap model for summarization. Auto-pin: tool results, user corrections, first message, code blocks. Archive originals (`is_active=FALSE`). Non-fatal: if compaction fails, fall back to truncation. | e-2, e-3 | 1.5d |
| **e-5** | Token usage UI | Segmented bar component in chat view: system (purple), history (blue), current (green), remaining (gray). Numeric breakdown on hover. Warning badge at 80%, red at 95%. Show per-message token count on assistant messages. | e-2 | 0.5d |

**Exit Criteria (Phase E):**
```
1. Token counts are accurate (within 5% of provider's reported usage)
2. 100+ message conversations work without context overflow errors
3. Compaction fires transparently and conversation quality is maintained
4. Token usage bar shows real-time context utilization
5. Compacted messages archived in DB (never deleted)
6. Compaction failure falls back to truncation (never crashes)
```

---

### Phase F: Auth & Production Hardening
**Goal:** Safe to expose to the internet with proper security.
**Duration estimate:** 2-3 days
**Dependency:** Phase D complete (needs provider management to protect)

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **f-1** | API key auth middleware | `internal/auth/middleware.go`: Check `Authorization: Bearer <key>` against `FORGE_API_KEY`. No-op when `FORGE_API_KEY` is unset (local mode). Return 401 with OpenAI-format error on mismatch. Apply to all `/v1/*` and `/api/*` routes. | — | 0.5d |
| **f-2** | UI session auth | httpOnly, Secure, SameSite=Strict cookie for chat UI. Login page when `FORGE_API_KEY` is set. Frontend never sees or stores the API key. Session token with configurable expiry. | f-1 | 0.5d |
| **f-3** | CORS hardening | Replace `Access-Control-Allow-Origin: *` with configurable `FORGE_CORS_ORIGINS`. Default `*` in local mode (no `FORGE_API_KEY`), require explicit origins in auth mode. | — | 0.25d |
| **f-4** | Security headers | CSP: `default-src 'self'`. X-Content-Type-Options: nosniff. X-Frame-Options: DENY. Referrer-Policy: same-origin. | — | 0.25d |
| **f-5** | Health endpoint | `GET /api/health`: DB ping, provider connectivity status, uptime, version. Used by monitoring and frontend connection indicator. | — | 0.25d |
| **f-6** | Rate limiting | Token bucket per API key. Configurable: `FORGE_RATE_LIMIT` (requests/minute). Only active when `FORGE_API_KEY` is set. Return 429 with `Retry-After` header. | f-1 | 0.5d |
| **f-7** | Graceful shutdown (enhanced) | Current shutdown is basic. Add: drain in-flight SSE streams (wait up to 10s), WAL checkpoint on SQLite, log active connection count during drain. | — | 0.5d |

**Exit Criteria (Phase F):**
```
1. With FORGE_API_KEY set, all API calls require valid bearer token
2. Chat UI shows login page, uses httpOnly cookie after authentication
3. API keys never appear in browser devtools (cookies are httpOnly)
4. Security headers present on all responses
5. Rate limiting returns 429 with Retry-After on excessive requests
6. `./forge` with no env vars still works with zero auth (local mode)
```

---

### Phase G: Tools & Inspector (Differentiators)
**Goal:** Deliver the two features that differentiate Forge from competitors.
**Duration estimate:** 5-7 days
**Dependency:** Phase E complete (context management needed for tool results)

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **g-1** | Stream interceptor | `internal/tools/interceptor.go`: Pause-Execute-Resume loop. Detect `finish_reason: "tool_calls"` in stream. Accumulate tool call arguments from streamed deltas. Pause stream → execute → inject result → re-infer. Safety limits: `max_tool_rounds=10`, `max_calls_per_round=5`. | — | 1.5d |
| **g-2** | Tool executor (local) | `internal/tools/executor.go`: `exec.CommandContext` with argv (never `sh -c`). Allowlisted commands. Restricted env vars. `Setpgid` for process group kill. Max output 64KB. Per-tool timeout (default 10s). | — | 1d |
| **g-3** | Tool manifest | `internal/tools/manifest.go`: JSON manifests in `~/.forge/tools/`. Discovery at startup. JSON Schema parameter validation. Built-in tools: `read_file`, `list_dir`, `run_command` (allowlisted). | g-2 | 0.5d |
| **g-4** | Tool cards (UI) | `ToolCard.tsx`: Inline status cards in message thread. States: Pending → Running (elapsed timer) → Success (collapsed output) / Error. Expand/collapse output. | g-1 | 0.5d |
| **g-5** | Tool approval mode | Configurable: auto-approve, always-ask, deny-all. Approval dialog in UI. Default: always-ask for destructive commands, auto-approve for read-only. | g-4 | 0.5d |
| **g-6** | Event bus | `internal/events/bus.go`: In-process typed pub/sub. Fan-out to multiple subscribers. No domain knowledge. Used by Inspector. | — | 0.5d |
| **g-7** | WebSocket endpoint | `GET /ws`: Upgrade to WebSocket. Event subscription per session. Ping/pong heartbeat every 30s. Use event types from `pkg/types/ws_events.go` (already defined). | g-6 | 0.5d |
| **g-8** | Inspector: context viewer | Right panel (toggle `Cmd+Shift+I`). Accordion list of messages in current context window. Role icon, truncated content, token count per message. "Raw JSON" tab showing exact payload sent to provider. | g-6, g-7 | 1d |
| **g-9** | Inspector: token bar | Segmented horizontal bar: system (purple), history (blue), current (green), remaining (gray). Hover tooltips with numeric breakdown. | g-8 | 0.5d |
| **g-10** | Inspector: event stream | Log-style scrolling feed of real-time events. Filterable by type. Pause/resume. Max 1000 events buffer. Color-coded by event type. | g-7 | 0.5d |

**Exit Criteria (Phase G):**
```
1. Model can call registered tools, results feed back into inference
2. Tool execution shows live progress in chat (Pending → Running → Done)
3. No shell injection possible (argv-only, allowlisted commands)
4. Inspector shows real-time context window contents
5. Inspector shows token usage breakdown
6. Inspector shows live event stream
7. Toggle Inspector with Cmd+Shift+I
8. Safety limits prevent infinite tool loops
```

---

### Phase H: Power Features & Polish (Post-MVP)
**Goal:** Features that make Forge special for power users.
**Duration estimate:** 5+ days
**Dependency:** Phase G complete

| ID | Task | Description | Depends On | Est. |
|:---|:---|:---|:---|:---|
| **h-1** | PostgreSQL adapter | Implement `Store` interface for PostgreSQL via `pgx/v5`. Enabled via `DATABASE_URL`. Interface compliance tests run against both SQLite and PG. | — | 1.5d |
| **h-2** | Message editing | Edit user message → create branch (new `parent_id`). Show "edited" indicator. Regenerate assistant response for edited message. | — | 1d |
| **h-3** | Response regeneration | "Regenerate" button on assistant messages. Creates alternative response (carousel: ← 1/3 →). Uses tree-based message schema (`parent_id` already in types). | h-2 | 0.5d |
| **h-4** | Conversation export | Download as Markdown or JSON. Export button in sidebar context menu. | — | 0.5d |
| **h-5** | Conversation search (FTS) | SQLite FTS5 full-text search across message content. Search bar in sidebar with instant results. | — | 1d |
| **h-6** | Image upload (vision) | Paste/drag-drop images into composer. Thumbnail preview. Base64 multimodal format. Only shown if model supports vision (check `ModelCapabilities`). | — | 1d |
| **h-7** | Thinking blocks | Display extended thinking from Claude/o3 in collapsible blocks. Parse `thinking` content blocks from Anthropic responses. | — | 0.5d |
| **h-8** | Dockerfile + compose | Multi-stage build: Node → Go → scratch. `docker-compose.yml` with optional PostgreSQL. Target image < 30MB. | h-1 | 0.5d |
| **h-9** | Keyboard shortcuts | Full shortcut set from §7.4 of IMPL_PLAN. `Cmd+/` overlay showing all shortcuts. | — | 0.5d |
| **h-10** | Auto-title via LLM | Background LLM call after first exchange to generate conversation title. Use cheap/fast model. Fallback: truncated first user message. | — | 0.5d |

---

## 3. Dependency Graph (Critical Path)

```
Phase A ──────► Phase B ──────► Phase C ──────┬──► Phase D ──────► Phase E
(Backend)       (Frontend)      (Tests)        │    (Providers)     (Context)
                                               │
                                               └──► Phase F ──────► Phase G ──────► Phase H
                                                    (Auth)          (Tools+Insp)    (Power)
```

**Critical path:** A → B → C → D → E → G

Phases D and F can run in parallel after C.
Phase G requires both D (multi-provider) and E (context management).
Phase H is post-MVP and can be cherry-picked.

---

## 4. Scope Boundaries: What to Explicitly Defer

### NEVER Build (Out of Scope)

| Feature | Rationale |
|:---|:---|
| RAG / document ingestion | Separate product. Scope creep risk #1. |
| Multi-user auth (OAuth, OIDC) | v1 is single-user. `user_id: "default"` in schema enables future migration. |
| Custom model training/fine-tuning | Not a training tool. |
| Plugin/extension system beyond tools | "Tools as the Only Extension Point" is a design principle. |
| Mobile native app | Web UI with responsive design is sufficient. |

### Defer to Post-v1

| Feature | Reason to Defer |
|:---|:---|
| Conversation branching UI (tree view) | Schema supports it (`parent_id`), but UI is complex. Ship linear first. |
| Prompt template library | Nice-to-have, not core value. |
| Cost estimation | Requires pricing data maintenance. Low urgency for local-first users. |
| `--ui-dir` flag for dev | Developer convenience, not user-facing. |
| Conversation import (ChatGPT/Claude) | Retention feature, not acquisition feature. |

---

## 5. Risk Register

| ID | Risk | Severity | Likelihood | Mitigation | Owner |
|:---|:---|:---|:---|:---|:---|
| **R1** | `modernc.org/sqlite` performance on ARM | 🟡 High | Medium | Benchmark early in Phase A. Pure Go SQLite is ~2-3x slower than C version. Acceptable for single-user, but validate with 10K messages. | Developer |
| **R2** | Ollama NDJSON parsing edge cases | 🟡 High | Medium | Ollama uses NDJSON (not SSE). Test with slow models, cancelled requests, and network interruptions. The existing SSE parser won't work — needs dedicated NDJSON reader. | Developer |
| **R3** | Frontend embed bloats binary | 🟢 Medium | Low | Budget: < 300KB gzipped JS+CSS. Use Vite tree-shaking. Audit with `npx vite-bundle-analyzer`. Shiki is the biggest risk (~200KB) — consider lazy loading or lighter alternative. | Design |
| **R4** | Concurrent SQLite writes cause `SQLITE_BUSY` | 🟡 High | High | Single writer connection with `SetMaxOpenConns(1)`. WAL mode. Retry with backoff on busy. This is a known pattern for SQLite in Go. | Developer |
| **R5** | Token counting inaccuracy causes context overflow | 🟡 High | Medium | Use conservative fallback (`bytes/3.5`, round UP). Better to compact/truncate early than overflow. Validate against provider-reported usage in tests. | LLM Architect |
| **R6** | `main.go` doesn't use ProviderRegistry | 🟢 Medium | Certain | Task a-4 addresses this directly. Without it, model resolution is broken and duplicated. | Developer |
| **R7** | `http.CloseNotifier` deprecated since Go 1.11 | 🟢 Medium | Certain | Task c-5. Replace with `r.Context().Done()`. Won't break now but will be removed in future Go versions. | Developer |
| **R8** | Scope creep into multi-user features | 🔴 Critical | High | `user_id: "default"` in every DB record. Never build multi-user UI. This is the "one more feature" trap. PM must block aggressively. | PM |
| **R9** | SSE streaming breaks behind reverse proxies | 🟡 High | Medium | Already have `X-Accel-Buffering: no` header. Document Nginx/Caddy config. Test with common reverse proxy setups. | Developer |

---

## 6. Success Metrics

| Phase | Metric | Target |
|:---|:---|:---|
| **A** | API responds with real Ollama data | All 6 curl commands in exit criteria pass |
| **B** | Time-to-first-response | < 60 seconds from download to first AI response |
| **B** | UI functional completeness | Send, receive, stream, create/switch/delete conversations |
| **C** | Test coverage | ≥ 70% on store, session, API packages |
| **C** | CI green | All tests pass with `-race` flag |
| **D** | Provider count | ≥ 4 providers working (Ollama + OpenAI + Anthropic + Gemini) |
| **E** | Long conversation stability | 100+ message conversation without errors |
| **F** | Security scan | Zero OWASP Top 10 findings on `/api/*` endpoints |
| **G** | Inspector accuracy | Context viewer matches actual payload sent to provider (byte-for-byte) |
| **Overall** | Binary size | < 30MB (Go binary + embedded UI) |
| **Overall** | Startup time | < 500ms from `./forge` to accepting HTTP requests |

---

## 7. Task Summary (Ordered Backlog)

### Phase A — 7 tasks, ~5.5 days
```
a-1  Store interface ................... 0.5d  [no deps]
a-2  SQLite implementation ............. 1.5d  [a-1]
a-3  Ollama provider ................... 1.0d  [no deps]
a-4  Wire ProviderRegistry ............. 0.5d  [a-3]
a-5  Session manager ................... 1.0d  [a-1, a-2, a-4]
a-6  Forge API endpoints ............... 1.0d  [a-5]
a-7  Structured logging ................ 0.5d  [no deps]
```

### Phase B — 10 tasks, ~6.25 days
```
b-1  React scaffold .................... 0.5d  [no deps]
b-2  go:embed wiring ................... 0.5d  [b-1]
b-3  API client module ................. 0.5d  [b-1]
b-4  Message thread .................... 1.5d  [b-3]
b-5  Streaming renderer ................ 1.0d  [b-4]
b-6  Composer bar ...................... 0.5d  [b-3]
b-7  Conversation sidebar .............. 1.0d  [b-3]
b-8  Model selector .................... 0.5d  [b-3]
b-9  Dark/light theme .................. 0.25d [b-1]
b-10 Empty state & onboarding .......... 0.5d  [b-7, b-8]
```

### Phase C — 7 tasks, ~2.75 days
```
c-1  Store tests ....................... 0.5d  [no deps]
c-2  Provider tests .................... 0.5d  [no deps]
c-3  API integration tests ............. 1.0d  [c-1]
c-4  Session manager tests ............. 0.5d  [c-1, c-2]
c-5  Fix deprecated CloseNotifier ...... 0.25d [no deps]
c-6  CI pipeline ....................... 0.25d [c-1, c-2, c-3]
c-7  Makefile .......................... 0.25d [no deps]
```

### Phases D–H — See detailed tables above

---

## 8. Immediate Next Actions

1. **Start Phase A tasks a-1, a-3, a-7 in parallel** (no dependencies between them)
2. **Validate `modernc.org/sqlite`** works with `CGO_ENABLED=0` on the target platform (Risk R1)
3. **Study Ollama's NDJSON format** — it's different from OpenAI's SSE. The existing SSE parser in `openai.go` won't work. Ollama returns `{"message":{"content":"Hello"},"done":false}\n` line by line.
4. **Do NOT start the frontend (Phase B) until Phase A exit criteria pass.** The API surface must be stable before building against it.

---

*This plan is designed to be executed sequentially through the critical path (A→B→C) with parallel work possible in later phases. Every phase delivers independently testable, user-visible value. Scope is aggressively constrained to avoid the "trying to out-feature Open WebUI" trap identified in the risk register.*
