# Forge: Architecture Implementation Plan

> **Scope:** Next-build modules — Storage Layer, Session Manager, Model Registry, Auth Middleware, Ollama Provider.
> **Audience:** Developer agent. Every interface, file path, SQL statement, and initialization line is implementation-ready.
> **Constraint:** All code must compile with `CGO_ENABLED=0`. No `mattn/go-sqlite3`.

---

## 1. Module Initialization Order & Build Waves

Modules are grouped into waves. Within a wave, all modules can be built **in parallel** — they share no compile-time dependency on each other. A module in Wave N+1 depends on at least one module from Wave N.

```
Wave 0 (EXISTS)
  ├── internal/config        ✅ done
  ├── internal/inference     ✅ done (interface + openai + mock)
  ├── internal/streaming     ✅ done
  ├── internal/server        ✅ done
  ├── internal/api           ✅ done
  └── pkg/types              ✅ done

Wave 1 (ZERO cross-dependencies — build in parallel)
  ├── pkg/types/errors.go          NEW  — structured error types
  ├── internal/store/store.go      NEW  — Store interface
  ├── internal/store/sqlite.go     NEW  — SQLite implementation
  ├── internal/store/migrations/   NEW  — embedded SQL
  ├── internal/auth/middleware.go   NEW  — no-op auth middleware
  └── internal/inference/ollama.go NEW  — Ollama provider

Wave 2 (depends on Wave 1: store.Store)
  ├── internal/session/manager.go  NEW  — SessionManager interface + impl
  └── internal/registry/registry.go NEW — ModelRegistry interface + impl

Wave 3 (depends on Wave 1 + 2: wiring)
  ├── internal/api/handlers_sessions.go  NEW  — Forge session CRUD API
  ├── internal/api/handlers_health.go    NEW  — /api/health endpoint
  ├── internal/server/server.go          EDIT — accept new dependencies
  └── cmd/forge/main.go                  EDIT — DI wiring for all new modules
```

### Compile-time dependency chain (import graph)

```
pkg/types             ← depends on nothing (stdlib only)
pkg/types/errors      ← depends on nothing

internal/config       ← depends on nothing (env lib)

internal/store        ← depends on pkg/types, internal/config
internal/auth         ← depends on internal/config
internal/inference/*  ← depends on pkg/types

internal/session      ← depends on internal/store, pkg/types
internal/registry     ← depends on internal/inference, pkg/types, internal/store

internal/api          ← depends on internal/session, internal/registry,
                         internal/inference, internal/streaming, pkg/types

internal/server       ← depends on internal/api, internal/config, internal/auth

cmd/forge/main.go     ← depends on everything (DI root)
```

---

## 2. Interface Contracts

### 2.1 `internal/store/store.go` — Storage Layer

This is the persistence boundary. Every other module talks to the database **only** through this interface. Two implementations: SQLite (Wave 1) and PostgreSQL (Phase 7, not in scope now).

```go
package store

import (
    "context"
    "time"
)

// ---- Domain Models (stored in this package, not pkg/types) ----

type Session struct {
    ID           string    `json:"id"`
    UserID       string    `json:"user_id"`
    Title        string    `json:"title"`
    Model        string    `json:"model"`
    SystemPrompt string    `json:"system_prompt"`
    Status       string    `json:"status"` // "active", "idle", "archived"
    TokenCount   int       `json:"token_count"`
    MessageCount int       `json:"message_count"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
    LastAccess   time.Time `json:"last_access"`
}

type Message struct {
    ID         string    `json:"id"`
    SessionID  string    `json:"session_id"`
    ParentID   *string   `json:"parent_id,omitempty"` // nil = root message
    Role       string    `json:"role"`                 // "system","user","assistant","tool"
    Content    string    `json:"content"`
    TokenCount int       `json:"token_count"`
    IsActive   bool      `json:"is_active"`            // false after compaction
    Pinned     bool      `json:"pinned"`
    Model      string    `json:"model,omitempty"`       // which model generated this
    Metadata   *string   `json:"metadata,omitempty"`    // JSON blob: tool_calls, etc.
    CreatedAt  time.Time `json:"created_at"`
}

