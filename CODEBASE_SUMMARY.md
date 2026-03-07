# FORGE CODEBASE — COMPLETE DESIGN & ROADMAP SUMMARY

## EXECUTIVE SUMMARY

**Forge** is a unified AI backend written in Go that provides a single static binary for LLM inference across multiple providers. It abstracts away provider differences through an OpenAI-compatible REST API, manages stateful conversation sessions with SQLite persistence, and powers both a React web UI (phase 2) and a TUI terminal client (forge-link, phase 3+).

### Current Status
- ✅ **Core inference engine working**: Ollama, OpenAI, Qwen, Llama, Minimax, OSS providers
- ✅ **OpenAI-compatible API** for chat completions and model listing
- ✅ **SSE streaming** with backpressure handling
- ✅ **SQLite persistence** (WAL mode, connection pooling)
- ✅ **Session management** CRUD scaffolding
- ❌ **Frontend integration** (missing most error handling & structured responses)
- ❌ **WebSocket support** (event types defined but not implemented)
- ❌ **Production hardening** (14 critical fixes needed)

---

## DOCUMENTED FILES

1. **README.md** (237 lines) — Quick start, features, architecture overview
2. **QUICK_REFERENCE.md** (321 lines) — Key file locations, stats, flow diagrams
3. **API_SPEC.md** (36 KB / 1300+ lines) — Complete API contract for frontend
4. **sprint-1.md** (136 lines) — 14 critical/high-priority fixes needed
5. **backlog.md** (537 lines) — Medium/low priority work items (100+ items)
6. **forge-link-prd.md** (495 lines) — TUI client product vision & requirements
7. **tui-design.md** (1095 lines) — TUI design specification & keybindings

---

## ARCHITECTURE LAYERS

### Layer 1: Frontend (Phase 2+)
- React web UI (3-column: sidebar, chat, inspector)
- forge-link terminal client (inline REPL)

### Layer 2: REST API

**OpenAI-Compatible (Existing):**
```
POST   /v1/chat/completions          (streaming & non-streaming)
GET    /v1/models                    (all providers)
```

**Forge-Native (NEW):**
```
Health & Config:
  GET  /api/health                   (status indicator)
  GET  /api/config                   (runtime config)

Session Management:
  GET/POST   /api/sessions           (list/create)
  GET/PATCH/DELETE /api/sessions/{id}

Messaging:
  POST /api/sessions/{id}/messages   (send message, stream response)
  POST /api/sessions/{id}/stop       (cancel inference)
  POST /api/sessions/{id}/regenerate (regenerate last response)

Provider Management:
  GET  /api/providers                (list configured)
  POST /api/providers/{id}/test      (test connection)
```

**WebSocket Events (Planned):**
- 14 event types: inference.*, session.*, model.status_changed, compaction.*

### Layer 3: Inference

**InferenceProvider Interface:**
```go
type InferenceProvider interface {
    StreamChat(ctx, req, out chan) error
    Complete(ctx, req) (*Response, error)
    CountTokens(messages) (int, error)
    Capabilities(model) ModelCapabilities
    ListModels(ctx) ([]ModelInfo, error)
    Name() string
}
```

**Implementations (6):**
1. Ollama — Auto-detected at http://localhost:11434
2. OpenAI — If OPENAI_API_KEY set
3. Qwen — OpenAI-compatible
4. Llama — OpenAI-compatible
5. Minimax — OpenAI-compatible
6. OSS + Mock — Fallback/testing

### Layer 4: Storage

**SQLite (Primary):**
- WAL mode for concurrent reads
- Dual pools: 1 writer + 4 readers
- Automatic migrations
- Foreign key constraints

**PostgreSQL (Future):**
- Switch via DATABASE_URL env var

---

## CRITICAL PRODUCTION ISSUES (Sprint 1)

### 🔴 P0 — CRITICAL (6 items):

| ID | Issue | Problem | Impact |
|----|-------|---------|--------|
| WI-001 | Request body limits | Unbounded reads | OOM on multi-GB |
| WI-002 | CORS multi-origin | Violates spec | Browsers reject |
| WI-003 | ID collision | 32-bit entropy | 50% collision @ 65K |
| WI-004 | Streaming context | Context cancelled after stream | Messages dropped |
| WI-005 | MessageCount race | Read-modify-write not atomic | Lost updates |
| WI-006 | Error sanitization | Exposes internal info | Info leak |

