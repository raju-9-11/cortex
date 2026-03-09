# Sprint 1 — Critical & High Priority Fixes

> **Goal:** Harden the backend for production readiness before starting frontend work.
> **Source:** Consolidated findings from Architecture, Design, LLM Engine, and Product reviews.

---

## 🔴 P0 — Critical (Security / Data Loss)

### WI-001: Add request body size limits
- **Severity:** Critical
- **Flagged by:** Architect, Design, LLM Engine
- **Files:** `internal/server/server.go`, `internal/api/handlers_chat.go:37`, `internal/api/handlers_sessions.go:85,158,216`
- **Problem:** `json.NewDecoder(r.Body).Decode()` performs unbounded reads. `MaxMessageSize` (100KB) is defined in config but never enforced. A multi-GB payload causes OOM.
- **Fix:** Wrap `r.Body` with `http.MaxBytesReader(w, r.Body, cfg.MaxMessageSize)` — either as chi middleware or at each handler entry point.
- **Acceptance:** Requests exceeding 100KB return `413 Request Entity Too Large` with structured error JSON.

### WI-002: Fix CORS multi-origin handling
- **Severity:** Critical
- **Flagged by:** Design
- **Files:** `internal/server/server.go:76-79`
- **Problem:** `Access-Control-Allow-Origin` header is set to comma-separated origins, which violates the CORS spec. Browsers reject all cross-origin requests when 2+ origins are configured.
- **Fix:** Check inbound `Origin` header against the allowed list. Reflect the matching origin (or omit the header if no match). Add `Vary: Origin` when reflecting.
- **Acceptance:** Multiple configured origins work correctly in browsers. Unrecognized origins are rejected.

### WI-003: Fix session/message ID collision risk
- **Severity:** Critical
- **Flagged by:** Architect, Design
- **Files:** `internal/session/manager.go:81,179`
- **Problem:** `uuid.NewString()[:8]` yields only 32 bits of entropy. Birthday-paradox collision probability reaches 50% at ~65K items.
- **Fix:** Use full UUID (`sess_<full-uuid>`, `msg_<full-uuid>`) or at minimum 16 hex chars.
- **Acceptance:** IDs are at least 128 bits. Existing data migration not required (new IDs only).

### WI-004: Fix streaming context lifecycle (data loss)
- **Severity:** Critical
- **Flagged by:** Architect, LLM Engine
- **Files:** `internal/api/handlers_sessions.go:316`
- **Problem:** After SSE streaming completes, `r.Context()` may already be cancelled (client disconnect). The assistant message save uses this context, silently dropping the message.
- **Fix:** Use `context.Background()` (or a detached context with timeout) for the post-stream database save.
- **Acceptance:** Assistant messages are persisted even if the client disconnects mid-stream.

### WI-005: Fix MessageCount race condition
- **Severity:** Critical
- **Flagged by:** Architect, LLM Engine
- **Files:** `internal/session/manager.go:188-199`, `internal/store/sqlite.go`
- **Problem:** `AddMessage` reads `session.MessageCount`, increments in Go, then writes back. Two concurrent messages produce a lost update.
- **Fix:** Use atomic SQL: `UPDATE sessions SET message_count = message_count + 1 WHERE id = ?`
- **Acceptance:** Concurrent message sends produce correct count. Add a test with goroutines.

### WI-006: Sanitize error messages (info leak)
- **Severity:** Critical
- **Flagged by:** Design, LLM Engine
- **Files:** `internal/api/handlers_chat.go`, `internal/api/handlers_sessions.go`, `internal/api/handlers_health.go` (14 error paths)
- **Problem:** Error responses concatenate `err.Error()` directly, exposing SQLite file paths, internal URLs, and driver-level messages to clients.
- **Fix:** Log full error server-side. Return generic client-safe messages (e.g., "Internal server error", "Session not found"). Map known errors (ErrNotFound, validation) to specific messages.
- **Acceptance:** No internal paths, driver names, or stack traces appear in any API response.

---

## 🟠 P1 — High (Functionality / Resilience)

### WI-007: Add context window truncation
- **Severity:** High
- **Flagged by:** Product Manager, LLM Engine
- **Files:** `internal/api/handlers_sessions.go` (message sending), new `internal/inference/context.go`
- **Problem:** Long conversations send ALL messages to the provider. This will exceed context limits and cause failures or silent truncation by the provider.
- **Fix:** Before inference, estimate total tokens. If over 80% of model's context window, drop oldest messages (keeping system prompt + last N). Log when truncation occurs.
- **Acceptance:** A 200-message conversation doesn't crash the provider. Truncation is logged.