type Provider struct {
    ID        string    `json:"id"`        // "ollama", "openai", etc.
    Type      string    `json:"type"`      // "ollama","openai","anthropic","gemini","openai_compat"
    BaseURL   string    `json:"base_url"`
    APIKey    string    `json:"-"`         // never serialized to JSON
    Enabled   bool      `json:"enabled"`
    CreatedAt time.Time `json:"created_at"`
}

// ---- Query Parameters ----

type SessionListParams struct {
    UserID string
    Status string // "" = all statuses
    Limit  int    // 0 = default (50)
    Offset int
}

type MessageListParams struct {
    SessionID  string
    ActiveOnly bool // true = only is_active=TRUE
    Limit      int  // 0 = all
    Offset     int
}

// ---- The Store Interface ----

type Store interface {
    // -- Lifecycle --
    // Migrate runs all pending migrations. Called once at startup.
    Migrate(ctx context.Context) error
    // Close gracefully shuts down the database connection.
    Close() error
    // Ping verifies database connectivity (for health checks).
    Ping(ctx context.Context) error

    // -- Sessions --
    CreateSession(ctx context.Context, s *Session) error
    GetSession(ctx context.Context, id string) (*Session, error)
    ListSessions(ctx context.Context, params SessionListParams) ([]Session, error)
    UpdateSession(ctx context.Context, s *Session) error
    DeleteSession(ctx context.Context, id string) error

    // -- Messages --
    CreateMessage(ctx context.Context, m *Message) error
    GetMessage(ctx context.Context, id string) (*Message, error)
    ListMessages(ctx context.Context, params MessageListParams) ([]Message, error)
    // DeactivateMessages sets is_active=FALSE for messages older than the
    // given message ID within a session. Used by the compaction engine.
    DeactivateMessages(ctx context.Context, sessionID string, messageIDs []string) error

    // -- Providers --
    UpsertProvider(ctx context.Context, p *Provider) error
    GetProvider(ctx context.Context, id string) (*Provider, error)
    ListProviders(ctx context.Context) ([]Provider, error)
    DeleteProvider(ctx context.Context, id string) error
}
```

**Critical implementation notes for `internal/store/sqlite.go`:**
- Use `modernc.org/sqlite` (import as `_ "modernc.org/sqlite"`).
- Open with `?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON`.
- Use **two** `*sql.DB` handles: one for writes (`SetMaxOpenConns(1)`), one for reads (`SetMaxOpenConns(4)`).
- ID generation: use `github.com/google/uuid` or `crypto/rand` to produce `msg_xxxx` / `sess_xxxx` prefixed IDs. The **caller** sets the ID before calling `CreateSession`/`CreateMessage` — the store does NOT generate IDs.
- `Migrate()` reads embedded SQL files in lexicographic order, checks `_migrations` table, applies only unapplied ones, within a transaction.

---

### 2.2 `internal/session/manager.go` — Session Manager

The Session Manager owns the **business logic** for sessions. It calls `store.Store` for persistence and handles ID generation, default values, timestamp management, and validation. API handlers call the Session Manager — never the Store directly.

```go
package session

import (
    "context"
    "forge/internal/store"
)

// Manager is the session lifecycle controller.
type Manager interface {
    // Create creates a new session with generated ID and defaults.
    // The caller provides Model and optionally Title/SystemPrompt.
    Create(ctx context.Context, params CreateParams) (*store.Session, error)

    // Get returns a session by ID. Returns ErrSessionNotFound if missing.
    Get(ctx context.Context, id string) (*store.Session, error)

    // List returns sessions for a user, ordered by last_access DESC.
    List(ctx context.Context, userID string) ([]store.Session, error)

    // Update updates mutable fields (title, model, system_prompt, status).
    // Only non-zero fields in UpdateParams are applied.
    Update(ctx context.Context, id string, params UpdateParams) (*store.Session, error)

    // Delete permanently removes a session and all its messages (CASCADE).
    Delete(ctx context.Context, id string) error

    // AppendMessage adds a message to a session. Generates message ID,
    // updates session.message_count, session.token_count, session.updated_at.
    AppendMessage(ctx context.Context, sessionID string, msg MessageInput) (*store.Message, error)

    // GetMessages returns active messages for a session, ordered by created_at ASC.
    GetMessages(ctx context.Context, sessionID string) ([]store.Message, error)

    // Touch updates session.last_access to now. Called on every interaction.
    Touch(ctx context.Context, sessionID string) error
}