### 🟠 P1 — HIGH (8 items):

| ID | Issue | Fix |
|----|-------|-----|
| WI-007 | Context truncation | Drop old messages if >80% context |
| WI-008 | Retry logic unused | Wrap Complete() in retryDo() |
| WI-009 | No pagination | Add ?limit= and ?offset= |
| WI-010 | SSE fields missing | Add id and created to chunks |
| WI-011 | Role validation | Reject non-"user" roles |
| WI-012 | Stream errors | Send error events |
| WI-013 | Unstructured logging | Use log/slog |
| WI-014 | No timeouts | Set ReadHeaderTimeout, IdleTimeout |

**Recommended order:** 1→6→2→3→4→5→14→11→7→8→12→10→9→13

---

## DATA MODELS

### Session
```
id, user_id, title, model, system_prompt
status (active|idle|archived), token_count, message_count
created_at, updated_at, last_access
```

### Message
```
id, session_id, parent_id (for branching)
role (system|user|assistant|tool), content, token_count
is_active, pinned, model, metadata
metadata: { tool_calls[], finish_reason, usage, compaction_ref }
```

### ProviderInfo
```
id, type, base_url, enabled, status
models[], has_api_key (never expose), is_env_var
```

### Error Response (Standardized)
```json
{
  "error": {
    "code": "session_not_found",
    "message": "...",
    "type": "not_found",
    "param": "session_id",
    "details": null
  }
}
```

---

## ROADMAP

**Phase 1 (Current):** Backend Hardening
- Fix 14 critical/high-priority issues
- Standardized error handling
- Test coverage expansion

**Phase 2:** React Web UI
- 3-column chat interface
- Session/message CRUD
- Static file serving (embed.go)

**Phase 3:** forge-link TUI Client
- Inline scrolling REPL
- Session switching
- Streaming display

**Phase 4:** Advanced Features
- Compaction (LLM-based compression)
- Tool calling framework
- Multi-branch conversations
- Full-text search
- Anthropic support

**Phase 5:** Scaling
- PostgreSQL support
- Multi-user support
- Rate limiting
- Request logging

**Phase 6+:** Observability, fine-tuning, model management

---

## CONFIGURATION

### Server
- `FORGE_ADDR` (default: `:8080`)
- `FORGE_DEV` (default: `false`)
- `FORGE_API_KEY` (optional, enables auth)

### Database
- `DATABASE_URL` (PostgreSQL URL; overrides SQLite)
- `FORGE_DB_PATH` (default: `forge.db`)

### Providers
- `FORGE_PROVIDER` (default: `qwen`)
- `OLLAMA_URL` (default: `http://localhost:11434`)
- `OPENAI_API_KEY`, `OPENAI_BASE_URL`
- `QWEN_API_KEY`, `QWEN_BASE_URL`
- `LLAMA_API_KEY`, `LLAMA_BASE_URL`
- `MINIMAX_API_KEY`, `MINIMAX_BASE_URL`
- `OSS_API_KEY`, `OSS_BASE_URL`

### Limits
- `FORGE_MAX_TOOL_TIMEOUT` (default: `60s`)
- `FORGE_MAX_TOOL_OUTPUT` (default: `65536` bytes)
- `FORGE_MAX_MESSAGE_SIZE` (default: `102400` bytes)

### Logging
- `FORGE_LOG_LEVEL` (default: `info`)
- `FORGE_LOG_FORMAT` (default: `json`)
- `FORGE_CORS_ORIGINS` (default: `*`)

---

## TECHNOLOGY STACK

- **Language:** Go 1.24
- **HTTP Router:** chi/v5 (lightweight, composable)
- **Database:** modernc.org/sqlite (pure Go, no CGO)
- **Config:** caarlos0/env (struct-tag driven)
- **UUIDs:** google/uuid

**Direct Dependencies:** 2
- github.com/go-chi/chi/v5 v5.2.5
- github.com/caarlos0/env/v11 v11.4.0

**Key Stats:**
- Go Files: 31
- Lines of Code: 6,239
- Test Files: 6 (70 tests total)

---

## KEY FILE LOCATIONS

