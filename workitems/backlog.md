# Backlog — Medium, Low & Feature Work

> **Scope:** Items not in Sprint 1. Ordered by theme, then priority within each theme.
> **Source:** Consolidated findings from Architecture, Design, LLM Engine, and Product reviews.

---

## 🟡 Medium Priority — Code Quality & Robustness

### WI-100: Add test coverage for OpenAI provider
- **Files:** `internal/inference/openai.go` (new `openai_test.go`)
- **Problem:** Zero tests for the OpenAI provider. Streaming, tool calls, error handling, and token counting are all untested.
- **Scope:** HTTP mock server testing Complete(), StreamChat(), ListModels(), error responses, rate limiting, malformed JSON.

### WI-101: Add test coverage for SSE pipeline
- **Files:** `internal/streaming/sse.go` (new `sse_test.go`)
- **Problem:** The SSE pipeline has no tests. Backpressure, stall detection, event formatting, and error propagation are untested.
- **Scope:** Test all 8 event types, client disconnect handling, timeout behavior, malformed events.

### WI-102: Add test coverage for registry
- **Files:** `internal/inference/registry.go` (new `registry_test.go`)
- **Problem:** Model resolution (prefix, catalog, default, fallback) has no tests.
- **Scope:** Test Register, Resolve (all paths), SetDefault, RefreshModelMap, ListAllModels, concurrent access.

### WI-103: Make ProviderRegistry an interface
- **Files:** `internal/inference/registry.go`, `internal/api/*.go`, `internal/server/server.go`
- **Problem:** `ProviderRegistry` is a concrete struct, making it impossible to mock in handler tests without real providers.
- **Fix:** Extract a `Registry` interface with `Resolve()`, `ListAllModels()`, `Providers()`. Handlers accept the interface.

### WI-104: Extract `accumulatingProvider` from handlers
- **Files:** `internal/api/handlers_sessions.go` → new `internal/inference/accumulator.go`
- **Problem:** The `accumulatingProvider` wrapper is defined inside the API handlers package. It's an inference concern, not an API concern.
- **Fix:** Move to `internal/inference/` as a reusable decorator.

### WI-105: Validate ChatCompletionRequest fields
- **Files:** `internal/api/handlers_chat.go`
- **Problem:** No validation on `messages` (could be empty), `temperature` (could be negative), `max_tokens` (could be zero or enormous).
- **Fix:** Validate: messages non-empty, valid roles, temperature 0-2, max_tokens within bounds.

### WI-106: Add database CHECK constraints
- **Files:** `internal/store/migrations/` (new migration)
- **Problem:** No constraints on `sessions.status`, `messages.role`. Invalid values can be inserted.
- **Fix:** Add `CHECK(status IN ('active','idle','archived'))`, `CHECK(role IN ('system','user','assistant','tool'))`.

### WI-107: Add message `created_at` index
- **Files:** `internal/store/migrations/` (new migration)
- **Problem:** `GetMessages` queries by `session_id + is_active + created_at` but there's no composite index.
- **Fix:** `CREATE INDEX idx_messages_session_active_created ON messages(session_id, is_active, created_at)`.

### WI-108: Handle `time.Parse` errors in DB scan helpers
- **Files:** `internal/store/sqlite.go`
- **Problem:** All timestamp parsing uses `time.Parse()` with errors silently discarded (zero time used on failure).
- **Fix:** Log parse errors. Consider storing as Unix timestamps instead of ISO strings.

### WI-109: Remove mock providers from production startup
- **Files:** `cmd/forge/main.go:79-86`
- **Problem:** When no real providers are configured, mock providers are registered and serve fake responses. This can confuse users.
- **Fix:** Only register mocks if `FORGE_DEV=true`. Otherwise, start with zero providers and return clear errors.

### WI-110: Add `Probe()` to InferenceProvider interface
- **Files:** `internal/inference/openai.go`, `internal/inference/registry.go`
- **Problem:** Only OllamaProvider has `Probe()`. The health endpoint can't check OpenAI-compatible provider connectivity.
- **Fix:** Add `Probe(ctx context.Context) bool` to `InferenceProvider` interface. Implement for OpenAI (HEAD request to base URL).

### WI-111: Add per-session inference lock
- **Files:** `internal/session/manager.go` or `internal/api/handlers_sessions.go`
- **Problem:** Two concurrent `POST /api/sessions/{id}/messages` can interleave, producing garbled conversation history.
- **Fix:** `sync.Map` of session ID → mutex. Lock during message send. Return `409 Conflict` if already in progress.

### WI-112: Improve token counting accuracy
- **Files:** `internal/inference/openai.go`, new `internal/inference/tokenizer.go`
- **Problem:** `len(content)/4` underestimates CJK by 50-75% and overestimates code/punctuation.
- **Fix:** Integrate `tiktoken-go` for OpenAI models. For Ollama, use `/api/tokenize` endpoint. Fall back to `len/4` with logged warning.

---

## 🟢 Low Priority — Polish & Nice-to-Have

### WI-200: Consistent response envelopes
- **Files:** `internal/api/handlers_chat.go`, `internal/api/handlers_sessions.go`
- **Problem:** Some endpoints return bare objects, others use `{data: [...]}` wrappers.
- **Fix:** Standardize: collections return `{data: [...], total, has_more}`, singles return the object directly (OpenAI convention).