type CreateParams struct {
    UserID       string // defaults to "default" if empty
    Title        string // defaults to "New Chat"
    Model        string // required
    SystemPrompt string
}

type UpdateParams struct {
    Title        *string // nil = don't update
    Model        *string
    SystemPrompt *string
    Status       *string
}

type MessageInput struct {
    Role       string  // "user", "assistant", "system", "tool"
    Content    string
    ParentID   *string
    Model      string  // which model generated this (empty for user messages)
    TokenCount int     // caller must count before appending
    Metadata   *string // JSON blob for tool_calls, etc.
}
```

**Implementation notes for `internal/session/manager_impl.go`:**
- The struct `DefaultManager` takes a `store.Store` in its constructor.
- ID generation: `sess_` prefix + 20 random hex chars for sessions, `msg_` prefix + 20 random hex chars for messages.
- `Create` sets `Status = "active"`, `CreatedAt/UpdatedAt/LastAccess = time.Now()`.
- `AppendMessage` must: (1) verify session exists, (2) create the message, (3) increment `session.message_count` and add `msg.TokenCount` to `session.token_count`, (4) update `session.updated_at`. This should be a **single transaction** on the store — but since the Store interface is per-operation, the manager calls `UpdateSession` after `CreateMessage`. If the update fails, the message is orphaned but recoverable. (A transaction-aware store is a Phase 7 enhancement.)
- `List` always queries with `userID` (hardcoded `"default"` in v1).

---

### 2.3 `internal/registry/registry.go` — Model Registry

The Model Registry discovers models from all configured providers, caches the catalog, and resolves model-name → provider routing. It replaces the current ad-hoc `getProviderForModel()` in `handlers_chat.go`.

```go
package registry

import (
    "context"
    "forge/internal/inference"
    "forge/pkg/types"
)

// ModelEntry is a model with its resolved provider.
type ModelEntry struct {
    Info     types.ModelInfo
    Provider inference.InferenceProvider
}

// Registry discovers, caches, and routes to model providers.
type Registry interface {
    // Refresh re-queries all providers for their model lists.
    // Called at startup and periodically (every 60s) or on-demand.
    Refresh(ctx context.Context) error

    // ListModels returns all known models across all providers.
    ListModels() []types.ModelInfo

    // Resolve maps a model ID to its provider. Returns the provider and
    // the canonical model ID (which may differ from the input if aliased).
    // Returns ErrModelNotFound if no provider claims the model.
    Resolve(modelID string) (inference.InferenceProvider, string, error)

    // RegisterProvider adds a provider to the registry. Called during
    // startup wiring. The registry will query it during Refresh().
    RegisterProvider(provider inference.InferenceProvider)

    // GetProvider returns a provider by name.
    GetProvider(name string) (inference.InferenceProvider, bool)

    // ProviderNames returns all registered provider names.
    ProviderNames() []string
}
```

**Implementation notes for `internal/registry/registry_impl.go`:**
- The struct `DefaultRegistry` holds a `map[string]inference.InferenceProvider` (by provider name) and a cached `[]ModelEntry` protected by `sync.RWMutex`.
- `Refresh()` iterates all providers, calls `ListModels(ctx)`, aggregates results. Failures for individual providers are logged but do not fail the whole refresh (other providers' models remain available).
- `Resolve()` lookup order: (1) exact match on `ModelInfo.ID`, (2) prefix match `providerName/modelID` → strip prefix and delegate, (3) if exactly one provider, route to it.
- Start a background goroutine in `NewRegistry()` that calls `Refresh()` every 60 seconds. Accept a `context.Context` for shutdown.
- The registry does **not** persist to the database in v1. It's in-memory only, rebuilt on startup.

---

### 2.4 `internal/auth/middleware.go` — Auth Middleware

```go
package auth

import (
    "net/http"
    "forge/internal/config"
)