### WI-008: Wire retry logic to providers
- **Severity:** High
- **Flagged by:** Architect, LLM Engine
- **Files:** `internal/inference/retry.go`, `internal/inference/openai.go`, `internal/inference/ollama.go`
- **Problem:** `retryDo()` (111 lines of exponential backoff with jitter) exists but is never called. All HTTP requests fail on the first transient error (429, 502, 503).
- **Fix:** Wrap `Complete()` calls in `retryDo()`. Streaming calls should NOT retry (not idempotent mid-stream). Respect `Retry-After` header for 429s.
- **Acceptance:** A single 503 from a provider doesn't fail the request. 429s back off correctly. Streaming is not retried.

### WI-009: Add pagination to session and message endpoints
- **Severity:** High
- **Flagged by:** Design, Product Manager
- **Files:** `internal/api/handlers_sessions.go`, `internal/store/store.go`, `internal/store/sqlite.go`
- **Problem:** `GET /api/sessions` and `GET /api/sessions/{id}` return all results unbounded. `SessionListResponse` has `total`/`has_more` fields but they're hardcoded.
- **Fix:** Accept `?limit=` and `?offset=` query params. Wire to store's existing `ListParams`. Return accurate `total` and `has_more`. Default limit: 50, max: 100.
- **Acceptance:** Large session lists paginate correctly. Messages within a session paginate. `has_more` is accurate.

### WI-010: Add SSE `id` and `created` fields
- **Severity:** High
- **Flagged by:** Design, LLM Engine
- **Files:** `internal/streaming/sse.go`, `internal/inference/openai.go`
- **Problem:** SSE chunks are missing `id` and `created` fields that OpenAI-compatible clients expect. Zero values break libraries like Vercel AI SDK.
- **Fix:** Generate a unique `chatcmpl-<id>` per stream. Set `created` to `time.Now().Unix()` at stream start. Include both in every chunk.
- **Acceptance:** Vercel AI SDK and OpenAI Python SDK parse streaming responses without errors.

### WI-011: Validate SendMessage role (injection prevention)
- **Severity:** High
- **Flagged by:** Design, LLM Engine
- **Files:** `internal/api/handlers_sessions.go`
- **Problem:** `SendMessageRequest.Role` accepts any value, allowing clients to inject `system` or `assistant` messages into conversations.
- **Fix:** Reject roles other than `"user"` on the send-message endpoint. System prompts are set via session config only.
- **Acceptance:** `POST /api/sessions/{id}/messages` with `role: "system"` returns 400.

### WI-012: Add OpenAI stream error event handling
- **Severity:** High
- **Flagged by:** LLM Engine
- **Files:** `internal/inference/openai.go:263`
- **Problem:** When the OpenAI stream scanner encounters an I/O error, it returns without sending an error event. The client sees a clean stream termination instead of an error.
- **Fix:** On `scanner.Err() != nil`, send an error `StreamEvent` through the channel before closing.
- **Acceptance:** Network errors during streaming produce a visible error event on the client side.

### WI-013: Migrate to structured logging (`slog`)
- **Severity:** High
- **Flagged by:** Product Manager, Architect
- **Files:** All files using `log.Printf` or `log.Fatalf`
- **Problem:** `log.Printf` produces unstructured text with no levels, no request IDs, no JSON output. Impossible to parse in production log aggregators.
- **Fix:** Replace with `log/slog` (stdlib). Use `slog.Info`, `slog.Error`, `slog.Warn`. Respect `CORTEX_LOG_FORMAT` config for JSON vs text. Add request ID from chi middleware.
- **Acceptance:** All log output is structured. JSON format works. Request IDs appear in request-scoped logs.

### WI-014: Add HTTP server timeouts
- **Severity:** High
- **Flagged by:** LLM Engine
- **Files:** `internal/server/server.go:64-67`
- **Problem:** `http.Server` has no `ReadHeaderTimeout`, `ReadTimeout`, or `IdleTimeout`. Slowloris attacks can exhaust connections.
- **Fix:** Set `ReadHeaderTimeout: 10s`, `IdleTimeout: 120s`. Do NOT set `ReadTimeout` or `WriteTimeout` globally (breaks SSE streaming). For non-streaming endpoints, use per-handler `http.TimeoutHandler`.
- **Acceptance:** Idle connections are reaped. SSE streams still work for long durations.

---

## Summary

| Priority | Count | Focus |
|----------|-------|-------|
| 🔴 P0 | 6 | Security, data integrity, spec compliance |
| 🟠 P1 | 8 | Functionality, resilience, production readiness |
| **Total** | **14** | |

**Recommended order:** WI-001 → WI-006 → WI-002 → WI-003 → WI-004 → WI-005 → WI-014 → WI-011 → WI-007 → WI-008 → WI-012 → WI-010 → WI-009 → WI-013