### WI-201: Add `GET /api/config` endpoint
- **Files:** `internal/api/` (new handler), `pkg/types/health.go` (ConfigResponse already defined)
- **Problem:** `ConfigResponse` type exists in pkg/types but no handler serves it.
- **Fix:** Implement handler returning non-sensitive config (providers, default model, limits). Never expose API keys.

### WI-202: Add `GET /api/providers` endpoint
- **Files:** `internal/api/` (new handler), `pkg/types/provider.go` (ProviderInfo already defined)
- **Problem:** `ProviderInfo` type exists but no endpoint lists available providers.
- **Fix:** List providers with name, status, model count. Never expose API keys (type already has `json:"-"` tag).

### WI-203: Content-Type enforcement on all mutation endpoints
- **Files:** `internal/api/handlers_sessions.go`
- **Problem:** POST/PATCH endpoints don't validate Content-Type. Missing header is accepted.
- **Fix:** Chi middleware or per-handler check requiring `application/json` on POST/PATCH/PUT.

### WI-204: Add retry jitter randomization
- **Files:** `internal/inference/retry.go`
- **Problem:** Retry backoff uses fixed exponential intervals. Multiple failing requests retry in lockstep.
- **Fix:** Add ±25% random jitter to backoff intervals.

### WI-205: Replace hardcoded user ID
- **Files:** `internal/session/manager.go`
- **Problem:** User ID is hardcoded (e.g., `"default"` or empty). Multi-user support will require refactoring.
- **Fix:** Accept user ID from auth middleware context. Keep single-user default for no-auth mode.

### WI-206: Add `Vary: Origin` header to CORS
- **Files:** `internal/server/server.go`
- **Problem:** Missing `Vary: Origin` causes incorrect caching when multiple origins are configured.
- **Fix:** Add `Vary: Origin` when reflecting a specific origin (not `*`).

---

## 📦 Feature Backlog — Future Phases

### FB-001: Frontend — Chat UI (Phase B)
- **Priority:** Next major milestone
- **Scope:** React 19 + Vite + Tailwind CSS v4, embedded via `go:embed`
- **Components:**
  - Message thread with streaming markdown renderer
  - Conversation sidebar (create, rename, delete, search)
  - Model selector dropdown
  - System prompt editor
  - Settings page (provider config, theme)
  - Copy / regenerate / edit message buttons
  - Dark/light theme toggle
  - Keyboard shortcuts (Cmd+N new chat, Cmd+K search, Enter send)
- **Acceptance:** User downloads binary, runs it, opens browser, and is chatting within 60 seconds.

### FB-002: Anthropic Provider
- **Priority:** High (different API format from OpenAI)
- **Scope:** Dedicated provider for Claude models. Messages API format, system prompt as top-level field, streaming via SSE with `content_block_delta` events.

### FB-003: Gemini Provider
- **Priority:** Medium
- **Scope:** Google Gemini API. Different content structure (`parts` array), safety settings, grounding.

### FB-004: Context Window Manager
- **Priority:** High (after WI-007 quick fix)
- **Scope:** Proper token budget system. Per-model context limits. Strategy selection (truncate oldest, summarize, sliding window). Token counting integration. Budget visualization for Inspector UI.

### FB-005: Compaction Engine
- **Priority:** Medium
- **Scope:** Rolling summarization for long conversations. Summarize older messages into a condensed system message. Preserve key facts, decisions, and code snippets.

### FB-006: Tool Sandbox
- **Priority:** Medium
- **Scope:** Pause-Execute-Resume tool execution loop. Sandboxed command execution. User approval flow. Tool result injection into conversation.

### FB-007: Event Bus + WebSocket
- **Priority:** Low (needed for Inspector)
- **Scope:** Pub/sub event bus for internal events (inference start/end, token usage, tool execution). WebSocket endpoint for real-time event streaming to Inspector UI.

### FB-008: Inspector UI
- **Priority:** Low
- **Scope:** Token visualization, raw context viewer, event stream timeline, provider request/response viewer. Debug tool for prompt engineering.

### FB-009: PostgreSQL Adapter
- **Priority:** Low (SQLite handles most use cases)
- **Scope:** Implement `Store` interface for PostgreSQL. Connection pooling. Schema migrations in SQL dialect. Use when `DATABASE_URL` env var is set.

### FB-010: Conversation Branching
- **Priority:** Low
- **Scope:** Tree-based message structure. Branch from any message. Navigate branches in UI. Requires schema migration from flat list to parent_id tree.

### FB-011: CI/CD Pipeline
- **Priority:** Medium
- **Scope:** GitHub Actions workflow: `go test -race ./...`, `go vet`, `golangci-lint`. Goreleaser for cross-platform binary builds. Docker image. Homebrew tap.

### FB-012: Rate Limiting
- **Priority:** Medium (for multi-user deployments)
- **Scope:** Per-IP and per-API-key rate limiting. `429 Too Slow` responses with `Retry-After` header. Configurable via env vars. Use `golang.org/x/time/rate`.

---

## Summary

| Category | Count | Sprint |
|----------|-------|--------|
| 🟡 Medium (code quality) | 13 | Sprint 2 |
| 🟢 Low (polish) | 7 | Sprint 3+ |
| 📦 Features | 12 | Phase B+ |
| **Total** | **32** | |