// Middleware returns an http.Handler middleware that enforces API key
// authentication when FORGE_API_KEY is set. When the key is empty,
// all requests pass through (local mode).
//
// Authentication is via Bearer token in the Authorization header:
//   Authorization: Bearer <FORGE_API_KEY>
//
// Returns 401 Unauthorized with JSON error body on failure.
func Middleware(cfg *config.Config) func(http.Handler) http.Handler
```

**Implementation notes:**
- If `cfg.APIKey == ""`, return a passthrough (no-op) middleware.
- If `cfg.APIKey != ""`, extract the Bearer token and compare with constant-time comparison (`crypto/subtle.ConstantTimeCompare`).
- On failure: `{"error": {"message": "Invalid API key", "type": "auth_error", "code": 401}}`.
- Apply to `/v1/*` and `/api/*` routes. Do NOT apply to `/chat` (UI), `/ws` (WebSocket — uses separate auth), or `/api/health`.

---

### 2.5 `internal/inference/ollama.go` — Ollama Provider

```go
package inference

// OllamaProvider implements InferenceProvider for a local Ollama server.
// Key differences from OpenAI-compatible:
//   - Streaming is NDJSON (one JSON object per line), NOT SSE.
//   - Model list endpoint is GET /api/tags (not /v1/models).
//   - Chat endpoint is POST /api/chat (not /v1/chat/completions).
//   - Request/response schema differs from OpenAI.
//
// Constructor:
//   func NewOllamaProvider(baseURL string) *OllamaProvider
//
// The provider auto-probes the baseURL on construction:
//   GET {baseURL}/api/version → if 200, Ollama is available.
//
// Ollama request body for /api/chat:
//   {
//     "model": "llama3.2:1b",
//     "messages": [{"role":"user","content":"hello"}],
//     "stream": true,
//     "options": {"temperature": 0.7, "num_predict": 1024}
//   }
//
// Ollama streaming response (NDJSON, one line per chunk):
//   {"model":"llama3.2:1b","message":{"role":"assistant","content":"Hi"},"done":false}
//   {"model":"llama3.2:1b","message":{"role":"assistant","content":"!"},"done":true,
//    "total_duration":..., "eval_count":42}
//
// Ollama model list (GET /api/tags):
//   {"models":[{"name":"llama3.2:1b","size":1234567,...}]}
//
// CountTokens: POST /api/show with the model name to get context_length,
//   then estimate: len(content) / 3.5 rounded up.
//
// Name() returns "ollama".
```

---

## 3. Package Structure — New Files

```
forge/
├── internal/
│   ├── store/                          # NEW PACKAGE
│   │   ├── store.go                    # Store interface + domain models
│   │   ├── sqlite.go                   # SQLite implementation
│   │   └── migrations/
│   │       └── 001_init.sql            # First migration (sessions, messages, providers)
│   │
│   ├── session/                        # NEW PACKAGE
│   │   ├── manager.go                  # Manager interface + CreateParams/UpdateParams
│   │   └── manager_impl.go            # DefaultManager implementation
│   │
│   ├── registry/                       # NEW PACKAGE
│   │   ├── registry.go                 # Registry interface + ModelEntry
│   │   └── registry_impl.go           # DefaultRegistry implementation
│   │
│   ├── auth/                           # NEW PACKAGE
│   │   └── middleware.go               # API key auth middleware
│   │
│   ├── inference/
│   │   └── ollama.go                   # NEW FILE — Ollama provider
│   │
│   ├── api/
│   │   ├── handlers_chat.go            # EDIT — use registry.Resolve() instead of getProviderForModel()
│   │   ├── handlers_sessions.go        # NEW FILE — /api/sessions CRUD
│   │   └── handlers_health.go          # NEW FILE — /api/health
│   │
│   └── server/
│       └── server.go                   # EDIT — accept Store, SessionManager, Registry, auth middleware
│
├── pkg/
│   └── types/
│       └── errors.go                   # NEW FILE — structured error types
│
├── cmd/
│   └── forge/
│       └── main.go                     # EDIT — full DI wiring
│
└── go.mod                              # EDIT — add modernc.org/sqlite, google/uuid
```

**Total: 11 new files, 4 edited files, 3 new packages.**

---

## 4. Dependency Graph

```
                    ┌─────────────┐
                    │  pkg/types  │ (exists)
                    │  + errors   │ (new file)
                    └──────┬──────┘
                           │ imported by all
              ┌────────────┼────────────────┐
              │            │                │
     ┌────────▼───┐  ┌─────▼──────┐  ┌──────▼──────┐
     │  internal/  │  │ internal/  │  │  internal/  │
     │   store     │  │ inference  │  │   config    │
     │  (Wave 1)   │  │ + ollama   │  │  (exists)   │
     └──────┬──────┘  │  (Wave 1)  │  └──────┬──────┘
            │         └─────┬──────┘         │
            │               │                │
    ┌───────┼───────────────┼────────────────┘
    │       │               │
    │  ┌────▼────┐    ┌─────▼──────┐    ┌──────────┐
    │  │internal/ │    │ internal/  │    │ internal/│
    │  │ session  │    │  registry  │    │   auth   │
    │  │(Wave 2)  │    │ (Wave 2)   │    │ (Wave 1) │
    │  └────┬─────┘    └─────┬──────┘    └────┬─────┘
    │       │                │                │
    │       └────────┬───────┘                │
    │                │                        │
    │         ┌──────▼──────┐                 │
    │         │  internal/  │                 │
    │         │    api      │                 │
    │         │  (Wave 3)   │                 │
    │         └──────┬──────┘                 │
    │                │                        │
    │         ┌──────▼──────┐                 │
    │         │  internal/  │◄────────────────┘
    │         │   server    │
    │         │  (Wave 3)   │
    │         └──────┬──────┘
    │                │
    │         ┌──────▼──────┐
    └────────►│  cmd/forge  │
              │  main.go    │
              │  (Wave 3)   │
              └─────────────┘
```

### What can be built in parallel:

| Wave | Modules (all parallel within wave) | Estimated effort |
|:-----|:-----------------------------------|:-----------------|
| **1** | `store/store.go`, `store/sqlite.go`, `store/migrations/001_init.sql`, `auth/middleware.go`, `inference/ollama.go`, `pkg/types/errors.go` | ~4 hours |
| **2** | `session/manager.go`, `session/manager_impl.go`, `registry/registry.go`, `registry/registry_impl.go` | ~3 hours |
| **3** | `api/handlers_sessions.go`, `api/handlers_health.go`, EDIT `api/handlers_chat.go`, EDIT `server/server.go`, EDIT `cmd/forge/main.go` | ~3 hours |

---

## 5. Integration Points

### 5.1 `cmd/forge/main.go` — After all waves complete

```go
package main

import (
    "context"
    "log"

    "forge/internal/auth"
    "forge/internal/config"
    "forge/internal/inference"
    "forge/internal/registry"
    "forge/internal/server"
    "forge/internal/session"
    "forge/internal/store"
)

var version = "dev"

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    cfg.Version = version

    // ---- Wave 1: Storage ----
    db, err := store.NewSQLiteStore(cfg.SQLitePath)
    if err != nil {
        log.Fatalf("Failed to open database: %v", err)
    }
    defer db.Close()

    if err := db.Migrate(context.Background()); err != nil {
        log.Fatalf("Failed to run migrations: %v", err)
    }

    // ---- Wave 1: Auth ----
    authMiddleware := auth.Middleware(cfg)

    // ---- Wave 1: Providers ----
    reg := registry.NewRegistry()

    // Always try Ollama (zero-config local provider)
    ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
    reg.RegisterProvider(ollamaProvider)

    // Cloud providers (only if API keys are configured)
    if cfg.OpenAIKey != "" {
        reg.RegisterProvider(
            inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey),
        )
    }
    if cfg.QwenKey != "" {
        reg.RegisterProvider(
            inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey),
        )
    }
    if cfg.LlamaKey != "" {
        reg.RegisterProvider(
            inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey),
        )
    }
    if cfg.MinimaxKey != "" {
        reg.RegisterProvider(
            inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey),
        )
    }
    if cfg.OSSKey != "" {
        reg.RegisterProvider(
            inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey),
        )
    }

    // If NO providers are configured at all, add mocks for testing
    if len(reg.ProviderNames()) == 0 {
        reg.RegisterProvider(inference.NewMockProvider("mock", nil))
    }

    // Initial model discovery
    if err := reg.Refresh(context.Background()); err != nil {
        log.Printf("Warning: initial model refresh failed: %v", err)
    }

    // ---- Wave 2: Session Manager ----
    sessionMgr := session.NewManager(db)

    // ---- Wave 3: Server (wires everything together) ----
    srv := server.New(cfg, server.Dependencies{
        Store:          db,
        SessionManager: sessionMgr,
        Registry:       reg,
        AuthMiddleware: authMiddleware,
    })
    srv.StartAndServe()
}
```

### 5.2 `internal/server/server.go` — Updated constructor signature

```go
package server

// Dependencies bundles all injected dependencies for the server.
type Dependencies struct {
    Store          store.Store
    SessionManager session.Manager
    Registry       registry.Registry
    AuthMiddleware func(http.Handler) http.Handler
}

func New(cfg *config.Config, deps Dependencies) *Server {
    apiRouter := api.NewRouter(api.RouterDeps{
        Registry:       deps.Registry,
        SessionManager: deps.SessionManager,
        Store:          deps.Store,
    })

    r := chi.NewRouter()

    // Global middleware (all routes)
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(corsMiddleware(cfg))

    // Public routes (no auth)
    r.Get("/api/health", apiRouter.HandleHealth)

    // Authenticated routes
    r.Group(func(r chi.Router) {
        r.Use(deps.AuthMiddleware)

        // OpenAI-compatible
        r.Post("/v1/chat/completions", apiRouter.HandleChatCompletions)
        r.Get("/v1/models", apiRouter.HandleModels)

        // Forge-native session API
        r.Route("/api/sessions", func(r chi.Router) {
            r.Get("/", apiRouter.HandleListSessions)
            r.Post("/", apiRouter.HandleCreateSession)
            r.Get("/{id}", apiRouter.HandleGetSession)
            r.Patch("/{id}", apiRouter.HandleUpdateSession)
            r.Delete("/{id}", apiRouter.HandleDeleteSession)
        })
    })

    return &Server{
        cfg:    cfg,
        httpServer: &http.Server{
            Addr:    cfg.Addr,
            Handler: r,
        },
        store: deps.Store,
    }
}
```

### 5.3 `internal/api/handlers_chat.go` — Updated to use Registry

The key change: replace the ad-hoc `getProviderForModel()` with `registry.Resolve()`.

```go
// BEFORE (current code):
provider := router.getProviderForModel(req.Model)

// AFTER:
provider, resolvedModel, err := router.deps.Registry.Resolve(req.Model)
if err != nil {
    writeJSONError(w, http.StatusNotFound, "model_not_found",
        fmt.Sprintf("No provider found for model %q", req.Model))
    return
}
req.Model = resolvedModel // use the canonical model ID
```

The `HandleModels` handler similarly changes to `router.deps.Registry.ListModels()`.

### 5.4 `internal/api/` — New Router struct

```go
package api

type RouterDeps struct {
    Registry       registry.Registry
    SessionManager session.Manager
    Store          store.Store
}

type Router struct {
    deps RouterDeps
}

func NewRouter(deps RouterDeps) *Router {
    return &Router{deps: deps}
}
```

---

## 6. Database Schema — First Migration

### `internal/store/migrations/001_init.sql`

```sql
-- 001_init.sql: Core tables for Forge v1
-- Applied by store.Migrate() on first startup.

-- Sessions table: one row per conversation.
CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL DEFAULT 'default',
    title         TEXT NOT NULL DEFAULT 'New Chat',
    model         TEXT NOT NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'idle', 'archived')),
    token_count   INTEGER NOT NULL DEFAULT 0,
    message_count INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_access   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Messages table: one row per message in a conversation.
-- parent_id enables tree-structured branching (Phase 8).
-- is_active=FALSE marks compacted (archived) messages.
CREATE TABLE IF NOT EXISTS messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    parent_id   TEXT,
    role        TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content     TEXT NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    is_active   INTEGER NOT NULL DEFAULT 1,
    pinned      INTEGER NOT NULL DEFAULT 0,
    model       TEXT,
    metadata    TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Providers table: tracks configured inference providers.
-- API keys stored here are encrypted at the application layer.
CREATE TABLE IF NOT EXISTS providers (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL CHECK (type IN (
                   'ollama', 'openai', 'anthropic', 'gemini', 'openai_compat'
               )),
    base_url   TEXT NOT NULL,
    api_key    TEXT,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Migration tracking table.
CREATE TABLE IF NOT EXISTS _migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Indexes for common query patterns.
CREATE INDEX IF NOT EXISTS idx_messages_session_active
    ON messages(session_id, is_active);

CREATE INDEX IF NOT EXISTS idx_messages_session_created
    ON messages(session_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_sessions_user_status
    ON sessions(user_id, status);

CREATE INDEX IF NOT EXISTS idx_sessions_user_lastaccess
    ON sessions(user_id, last_access DESC);
```

**Why TEXT for timestamps (not TIMESTAMP)?**
SQLite has no native datetime type. `TEXT` with ISO-8601 format is the canonical approach. The Go driver parses these via `time.Parse(time.RFC3339Nano, ...)`. Using `strftime` with explicit format ensures consistency regardless of SQLite build options.

**Why INTEGER for booleans (not BOOLEAN)?**
SQLite stores BOOLEAN as INTEGER internally. Being explicit avoids driver confusion with `modernc.org/sqlite`.

---

## 7. `pkg/types/errors.go` — Structured Error Types

```go
package types

import "fmt"

// Sentinel errors for cross-package error checking.

type NotFoundError struct {
    Resource string // "session", "message", "model", "provider"
    ID       string
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// APIError is the JSON error response body (OpenAI-compatible format).
type APIError struct {
    Error APIErrorDetail `json:"error"`
}

type APIErrorDetail struct {
    Message string `json:"message"`
    Type    string `json:"type"`
    Code    int    `json:"code"`
}
```

---

## 8. `internal/api/handlers_sessions.go` — Session CRUD API

```
POST   /api/sessions                → HandleCreateSession
GET    /api/sessions                → HandleListSessions
GET    /api/sessions/{id}           → HandleGetSession
PATCH  /api/sessions/{id}           → HandleUpdateSession
DELETE /api/sessions/{id}           → HandleDeleteSession
```

### Request/Response contracts:

**POST /api/sessions**
```json
// Request:
{
    "model": "llama3.2:1b",
    "title": "Optional title",
    "system_prompt": "Optional"
}
// Response: 201 Created
{
    "id": "sess_a1b2c3d4e5f6...",
    "user_id": "default",
    "title": "New Chat",
    "model": "llama3.2:1b",
    "status": "active",
    "created_at": "2025-07-15T10:00:00.000Z"
}
```

**GET /api/sessions**
```json
// Response: 200 OK
{
    "sessions": [
        { "id": "sess_...", "title": "...", "model": "...", ... }
    ]
}
```

**GET /api/sessions/{id}**
```json
// Response: 200 OK — includes messages
{
    "session": { "id": "sess_...", "title": "...", ... },
    "messages": [
        { "id": "msg_...", "role": "user", "content": "hello", ... },
        { "id": "msg_...", "role": "assistant", "content": "Hi!", ... }
    ]
}
```

**PATCH /api/sessions/{id}**
```json
// Request (partial update):
{ "title": "New Title" }
// Response: 200 OK — full session object
```

**DELETE /api/sessions/{id}**
```json
// Response: 204 No Content
```

---

## 9. `internal/api/handlers_health.go` — Health Check

**GET /api/health**
```json
// Response: 200 OK
{
    "status": "ok",
    "version": "dev",
    "database": "ok",
    "providers": {
        "ollama": "connected",
        "openai": "configured"
    },
    "uptime_seconds": 3600
}
```

Implementation: call `store.Ping(ctx)` for DB status. Call `registry.ProviderNames()` and attempt a lightweight check for each.

---

## 10. `go.mod` Changes

Add these dependencies:

```
modernc.org/sqlite          // SQLite driver (pure Go, CGO_ENABLED=0)
github.com/google/uuid      // ID generation (or use crypto/rand directly)
```

Do NOT add `pgx` yet — that's Phase 7.

---

## 11. SQLite Store Implementation Skeleton

### `internal/store/sqlite.go` — Key patterns

```go
package store

import (
    "context"
    "database/sql"
    "embed"
    "fmt"
    "sort"
    "strings"

    _ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLiteStore struct {
    writer *sql.DB // MaxOpenConns=1 (serialized writes)
    reader *sql.DB // MaxOpenConns=4 (concurrent reads)
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
    dsn := fmt.Sprintf(
        "file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON",
        path,
    )

    writer, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, fmt.Errorf("opening sqlite writer: %w", err)
    }
    writer.SetMaxOpenConns(1)

    reader, err := sql.Open("sqlite", dsn)
    if err != nil {
        writer.Close()
        return nil, fmt.Errorf("opening sqlite reader: %w", err)
    }
    reader.SetMaxOpenConns(4)

    return &SQLiteStore{writer: writer, reader: reader}, nil
}

func (s *SQLiteStore) Close() error {
    s.reader.Close()
    return s.writer.Close()
}

func (s *SQLiteStore) Ping(ctx context.Context) error {
    return s.reader.PingContext(ctx)
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
    // 1. Create _migrations table if not exists (using writer)
    // 2. Read all files from migrationsFS, sort lexicographically
    // 3. For each file, check if version exists in _migrations
    // 4. If not, execute the SQL within a transaction and record it
    // ... (implementation details for developer)
    return nil
}

// All write operations use s.writer
// All read operations use s.reader
// All methods accept context.Context for cancellation
```

---

## 12. Critical Implementation Constraints

1. **No circular imports.** The dependency DAG in §4 is law. If you find yourself importing `internal/api` from `internal/session`, you have a design error.

2. **`user_id = "default"` everywhere in v1.** Every query that touches `sessions` must filter by `user_id`. This is annoying now but prevents a migration nightmare when multi-user lands.

3. **IDs are caller-generated, not DB-generated.** The `Store` interface never returns a generated ID. The `SessionManager` generates `sess_xxx` / `msg_xxx` IDs before calling `Store.CreateSession()` / `Store.CreateMessage()`. This makes the Store implementation simpler and testing easier.

4. **Timestamps are `time.Time` in Go, ISO-8601 TEXT in SQLite.** Parse with `time.Parse(time.RFC3339Nano, ...)` in the SQLite implementation. The PostgreSQL implementation (Phase 7) will use native `TIMESTAMPTZ`.

5. **The `Store` interface has NO transaction method in v1.** Multi-step operations (like "create message + update session counts") are not atomic. This is acceptable for a single-user local app. Phase 7 adds `WithTx(ctx, func(Store) error) error`.

6. **The Auth middleware is a `func(http.Handler) http.Handler`.** It plugs directly into Chi's middleware chain. It does not know about sessions, users, or the database.

7. **The Ollama provider must handle NDJSON streaming, not SSE.** The `StreamChat` implementation reads `bufio.Scanner` lines and JSON-decodes each line as an Ollama response object. Do NOT reuse the OpenAI SSE parsing logic.

8. **`Registry.Refresh()` must be non-blocking on provider failures.** If Ollama is down, the refresh logs a warning but continues. The registry keeps stale model data from the last successful refresh.

---

## 13. Verification Checklist

After all three waves are implemented, these must pass:

```bash
# 1. Binary starts with zero config and SQLite auto-creates
rm -f forge.db && go run ./cmd/forge
# → logs "Starting server on :8080"
# → forge.db file exists with tables

# 2. Health check works
curl http://localhost:8080/api/health
# → {"status":"ok","version":"dev","database":"ok",...}

# 3. Session CRUD works
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:1b"}'
# → 201 with session object

curl http://localhost:8080/api/sessions
# → {"sessions":[...]}

# 4. Models endpoint uses registry
curl http://localhost:8080/v1/models
# → {"object":"list","data":[...]} (includes ollama models if running)

# 5. Chat completion routes through registry
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"mock-model","messages":[{"role":"user","content":"hi"}],"stream":true}'
# → SSE stream

# 6. Auth blocks when API key is set
FORGE_API_KEY=secret go run ./cmd/forge &
curl http://localhost:8080/v1/models
# → 401 Unauthorized

curl -H "Authorization: Bearer secret" http://localhost:8080/v1/models
# → 200 OK

# 7. Compile with CGO_ENABLED=0
CGO_ENABLED=0 go build -o bin/forge ./cmd/forge
# → succeeds, produces static binary
```

---

*This document is the single source of truth for implementation. Do not deviate from the interface signatures, file paths, or SQL schema without updating this document first.*
