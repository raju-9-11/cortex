# Forge

Forge is a unified AI backend delivered as a **single static binary** written in Go. It abstracts LLM inference across multiple providers, manages stateful conversation sessions with SQLite persistence, and exposes an OpenAI-compatible API — all with zero external dependencies.

```
🔥 Forge dev
  API:    http://localhost:8080/v1/chat/completions
  Health: http://localhost:8080/api/health

  Providers: 2 registered
  Models:    14 available
```

## Features

- **Single binary, zero dependencies** — Pure Go with embedded SQLite ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)). No CGO, no Docker required. `CGO_ENABLED=0` builds work out of the box.
- **Multi-provider inference** — Ollama (auto-detected), OpenAI, and any OpenAI-compatible API (Qwen, Llama, Minimax, OSS). Providers register at startup; model routing resolves via prefix (`openai/gpt-4o`), catalog lookup, or default fallback.
- **OpenAI-compatible API** — Drop-in `/v1/chat/completions` and `/v1/models` endpoints with SSE streaming support.
- **Session management** — Full CRUD for conversations with message history, model binding per session, and streaming message send with automatic content accumulation.
- **SQLite persistence** — WAL mode, dual connection pools (1 writer / 4 readers), embedded migrations, foreign keys enforced.
- **Auth middleware** — Optional Bearer token authentication with timing-safe comparison. Disabled when no key is set.
- **Health monitoring** — Public `/api/health` endpoint with DB connectivity checks, per-provider status, and uptime reporting.
- **Structured errors** — Consistent JSON error envelope matching the OpenAI error format across all endpoints.
- **Graceful shutdown** — SIGINT/SIGTERM handling with 5-second drain timeout.

## Quick Start

### Build

```bash
go build -o forge ./cmd/forge
```

Or with version info:

```bash
go build -ldflags "-X main.version=1.0.0" -o forge ./cmd/forge
```

### Run

```bash
# Minimal — auto-detects local Ollama, SQLite at ./forge.db
./forge

# With OpenAI
OPENAI_API_KEY=sk-... ./forge

# With auth enabled
FORGE_API_KEY=my-secret-key ./forge

# Custom port
FORGE_ADDR=:3000 ./forge
```

Forge auto-detects Ollama at `http://localhost:11434` on startup. If no providers are configured, it falls back to built-in mock providers for testing.

## API Reference

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check (DB, providers, uptime) |

### Protected Endpoints (require `Authorization: Bearer <key>` when `FORGE_API_KEY` is set)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI-compatible chat completion (streaming & non-streaming) |
| `GET` | `/v1/models` | List available models across all providers |
| `GET` | `/api/sessions` | List all sessions |
| `POST` | `/api/sessions` | Create a new session |
| `GET` | `/api/sessions/{id}` | Get session with message history |
| `PATCH` | `/api/sessions/{id}` | Update session (title, model, system prompt) |
| `DELETE` | `/api/sessions/{id}` | Delete session and all messages |
| `POST` | `/api/sessions/{id}/messages` | Send a message and stream the AI response |

### Example: Chat Completion

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5:0.5b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Example: Create a Session

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "title": "My Chat",
    "model": "qwen2.5:0.5b",
    "system_prompt": "You are a helpful assistant."
  }'
```

### Example: Send a Message (Streaming)

```bash
curl -N -X POST http://localhost:8080/api/sessions/{session_id}/messages \
  -H "Content-Type: application/json" \
  -d '{"content": "Explain quantum computing in simple terms"}'
```

### Model Routing

Models are resolved in order:

1. **Prefix** — `openai/gpt-4o` routes to the `openai` provider with model `gpt-4o`
2. **Catalog** — Known model names are mapped to their provider at startup via `/v1/models` (or `/api/tags` for Ollama)
3. **Default** — Falls back to the provider set via `FORGE_PROVIDER`

## Configuration

All configuration is via environment variables:

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_ADDR` | `:8080` | Listen address |
| `FORGE_DEV` | `false` | Development mode |
| `FORGE_API_KEY` | *(empty)* | API key for auth (empty = no auth) |
| `FORGE_CORS_ORIGINS` | `*` | Comma-separated allowed origins |

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_DB_PATH` | `forge.db` | SQLite database file path |
| `DATABASE_URL` | *(empty)* | PostgreSQL URL (if set, overrides SQLite) |

### Providers

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_PROVIDER` | `qwen` | Default provider name |
| `FORGE_MODEL` | `qwen2.5:0.5b` | Default model |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama server URL |
| `OPENAI_API_KEY` | *(empty)* | OpenAI API key |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | OpenAI base URL |
| `ANTHROPIC_API_KEY` | *(empty)* | Anthropic API key (reserved) |
| `QWEN_API_KEY` / `QWEN_BASE_URL` | *(empty)* | Qwen (OpenAI-compatible) |
| `LLAMA_API_KEY` / `LLAMA_BASE_URL` | *(empty)* | Llama (OpenAI-compatible) |
| `MINIMAX_API_KEY` / `MINIMAX_BASE_URL` | *(empty)* | Minimax (OpenAI-compatible) |
| `OSS_API_KEY` / `OSS_BASE_URL` | *(empty)* | OSS (OpenAI-compatible) |

### Limits

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_MAX_TOOL_TIMEOUT` | `60s` | Max tool execution timeout |
| `FORGE_MAX_TOOL_OUTPUT` | `65536` | Max tool output (bytes) |
| `FORGE_MAX_MESSAGE_SIZE` | `102400` | Max message body (bytes) |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_LOG_LEVEL` | `info` | Log level |
| `FORGE_LOG_FORMAT` | `json` | Log format (`json` or `pretty`) |

## Architecture

```
cmd/forge/main.go          → Entry point, DI wiring, startup banner
internal/
  config/                  → Environment-based configuration
  server/                  → HTTP server, chi router, middleware, CORS
  api/
    handlers_chat.go       → POST /v1/chat/completions, GET /v1/models
    handlers_sessions.go   → Session CRUD + message sending
    handlers_health.go     → GET /api/health
  inference/
    openai.go              → OpenAI-compatible provider
    ollama.go              → Ollama provider (NDJSON streaming)
    registry.go            → Thread-safe model→provider resolution
    retry.go               → Exponential backoff retry
    mock.go                → Mock provider for testing
  store/
    store.go               → Store interface + domain models
    sqlite.go              → SQLite implementation (WAL, dual pools)
    migrations/001_init.sql → Schema DDL
  session/
    manager.go             → Session lifecycle, ID generation
  auth/
    middleware.go           → Bearer token validation
  streaming/
    sse.go                 → SSE pipeline with backpressure
pkg/types/                 → API type definitions (errors, sessions, messages, etc.)
```

## Testing

Run all tests:

```bash
go test ./... -v
```

Tests cover 5 packages (70 tests total): `api`, `auth`, `inference`, `session`, `store`.

### Python Integration Tests

A Python script is included for end-to-end testing against mock providers:

```bash
pip install -r requirements.txt
python test_endpoints.py
```

## Downloading Models

Download local models via HuggingFace Hub or ModelScope:

```bash
python download_models.py --model qwen --source hf
```

## Tech Stack

- **Go 1.24** — Single binary, fast compile, great concurrency
- **[chi/v5](https://github.com/go-chi/chi)** — Lightweight HTTP router
- **[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** — Pure Go SQLite (no CGO)
- **[caarlos0/env](https://github.com/caarlos0/env)** — Struct-tag environment config
- **[google/uuid](https://github.com/google/uuid)** — UUID generation for session/message IDs

## License

See [LICENSE](LICENSE) for details.