```
cmd/forge/main.go                    — Entry point, DI

internal/
  config/config.go                   — Configuration
  server/server.go                   — HTTP server, middleware
  api/
    handlers_chat.go                 — /v1/chat/completions, /v1/models
    handlers_sessions.go             — Session/message CRUD
    handlers_health.go               — /api/health
  inference/
    provider.go                      — Provider interface
    registry.go                      — Model resolution
    openai.go                        — OpenAI provider (265 lines)
    ollama.go                        — Ollama provider (584 lines)
    mock.go                          — Testing
    retry.go                         — Exponential backoff (50+ lines)
  store/
    store.go                         — Store interface
    sqlite.go                        — SQLite impl
    migrations/001_init.sql          — Schema
  session/
    manager.go                       — Session lifecycle
  auth/
    middleware.go                    — Auth validation
  streaming/
    sse.go                           — SSE pipeline (189 lines)

pkg/types/                           — All type definitions
```

---

## WORKITEMS SUMMARY

**sprint-1.md (136 lines):**
- 14 work items (P0 + P1)
- Detailed severity, files, acceptance criteria
- Ready for implementation

**backlog.md (537 lines):**
- Test coverage (OpenAI, SSE, registry)
- Refactoring (interfaces, validators)
- Database optimization

**forge-link-prd.md (495 lines):**
- Vision: "Your terminal's AI brain — any model, fully local or cloud"
- User personas: Solo dev, team lead, researcher, privacy-focused
- MVP: Sessions, streaming, multi-provider, inline REPL

**tui-design.md (1095 lines):**
- Design: Inline REPL (not full-screen)
- Message stream with streaming display
- Tool calls (compact → expandable)
- Status bar with token count + latency
- Keybindings: Ctrl+D (exit), Ctrl+C (stop), Ctrl+L (clear)

---

## DESIGN DECISIONS & RATIONALE

**Single Binary:**
- Why: Zero installation, works anywhere
- How: Pure Go + embedded SQLite
- Trade-off: Can't use native PostgreSQL (optional at scale)

**Multi-Provider:**
- Why: No vendor lock-in
- How: InferenceProvider interface + registry
- Trade-off: New provider requires code change

**SQLite Default:**
- Why: Single binary, zero setup, WAL mode
- Trade-off: Use PostgreSQL for enterprise scale

**OpenAI-Compatible API:**
- Why: Reuse integrations (Vercel AI SDK)
- Trade-off: Forge-specific features in /api/* namespace

**Session-First:**
- Why: Conversations are core use case
- Trade-off: Stateless inference requires separate endpoints

**Three-Column Web UI:**
- Why: Sidebar (sessions) + main (chat) + right (inspector)
- Trade-off: Requires frontend build step (React SPA)

**Inline REPL (TUI):**
- Why: Works everywhere (SSH, tmux), familiar
- Trade-off: Not a full-screen modal interface

---

## NEXT IMMEDIATE ACTIONS

**Week 1: Fix Critical Issues**
1. Standardized error format (pkg/types/errors.go)
2. Refactor handlers to use WriteError()
3. Request body size limits (WI-001)
4. Fix CORS (WI-002)
5. Fix ID generation (WI-003)

**Week 2: Resilience & Functionality**
6. Fix context lifecycle (WI-004)
7. Fix MessageCount race (WI-005)
8. Server timeouts (WI-014)
9. Pagination (WI-009)
10. SSE id/created fields (WI-010)

**Week 3: Features & Polish**
11. Context truncation (WI-007)
12. Wire retry logic (WI-008)
13. Structured logging (WI-013)
14. Validate request fields

**Week 4: Testing & Documentation**
15. Add test coverage
16. Update documentation
17. Prepare for frontend integration

---

## CONCLUSION

**Forge** is well-architected, well-documented, with clear vision and roadmap. The core inference engine is solid and production-ready in isolation.

**Current Gaps:**
- Production hardening (14 identified issues)
- Frontend integration (React web UI + TUI not started)
- Error standardization (partial implementation)

**Strengths:**
- Clean, separated concerns
- Comprehensive API specification
- Thorough planning (roadmap through Phase 6+)
- Good documentation
- Extensible architecture
- Single binary deployment model

**Status:** Ready for Phase 2 (React web UI) after Sprint 1 fixes complete.
