# 🏗️ Forge: Module Architect Review

> **Reviewer:** Lead Module Architect  
> **Document Under Review:** `IMPLEMENTATION_PLAN.md`  
> **Date:** 2025-01-22  
> **Verdict:** The plan is a strong starting point but lacks the structural rigor needed for contract-first development. This review fills the gaps.

---

## Table of Contents

1. [Module Decomposition Review](#1-module-decomposition-review)
2. [JSON/Protocol Contracts](#2-jsonprotocol-contracts)
3. [Plugin/Extension Architecture](#3-pluginextension-architecture)
4. [State Management Architecture](#4-state-management-architecture)
5. [Orchestrator Pattern (Inference State Machine)](#5-orchestrator-pattern)
6. [Scalability Considerations](#6-scalability-considerations)
7. [Contract-First Development (Full Interface Catalog)](#7-contract-first-development)

---

## 1. Module Decomposition Review

### 1.1 Assessment of the Current 6 Modules

The plan lists six modules. Here is a critique of each:

| Module | Verdict | Issue |
|:---|:---|:---|
| API Gateway | ✅ Correct | Well-scoped. Owns HTTP routing, SSE streaming, auth middleware. |
| Inference Layer | ⚠️ Under-specified | Needs to be split: **Provider Registry** (model catalog + selection) and **Inference Orchestrator** (the loop logic). These are two very different responsibilities. |
| Context Manager | ⚠️ Overloaded | Currently owns session persistence, history tracking, AND token counting. Token counting is a utility, not a manager concern. Sessions need their own lifecycle. |
| Compaction Engine | ✅ Correct | Single-responsibility. Consumes context, produces summary. Good boundary. |
| Tool Sandbox | ✅ Correct | Clean boundary. Receives tool call, returns result. |
| Web Server | ⚠️ Too Thin | This is not a "module" — it's a static file server. It should be a sub-component of the API Gateway, not a peer module. |

### 1.2 Missing Modules

The plan is missing **five** critical modules:

| Missing Module | Why It's Needed |
|:---|:---|
| **Config** | Centralized configuration loading (env vars, config files, CLI flags). Every module depends on config. Without it, each module reads `os.Getenv` independently — untestable and fragile. |
| **Session Manager** | The Context Manager conflates "session lifecycle" with "context window management." Sessions (create, list, delete, expire) are a CRUD concern. Context window management (what messages are in the active window) is a computation concern. These must be separate. |
| **Event Bus** | The plan mentions a WebSocket event broadcast in Phase 3 but doesn't elevate it to a module. The Event Bus is the backbone — it connects the Inference Orchestrator, Tool Sandbox, and all UI clients. It deserves first-class module status. |
| **Model Registry** | The plan says "Pluggable backend" but provides no design for how models are discovered, listed, or selected. A `/v1/models` endpoint requires a registry that knows which providers are configured and what models they offer. |
| **Auth Middleware** | Phase 5 mentions API key auth but treats it as an afterthought. Auth is cross-cutting — it must be designed as a module from Day 1, even if the initial implementation is a no-op passthrough. |

### 1.3 Revised Module Map (10 Modules)

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

### 1.4 Module Dependency Graph

```
Config ──────────────────────────────────────────────────┐
  │                                                       │
  ├──► Auth Middleware                                    │
  ├──► API Gateway ──► Auth Middleware                    │
  │        │                                              │
  │        ├──► Session Manager ──► Storage Layer          │
  │        ├──► Model Registry ──► Config                 │
  │        └──► Inference Orchestrator                    │
  │                 │                                     │
  │                 ├──► Context Window ──► Storage Layer  │
  │                 ├──► Model Registry                   │
  │                 ├──► Tool Sandbox                     │
  │                 ├──► Compaction Engine ──► Model Reg.  │
  │                 └──► Event Bus                        │
  │                                                       │
  ├──► Event Bus                                          │
  ├──► Storage Layer                                      │
  └──► Web Server (embedded frontend)                     │
                                                          │
  (all modules receive Config at init)────────────────────┘
```

**Dependency Rules (MUST enforce):**
1. **No circular dependencies.** The graph above is a DAG.
2. **Config is the root.** Every module receives its config struct at construction time — never reads env vars directly.
3. **Storage Layer is a leaf.** It depends on nothing except Config. All persistence goes through it.
4. **Event Bus has no domain knowledge.** It routes typed events but never inspects payloads.
5. **Inference Orchestrator is the "fat node."** It has the most dependencies — this is acceptable because it IS the core business logic.

---

## 2. JSON/Protocol Contracts

The plan provides only 2 example contracts. A production system needs **at minimum** the following 12 contracts. I define them all below.

### 2.1 Contract Catalog

| # | Contract | Direction | Transport |
|:--|:---|:---|:---|
| 1 | Chat Completion Request | Client → API Gateway | HTTP POST |
| 2 | Chat Completion Response (Streaming) | API Gateway → Client | SSE |
| 3 | Chat Completion Response (Non-Streaming) | API Gateway → Client | HTTP JSON |
| 4 | Session Create | Client → API Gateway | HTTP POST |
| 5 | Session List | Client → API Gateway | HTTP GET |
| 6 | Session Get (with messages) | Client → API Gateway | HTTP GET |
| 7 | Session Delete | Client → API Gateway | HTTP DELETE |
| 8 | Model List | Client → API Gateway | HTTP GET |
| 9 | Tool Execution (Internal) | Orchestrator → Tool Sandbox | In-process |
| 10 | Compaction Request (Internal) | Orchestrator → Compaction Engine | In-process |
| 11 | WebSocket Event Envelope | Event Bus → UI Client | WebSocket |
| 12 | Configuration Schema | File/Env → Config Module | YAML/Env |

### 2.2 Full Contract Definitions

#### Contract 1: Chat Completion Request (OpenAI-compatible)

```json
{
  "model": "llama3.2:3b",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is Forge?"}
  ],
  "stream": true,
  "temperature": 0.7,
  "max_tokens": 2048,
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "terminal_exec",
        "description": "Execute a shell command",
        "parameters": {
          "type": "object",
          "properties": {
            "command": {"type": "string"}
          },
          "required": ["command"]
        }
      }
    }
  ],
  "session_id": "forge_session_abc123"
}
```

> **Note:** `session_id` is a Forge extension to the OpenAI spec. If omitted, the request is stateless (no history persistence). If provided, messages are appended to the session's history.

#### Contract 2: SSE Stream Events (Chat Completion — Streaming)

```
data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","created":1700000000,"model":"llama3.2:3b","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","created":1700000000,"model":"llama3.2:3b","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","created":1700000000,"model":"llama3.2:3b","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}

data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","created":1700000000,"model":"llama3.2:3b","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

> **CRITICAL:** This MUST be OpenAI-compatible. Forge's value prop is that any OpenAI SDK client can point at Forge and "just work."

#### Contract 3: Chat Completion Response (Non-Streaming)

```json
{
  "id": "chatcmpl-xyz",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "llama3.2:3b",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 9,
    "total_tokens": 34
  }
}
```

#### Contract 4: Session Create

**Request:**
```http
POST /v1/sessions
Content-Type: application/json

{
  "title": "Debug Session",
  "model": "llama3.2:3b",
  "system_prompt": "You are a Go debugging expert.",
  "tools": ["terminal_exec", "file_read"]
}
```

**Response:**
```json
{
  "id": "sess_abc123",
  "title": "Debug Session",
  "model": "llama3.2:3b",
  "system_prompt": "You are a Go debugging expert.",
  "tools": ["terminal_exec", "file_read"],
  "created_at": "2025-01-22T10:00:00Z",
  "updated_at": "2025-01-22T10:00:00Z",
  "token_count": 45,
  "message_count": 0,
  "status": "active"
}
```

#### Contract 5: Session List

**Request:**
```http
GET /v1/sessions?status=active&limit=20&offset=0
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "sess_abc123",
      "title": "Debug Session",
      "model": "llama3.2:3b",
      "status": "active",
      "message_count": 42,
      "token_count": 3200,
      "created_at": "2025-01-22T10:00:00Z",
      "updated_at": "2025-01-22T11:30:00Z"
    }
  ],
  "total": 1,
  "has_more": false
}
```

#### Contract 6: Session Get (with messages)

**Request:**
```http
GET /v1/sessions/sess_abc123?include_messages=true
```

**Response:**
```json
{
  "id": "sess_abc123",
  "title": "Debug Session",
  "model": "llama3.2:3b",
  "system_prompt": "You are a Go debugging expert.",
  "status": "active",
  "messages": [
    {
      "id": "msg_001",
      "role": "user",
      "content": "Why is my goroutine leaking?",
      "created_at": "2025-01-22T10:01:00Z",
      "token_count": 12
    },
    {
      "id": "msg_002",
      "role": "assistant",
      "content": "Let me check your code...",
      "created_at": "2025-01-22T10:01:02Z",
      "token_count": 8,
      "tool_calls": [
        {
          "id": "call_001",
          "function": {"name": "file_read", "arguments": "{\"path\": \"main.go\"}"},
          "status": "completed",
          "result": "package main..."
        }
      ]
    }
  ],
  "token_count": 3200,
  "message_count": 42,
  "created_at": "2025-01-22T10:00:00Z",
  "updated_at": "2025-01-22T11:30:00Z"
}
```

#### Contract 7: Session Delete

**Request:**
```http
DELETE /v1/sessions/sess_abc123
```

**Response:**
```json
{
  "id": "sess_abc123",
  "object": "session",
  "deleted": true
}
```

#### Contract 8: Model List

**Request:**
```http
GET /v1/models
```

**Response (OpenAI-compatible):**
```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3.2:3b",
      "object": "model",
      "created": 1700000000,
      "owned_by": "ollama/local",
      "provider": "ollama",
      "context_window": 131072,
      "capabilities": ["chat", "tools"],
      "status": "available"
    },
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 1700000000,
      "owned_by": "openai",
      "provider": "openai",
      "context_window": 128000,
      "capabilities": ["chat", "tools", "vision"],
      "status": "available"
    }
  ]
}
```

> **Note:** `provider`, `context_window`, `capabilities`, `status` are Forge extensions to the OpenAI model object.

#### Contract 9: Tool Execution (Internal)

```json
// Request (Orchestrator → Tool Sandbox)
{
  "request_id": "forge_req_abc123",
  "call_id": "call_001",
  "session_id": "sess_abc123",
  "tool_name": "terminal_exec",
  "arguments": {"command": "grep -r 'todo' ."},
  "environment": "local",
  "timeout_ms": 10000,
  "working_dir": "/home/user/project"
}

// Response (Tool Sandbox → Orchestrator)
{
  "request_id": "forge_req_abc123",
  "call_id": "call_001",
  "tool_name": "terminal_exec",
  "status": "success",
  "output": "main.go:42: // todo: fix this\nutils.go:10: // todo: add tests",
  "stderr": "",
  "exit_code": 0,
  "duration_ms": 250,
  "truncated": false
}
```

**Error Response:**
```json
{
  "request_id": "forge_req_abc123",
  "call_id": "call_001",
  "tool_name": "terminal_exec",
  "status": "error",
  "error": {
    "code": "TOOL_TIMEOUT",
    "message": "Tool execution exceeded 10000ms timeout",
    "retryable": false
  },
  "output": "",
  "duration_ms": 10000,
  "truncated": false
}
```

#### Contract 10: Compaction Request (Internal)

```json
// Request (Orchestrator → Compaction Engine)
{
  "session_id": "sess_abc123",
  "model": "llama3.2:3b",
  "system_prompt": "You are a Go debugging expert.",
  "messages": [
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": "..."}
  ],
  "current_token_count": 120000,
  "max_token_count": 131072,
  "preserve_last_n": 5,
  "strategy": "summarize"
}

// Response (Compaction Engine → Orchestrator)
{
  "session_id": "sess_abc123",
  "status": "compacted",
  "summary_message": {
    "role": "system",
    "content": "[Conversation Summary]\nThe user has been debugging a goroutine leak in main.go. Key findings: ...",
    "metadata": {
      "compacted_message_count": 37,
      "original_token_count": 120000,
      "new_token_count": 4500
    }
  },
  "preserved_messages": [
    {"role": "user", "content": "Can you try the fix now?"},
    {"role": "assistant", "content": "..."}
  ]
}
```

#### Contract 11: WebSocket Event Envelope

All WebSocket events share a common envelope:

```json
{
  "type": "<event_type>",
  "session_id": "sess_abc123",
  "request_id": "forge_req_abc123",
  "timestamp": "2025-01-22T10:01:02.345Z",
  "payload": { }
}
```

**Full Event Type Catalog:**

| Event Type | Direction | Payload Description |
|:---|:---|:---|
| `inference.started` | Server → Client | `{"model": "...", "token_count": 3200}` |
| `inference.token` | Server → Client | `{"delta": "Hello"}` |
| `inference.tool_call` | Server → Client | `{"call_id": "...", "tool": "...", "arguments": {...}}` |
| `inference.tool_result` | Server → Client | `{"call_id": "...", "status": "...", "output": "..."}` |
| `inference.completed` | Server → Client | `{"finish_reason": "stop", "usage": {...}}` |
| `inference.error` | Server → Client | `{"code": "...", "message": "..."}` |
| `compaction.started` | Server → Client | `{"original_tokens": 120000}` |
| `compaction.completed` | Server → Client | `{"new_tokens": 4500, "removed_messages": 37}` |
| `session.created` | Server → Client | `{"session": {...}}` |
| `session.updated` | Server → Client | `{"session": {...}}` |
| `session.deleted` | Server → Client | `{"session_id": "..."}` |
| `model.status_changed` | Server → Client | `{"model_id": "...", "status": "available\|unavailable"}` |
| `system.error` | Server → Client | `{"code": "...", "message": "..."}` |
| `ping` | Server → Client | `{}` (keepalive) |
| `pong` | Client → Server | `{}` (keepalive response) |

#### Contract 12: Configuration Schema

```yaml
# forge.yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 120s

auth:
  enabled: false
  api_key: ""            # overridden by FORGE_API_KEY env var

storage:
  driver: "sqlite"       # "sqlite" | "postgres"
  sqlite:
    path: "./forge.db"
  postgres:
    url: ""              # overridden by DATABASE_URL env var

providers:
  ollama:
    enabled: true
    base_url: "http://localhost:11434"
  openai:
    enabled: false
    api_key: ""          # overridden by OPENAI_API_KEY env var
    base_url: "https://api.openai.com"
  anthropic:
    enabled: false
    api_key: ""          # overridden by ANTHROPIC_API_KEY env var
    base_url: "https://api.anthropic.com"

inference:
  default_model: "llama3.2:3b"
  max_concurrent_requests: 4
  request_timeout: 120s

compaction:
  enabled: true
  threshold: 0.9         # compact when token_count > max * threshold
  preserve_last_n: 5
  strategy: "summarize"  # "summarize" | "truncate"

tools:
  enabled: true
  sandbox: "local"       # "local" | "docker"
  timeout: 10s
  manifest_dir: "./tools"
  docker:
    image: "forge-sandbox:latest"
    network: "none"
    memory_limit: "256m"

logging:
  level: "info"          # "debug" | "info" | "warn" | "error"
  format: "text"         # "text" | "json"
```

---

## 3. Plugin/Extension Architecture

### 3.1 Recommendation: YES, but with strict boundaries

Forge should support plugins, but **only through well-defined extension points** — not a generic plugin API. Here's why:

- Forge's value is being a single binary. A full plugin system (like VS Code extensions) undermines this.
- However, two use cases are unavoidable: **custom tools** and **custom LLM providers**.

### 3.2 Extension Points (3 Tiers)

```
┌─────────────────────────────────────────────────┐
│                FORGE CORE BINARY                 │
│                                                  │
│  Tier 1: Tool Plugins (Day 1)                   │
│  ┌─────────────────────────────────────────┐    │
│  │ Tool Manifest (.json/.yaml files)        │    │
│  │ → External scripts/binaries              │    │
│  │ → Discovered at startup from tools_dir   │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│  Tier 2: Provider Plugins (Day 1)               │
│  ┌─────────────────────────────────────────┐    │
│  │ Go interfaces (compile-time)             │    │
│  │ → Add new provider = implement interface │    │
│  │ → Register in provider registry          │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│  Tier 3: UI Plugins (Future / Out of Scope)     │
│  ┌─────────────────────────────────────────┐    │
│  │ NOT recommended for v1                   │    │
│  │ → Embed a single opinionated UI          │    │
│  │ → Customization via CSS variables only   │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
└─────────────────────────────────────────────────┘
```

### 3.3 Tool Plugin Manifest Format

Tools are defined as JSON files in a `tools/` directory. Forge discovers them at startup.

```json
{
  "name": "google_search",
  "description": "Search Google and return top results",
  "version": "1.0.0",
  "type": "executable",
  "executable": {
    "command": "./tools/bin/google_search",
    "args_format": "json_stdin",
    "timeout_ms": 15000
  },
  "parameters": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "The search query"
      },
      "num_results": {
        "type": "integer",
        "description": "Number of results to return",
        "default": 5
      }
    },
    "required": ["query"]
  }
}
```

**Execution protocol:**
1. Forge invokes the executable.
2. Arguments are passed as JSON on **stdin**.
3. Tool writes its result as JSON on **stdout**.
4. Exit code 0 = success, non-zero = error.
5. Stderr is captured as diagnostic info.

This means tool plugins can be written in **any language** — Python, Bash, Rust, Node — as long as they read JSON from stdin and write JSON to stdout.

### 3.4 Provider Plugin Pattern (Compile-time)

New providers are added by implementing the `InferenceProvider` interface (defined in Section 7) and registering them in the provider registry. This is a **compile-time** extension — you rebuild the binary. This is intentional: LLM providers require nuanced stream parsing, auth handling, and error mapping that cannot be safely delegated to an external process.

---

## 4. State Management Architecture

### 4.1 Session Lifecycle State Machine

```
                    ┌──────────┐
          POST      │          │
       /v1/sessions │ CREATING │
       ─────────────►          │
                    └────┬─────┘
                         │ persist to DB
                         ▼
                    ┌──────────┐
                    │          │    user sends message
                    │  ACTIVE  │◄───────────────────┐
                    │          │                     │
                    └────┬─────┘                     │
                         │                           │
              ┌──────────┼──────────┐                │
              │          │          │                 │
              ▼          ▼          ▼                 │
         ┌────────┐ ┌────────┐ ┌────────┐           │
         │INFER-  │ │COMPACT-│ │ IDLE   │───────────┘
         │RING    │ │ING     │ │(no req │  user returns
         │(LLM    │ │(shrink │ │ for    │
         │working)│ │context)│ │ >idle_ │
         └───┬────┘ └───┬────┘ │ timeout│
             │          │      └───┬────┘
             └──────────┘          │ exceeds expire_timeout
                  │                ▼
                  │           ┌──────────┐
                  └──────────►│ EXPIRED  │
                              │ (frozen, │
                              │ read-only│
                              └────┬─────┘
                                   │ user deletes or
                                   │ auto-archive policy
                                   ▼
                              ┌──────────┐
                              │ ARCHIVED │
                              │(messages │
                              │ in cold  │
                              │ storage) │
                              └────┬─────┘
                                   │ DELETE
                                   ▼
                              ┌──────────┐
                              │ DELETED  │
                              │(hard or  │
                              │ soft del)│
                              └──────────┘
```

**States:**

| State | Description | Transitions |
|:---|:---|:---|
| `creating` | Session being initialized, system prompt tokenized | → `active` |
| `active` | Ready to accept inference requests | → `inferring`, `idle` |
| `inferring` | LLM is actively generating a response | → `active`, `compacting`, `active` (error recovery) |
| `compacting` | Context window being compressed | → `active` (resume), `active` (error → skip compaction) |
| `idle` | No requests for `idle_timeout` duration | → `active` (user returns), `expired` |
| `expired` | No requests for `expire_timeout` — read-only | → `active` (explicit reactivation), `archived` |
| `archived` | Messages moved to cold storage / compressed | → `deleted` |
| `deleted` | Terminal state | — |

### 4.2 State Flow Between Modules

```
┌──────────┐     ┌─────────────┐     ┌──────────────┐
│  API     │────►│  Session     │────►│   Storage    │
│ Gateway  │     │  Manager     │     │   Layer      │
│          │     │              │     │              │
│ creates/ │     │ enforces     │     │ persists     │
│ routes   │     │ lifecycle    │     │ state        │
│ requests │     │ transitions  │     │              │
└──────────┘     └──────┬───────┘     └──────────────┘
                        │
                        │ passes session context to
                        ▼
               ┌──────────────────┐
               │   Inference      │
               │   Orchestrator   │
               │                  │
               │ reads/writes     │
               │ context window   │
               │ via Context      │
               │ Window module    │
               └────────┬─────────┘
                        │
                ┌───────┼───────┐
                ▼       ▼       ▼
           ┌────────┐ ┌─────┐ ┌──────────┐
           │Context │ │Tool │ │Compaction│
           │Window  │ │Sand │ │Engine    │
           │        │ │box  │ │          │
           └────────┘ └─────┘ └──────────┘
```

### 4.3 Event Sourcing vs. CRUD: Recommendation

**Use CRUD with an append-only message log.** Here's why:

| Approach | Pros | Cons | Verdict |
|:---|:---|:---|:---|
| Full Event Sourcing | Complete audit trail, time travel | Massive complexity for a single-binary tool, replay overhead | ❌ Overkill |
| Pure CRUD | Simple, fast | Lose history on compaction, no audit trail | ❌ Too lossy |
| **CRUD + Append-Only Messages** | Simple writes, never lose data, compaction creates new summary row but original messages are soft-deleted (flagged, not erased) | Slightly more storage | ✅ **This one** |

**Schema implication:**
```sql
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,  -- 'system' | 'user' | 'assistant' | 'tool'
    content     TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,  -- FALSE after compaction
    created_at  TIMESTAMP NOT NULL,
    metadata    TEXT  -- JSON blob for tool_calls, compaction info, etc.
);
```

When compaction happens, old messages get `is_active = FALSE` and a new summary message is inserted with `is_active = TRUE`. The Inspector UI can show both active and archived messages.

### 4.4 Cache Layer Design

```
┌─────────────────────────────────────────────┐
│              In-Memory Cache                 │
│                                              │
│  ┌────────────────────────┐                  │
│  │ Active Session Cache   │                  │
│  │ map[session_id] →      │                  │
│  │   { messages []Msg,    │                  │
│  │     token_count int,   │                  │
│  │     model string,      │                  │
│  │     last_access Time } │                  │
│  └────────────────────────┘                  │
│                                              │
│  ┌────────────────────────┐                  │
│  │ Model Registry Cache   │                  │
│  │ map[model_id] →        │                  │
│  │   { provider,          │                  │
│  │     context_window,    │                  │
│  │     capabilities }     │                  │
│  └────────────────────────┘                  │
│                                              │
│  ┌────────────────────────┐                  │
│  │ Token Count Cache      │                  │
│  │ (avoid re-counting     │                  │
│  │  unchanged messages)   │                  │
│  └────────────────────────┘                  │
│                                              │
│  Eviction: LRU, max_sessions configurable    │
│  Write-through: all mutations hit DB first   │
└─────────────────────────────────────────────┘
```

**Cache policy:** Write-through (DB is always authoritative). Cache is populated on first access and evicted LRU when memory pressure is detected or session count exceeds `max_cached_sessions` (default: 100).

---

## 5. Orchestrator Pattern (Inference State Machine)

This is the most critical section. The inference loop in the plan is described informally. Here it is formalized as a state machine.

### 5.1 Inference State Machine

```
                         ┌───────────────────┐
    Request arrives      │                   │
    ────────────────────►│   VALIDATING      │
                         │                   │
                         │ • Validate input  │
                         │ • Resolve model   │
                         │ • Load session    │
                         └────────┬──────────┘
                                  │
                         ┌────────▼──────────┐
                         │                   │
                         │  BUILDING_CONTEXT  │
                         │                   │
                         │ • Append user msg │
                         │ • Count tokens    │
                         │ • Check threshold │
                         └────────┬──────────┘
                                  │
                          ┌───────┴───────┐
                          │               │
                 tokens < threshold    tokens >= threshold
                          │               │
                          │      ┌────────▼──────────┐
                          │      │                   │
                          │      │    COMPACTING      │
                          │      │                   │
                          │      │ • Summarize old   │
                          │      │ • Replace context │
                          │      │ • Recount tokens  │
                          │      └────────┬──────────┘
                          │               │
                          └───────┬───────┘
                                  │
                         ┌────────▼──────────┐
                         │                   │
                         │    INFERRING      │◄─────────────────┐
                         │                   │                   │
                         │ • Send to LLM    │                   │
                         │ • Stream tokens  │                   │
                         │ • Emit events    │                   │
                         └────────┬──────────┘                   │
                                  │                              │
                          ┌───────┴───────┐                     │
                          │               │                     │
                   finish_reason     finish_reason              │
                     = "stop"       = "tool_calls"              │
                          │               │                     │
                          │      ┌────────▼──────────┐          │
                          │      │                   │          │
                          │      │ EXECUTING_TOOLS   │          │
                          │      │                   │          │
                          │      │ • Parse tool call │          │
                          │      │ • Execute in      │          │
                          │      │   sandbox         │          │
                          │      │ • Append result   │          │
                          │      │   to context      │          │
                          │      └────────┬──────────┘          │
                          │               │                     │
                          │               │ tool result ready   │
                          │               └─────────────────────┘
                          │                  (loop back to INFERRING)
                          │
                 ┌────────▼──────────┐
                 │                   │
                 │   COMPLETING      │
                 │                   │
                 │ • Persist message │
                 │ • Update session  │
                 │ • Emit final      │
                 │   events          │
                 │ • Return response │
                 └───────────────────┘
```

### 5.2 Formal State Definitions

| State | Entry Condition | Exit Condition | Max Duration | On Timeout |
|:---|:---|:---|:---|:---|
| `VALIDATING` | Request received | Input valid, model resolved | 1s | → ERROR (400 Bad Request) |
| `BUILDING_CONTEXT` | Validation passed | Context assembled, token count known | 5s | → ERROR (500 Internal) |
| `COMPACTING` | Token count ≥ threshold | New context ready, tokens under limit | 60s | → ERROR with partial context (skip compaction, truncate instead) |
| `INFERRING` | Context ready | LLM returns finish_reason | `request_timeout` (120s default) | → ERROR (504 Gateway Timeout), cancel LLM request |
| `EXECUTING_TOOLS` | LLM returned tool_calls | All tools complete | `tool_timeout` per tool (10s default) | → ERROR result injected as tool output, resume to INFERRING |
| `COMPLETING` | finish_reason = "stop" | Response sent, DB updated | 5s | → LOG warning, response already sent |

### 5.3 Error States and Recovery

```
Any State ──── unrecoverable error ────► ERROR
                                           │
                                           ├── input_invalid (400)
                                           ├── model_unavailable (503)
                                           ├── provider_error (502)
                                           ├── context_overflow (413)
                                           ├── tool_execution_failed (*)
                                           ├── compaction_failed (*)
                                           ├── inference_timeout (504)
                                           └── internal_error (500)

(*) = recoverable: inject error message into context, continue
```

**Recovery strategies by error type:**

| Error | Severity | Recovery |
|:---|:---|:---|
| `input_invalid` | Fatal | Return 400 immediately. No state change. |
| `model_unavailable` | Fatal | Return 503. Suggest available models in error body. |
| `provider_error` | Fatal | Return 502. Log full provider error. Include provider name in response. |
| `context_overflow` | Recoverable | Force-trigger compaction. If compaction also fails, truncate oldest messages. |
| `tool_execution_failed` | Recoverable | Inject `{"error": "Tool X failed: reason"}` as tool result. Let LLM handle gracefully. |
| `compaction_failed` | Recoverable | Log error. Fall back to `truncate` strategy (drop oldest messages beyond system prompt). |
| `inference_timeout` | Fatal | Cancel LLM context. Return 504. Persist partial response if any tokens were generated. |
| `internal_error` | Fatal | Return 500. Log full stack trace. |

### 5.4 Tool Execution Loop Limits

**Anti-infinite-loop protection:** The Orchestrator MUST enforce a maximum tool-call loop depth.

| Parameter | Default | Description |
|:---|:---|:---|
| `max_tool_rounds` | 10 | Maximum number of INFERRING → EXECUTING_TOOLS → INFERRING cycles per request |
| `max_tool_calls_per_round` | 5 | Maximum parallel tool calls the LLM can make in a single response |
| `max_total_tool_calls` | 25 | Absolute maximum tool calls across all rounds |

If any limit is exceeded, the Orchestrator injects a system message: `"Tool call limit reached. Please provide your final answer based on the information gathered so far."` and runs one final inference with `tools: []` (no tools available).

---

## 6. Scalability Considerations

### 6.1 Single-User (v1) vs. Multi-User Architecture

The plan is designed for single-user. Here's what changes for multi-user:

```
┌─────────────────────────────────────────────────────┐
│                   SINGLE-USER (v1)                   │
│                                                      │
│  • No user concept — all sessions belong to "self"  │
│  • Auth = optional API key (bearer token)           │
│  • SQLite = fine                                    │
│  • Single inference at a time = fine                │
│  • In-memory session cache = fine                   │
│                                                      │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│                   MULTI-USER (v2)                    │
│                                                      │
│  • User model with tenant isolation                 │
│  • Auth = JWT / OAuth2 / API keys per user          │
│  • PostgreSQL required (concurrent writes)          │
│  • Inference queue (bounded concurrency per user)   │
│  • Session cache keyed by (user_id, session_id)     │
│  • Rate limiting per user                           │
│                                                      │
└─────────────────────────────────────────────────────┘
```

### 6.2 Design Decisions for v1 that DON'T Block v2

| Decision | v1 Implementation | v2 Migration Path |
|:---|:---|:---|
| User identity | Hardcoded `user_id = "default"` on all records | Add `user_id` column, populate from auth middleware |
| Auth | Single API key or none | Swap `AuthMiddleware` implementation to JWT |
| Storage | SQLite | Swap `StorageDriver` implementation to PostgreSQL (already planned) |
| Concurrency | `sync.Mutex` on inference (one at a time) | Replace with semaphore pool keyed by user_id |
| Session cache | `map[string]*Session` | `map[UserSessionKey]*Session` |

**CRITICAL v1 RULE:** Every database record MUST have a `user_id` field from Day 1, even if it's always `"default"`. This avoids a painful migration later.

### 6.3 Horizontal Scaling Strategy (v2+)

```
                    ┌─────────────┐
                    │   Load      │
                    │  Balancer   │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
         ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
         │ Forge   │  │ Forge   │  │ Forge   │
         │ Node 1  │  │ Node 2  │  │ Node 3  │
         └────┬────┘  └────┬────┘  └────┬────┘
              │            │            │
              └────────────┼────────────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
         ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
         │ Postgres │  │  Redis  │  │  NATS   │
         │ (state)  │  │ (cache) │  │ (events)│
         └─────────┘  └─────────┘  └─────────┘
```

**What changes:**
- SQLite → PostgreSQL (shared state)
- In-memory cache → Redis (shared cache)
- In-process Event Bus → NATS/Redis Pub-Sub (cross-node events)
- WebSocket connections are node-local; events are fanned out via NATS

**What stays the same:**
- All Go interfaces remain identical
- All JSON contracts remain identical
- The inference state machine runs identically on any node

### 6.4 Queue-Based Inference Scheduling

For v2 multi-user, inference requests should be queued:

```
┌──────────┐     ┌──────────────────┐     ┌──────────────────┐
│  API     │────►│ Inference Queue   │────►│ Inference Worker  │
│ Gateway  │     │                   │     │  Pool             │
│          │     │ • Priority queue  │     │                   │
│          │     │ • Per-user fair   │     │ • max_concurrent  │
│          │◄────│   scheduling     │◄────│   = N             │
│ (waits   │     │ • Timeout if     │     │ • Each worker     │
│  for     │     │   queue full     │     │   runs the state  │
│  result) │     │                  │     │   machine         │
└──────────┘     └──────────────────┘     └──────────────────┘
```

**v1 implementation:** The "queue" is a `sync.Mutex` — one inference at a time. The interface is designed so that swapping to a real queue is a drop-in replacement.

---

## 7. Contract-First Development (Full Interface Catalog)

### 7.1 Go Interface Definitions

Below are the Go interfaces that each module MUST implement. These are the **contracts** — the actual struct implementations are left to the developer.

#### 7.1.1 Config

```go
// config.go — Configuration loading and access

type Config struct {
    Server     ServerConfig
    Auth       AuthConfig
    Storage    StorageConfig
    Providers  ProvidersConfig
    Inference  InferenceConfig
    Compaction CompactionConfig
    Tools      ToolsConfig
    Logging    LoggingConfig
}

// LoadConfig reads from file path, env vars, and CLI flags.
// Priority: CLI flags > env vars > config file > defaults.
func LoadConfig(configPath string) (*Config, error)
```

#### 7.1.2 Storage Layer

```go
// storage.go — Persistence abstraction

type Store interface {
    // Sessions
    CreateSession(ctx context.Context, s *Session) error
    GetSession(ctx context.Context, id string) (*Session, error)
    ListSessions(ctx context.Context, filter SessionFilter) (*SessionList, error)
    UpdateSession(ctx context.Context, s *Session) error
    DeleteSession(ctx context.Context, id string) error

    // Messages
    AppendMessage(ctx context.Context, sessionID string, m *Message) error
    GetMessages(ctx context.Context, sessionID string, filter MessageFilter) ([]*Message, error)
    DeactivateMessages(ctx context.Context, sessionID string, messageIDs []string) error

    // Lifecycle
    Close() error
    Migrate(ctx context.Context) error
}

type SessionFilter struct {
    UserID string
    Status string   // "active", "idle", "expired", "archived"
    Limit  int
    Offset int
}

type MessageFilter struct {
    ActiveOnly bool
    Limit      int
    AfterID    string
}
```

#### 7.1.3 Inference Provider

```go
// provider.go — LLM provider abstraction

type InferenceProvider interface {
    // ID returns the provider identifier (e.g., "ollama", "openai").
    ID() string

    // ListModels returns all models available from this provider.
    ListModels(ctx context.Context) ([]Model, error)

    // ChatCompletion sends a chat request and returns a streaming response.
    // The caller MUST consume the stream and call stream.Close() when done.
    ChatCompletion(ctx context.Context, req *ChatRequest) (ChatStream, error)

    // SupportsModel returns true if this provider serves the given model ID.
    SupportsModel(modelID string) bool

    // Healthy returns nil if the provider is reachable.
    Healthy(ctx context.Context) error
}

type ChatStream interface {
    // Next returns the next chunk. Returns io.EOF when stream is complete.
    Next() (*ChatChunk, error)

    // Close cancels the stream and releases resources.
    Close() error
}

type ChatRequest struct {
    Model       string
    Messages    []Message
    Tools       []ToolDefinition
    Temperature float64
    MaxTokens   int
    Stream      bool
}

type ChatChunk struct {
    ID           string
    Delta        ContentDelta
    FinishReason string    // "", "stop", "tool_calls"
    ToolCalls    []ToolCall // populated when FinishReason == "tool_calls"
    Usage        *Usage    // populated on final chunk
}

type ContentDelta struct {
    Role    string // only set on first chunk
    Content string
}
```

#### 7.1.4 Model Registry

```go
// registry.go — Model discovery and routing

type ModelRegistry interface {
    // ListModels returns all models across all configured providers.
    ListModels(ctx context.Context) ([]Model, error)

    // GetProvider returns the provider that serves the given model ID.
    // Returns an error if the model is not found or the provider is unhealthy.
    GetProvider(ctx context.Context, modelID string) (InferenceProvider, error)

    // GetModelInfo returns metadata for a specific model.
    GetModelInfo(ctx context.Context, modelID string) (*Model, error)

    // RefreshModels re-queries all providers for their model lists.
    RefreshModels(ctx context.Context) error
}

type Model struct {
    ID            string   // "llama3.2:3b", "gpt-4o"
    Provider      string   // "ollama", "openai"
    OwnedBy       string   // "meta", "openai"
    ContextWindow int      // max tokens
    Capabilities  []string // ["chat", "tools", "vision"]
    Status        string   // "available", "unavailable", "loading"
    CreatedAt     int64    // unix timestamp
}
```

#### 7.1.5 Session Manager

```go
// session.go — Session lifecycle management

type SessionManager interface {
    // Create initializes a new session with system prompt and config.
    Create(ctx context.Context, req *CreateSessionRequest) (*Session, error)

    // Get retrieves a session by ID (with or without messages).
    Get(ctx context.Context, id string, includeMessages bool) (*Session, error)

    // List returns sessions matching the filter.
    List(ctx context.Context, filter SessionFilter) (*SessionList, error)

    // Delete removes a session and its messages.
    Delete(ctx context.Context, id string) error

    // Touch updates the session's last_access timestamp (resets idle timer).
    Touch(ctx context.Context, id string) error

    // GetContextWindow returns the active messages for a session,
    // suitable for sending to the LLM.
    GetContextWindow(ctx context.Context, id string) (*ContextWindow, error)

    // AppendMessage adds a message to the session and updates token counts.
    AppendMessage(ctx context.Context, sessionID string, msg *Message) error
}

type CreateSessionRequest struct {
    Title        string
    Model        string
    SystemPrompt string
    Tools        []string
    UserID       string
}

type Session struct {
    ID           string
    UserID       string
    Title        string
    Model        string
    SystemPrompt string
    Tools        []string
    Status       string    // "active", "idle", "expired", "archived"
    TokenCount   int
    MessageCount int
    CreatedAt    time.Time
    UpdatedAt    time.Time
    LastAccessAt time.Time
}

type ContextWindow struct {
    SessionID    string
    SystemPrompt string
    Messages     []Message
    TokenCount   int
    MaxTokens    int
    UsageRatio   float64  // token_count / max_tokens
}
```

#### 7.1.6 Tool Sandbox

```go
// tools.go — Tool discovery and execution

type ToolRegistry interface {
    // ListTools returns all registered tool definitions.
    ListTools() []ToolDefinition

    // GetTool returns a specific tool by name.
    GetTool(name string) (*ToolDefinition, error)

    // LoadManifests reads tool manifests from the configured directory.
    LoadManifests(dir string) error
}

type ToolExecutor interface {
    // Execute runs a tool and returns its output.
    // It MUST respect the context for cancellation and the configured timeout.
    Execute(ctx context.Context, req *ToolExecRequest) (*ToolExecResult, error)
}

type ToolExecRequest struct {
    RequestID  string
    CallID     string
    SessionID  string
    ToolName   string
    Arguments  json.RawMessage
    WorkingDir string
    TimeoutMS  int
}

type ToolExecResult struct {
    CallID     string
    ToolName   string
    Status     string // "success", "error", "timeout"
    Output     string
    Stderr     string
    ExitCode   int
    DurationMS int64
    Truncated  bool
    Error      *ToolError // nil on success
}

type ToolError struct {
    Code      string // "TOOL_NOT_FOUND", "TOOL_TIMEOUT", "TOOL_EXEC_FAILED"
    Message   string
    Retryable bool
}

type ToolDefinition struct {
    Name        string
    Description string
    Version     string
    Type        string                 // "executable", "builtin"
    Parameters  json.RawMessage        // JSON Schema
    Executable  *ExecutableConfig      // nil for builtins
}

type ExecutableConfig struct {
    Command    string
    ArgsFormat string // "json_stdin", "cli_args"
    TimeoutMS  int
}
```

#### 7.1.7 Inference Orchestrator

```go
// orchestrator.go — The main inference loop (state machine)

type Orchestrator interface {
    // RunInference executes the full inference state machine:
    // VALIDATING → BUILDING_CONTEXT → [COMPACTING] → INFERRING
    //   → [EXECUTING_TOOLS → INFERRING]* → COMPLETING
    //
    // It writes streaming chunks to the provided ResponseWriter.
    // It publishes events to the EventBus.
    // It persists state via the SessionManager.
    RunInference(ctx context.Context, req *InferenceRequest, w ResponseWriter) error
}

type InferenceRequest struct {
    RequestID   string
    SessionID   string        // optional — if empty, stateless mode
    Model       string
    Messages    []Message     // provided in the HTTP request
    Tools       []ToolDefinition
    Temperature float64
    MaxTokens   int
    Stream      bool
    UserID      string
}

// ResponseWriter abstracts SSE streaming and non-streaming HTTP responses.
type ResponseWriter interface {
    WriteChunk(chunk *ChatChunk) error
    WriteError(err *APIError) error
    Flush()
    Done()
}

// OrchestratorConfig holds the loop's safety limits.
type OrchestratorConfig struct {
    MaxToolRounds       int           // default: 10
    MaxToolCallsPerRound int          // default: 5
    MaxTotalToolCalls   int           // default: 25
    CompactionThreshold float64       // default: 0.9
    RequestTimeout      time.Duration // default: 120s
}
```

#### 7.1.8 Compaction Engine

```go
// compaction.go — Context window compression

type CompactionEngine interface {
    // Compact takes a context window that exceeds the token limit
    // and returns a compressed version.
    Compact(ctx context.Context, req *CompactionRequest) (*CompactionResult, error)
}

type CompactionRequest struct {
    SessionID     string
    Model         string
    SystemPrompt  string
    Messages      []Message
    CurrentTokens int
    MaxTokens     int
    PreserveLastN int
    Strategy      string // "summarize", "truncate"
}

type CompactionResult struct {
    SessionID          string
    Status             string // "compacted", "truncated", "skipped"
    SummaryMessage     *Message
    PreservedMessages  []Message
    RemovedMessageIDs  []string
    OriginalTokenCount int
    NewTokenCount      int
}
```

#### 7.1.9 Event Bus

```go
// eventbus.go — Internal pub/sub for system events

type EventBus interface {
    // Publish sends an event to all subscribers of the given topic.
    Publish(event Event)

    // Subscribe returns a channel that receives events for the given topics.
    // The caller MUST call Unsubscribe when done.
    Subscribe(topics ...string) (Subscription, error)
}

type Subscription interface {
    // Events returns a read-only channel of events.
    Events() <-chan Event

    // Unsubscribe stops receiving events and closes the channel.
    Unsubscribe()
}

type Event struct {
    Type      string          // from the Event Type Catalog (Section 2.2)
    SessionID string
    RequestID string
    Timestamp time.Time
    Payload   json.RawMessage
}
```

#### 7.1.10 Auth Middleware

```go
// auth.go — Authentication and authorization

type Authenticator interface {
    // Authenticate validates the request and returns a user context.
    // Returns nil error and a default user if auth is disabled.
    Authenticate(r *http.Request) (*UserContext, error)

    // Middleware returns an http.Handler middleware that calls Authenticate.
    Middleware() func(http.Handler) http.Handler
}

type UserContext struct {
    UserID string
    Role   string // "admin", "user"
}

// NoOpAuthenticator always returns UserContext{UserID: "default", Role: "admin"}
// APIKeyAuthenticator validates Bearer token against configured key
// (Future) JWTAuthenticator validates JWT tokens
```

### 7.2 Event Type Enum (Go Constants)

```go
const (
    // Inference lifecycle
    EventInferenceStarted   = "inference.started"
    EventInferenceToken     = "inference.token"
    EventInferenceToolCall  = "inference.tool_call"
    EventInferenceToolResult = "inference.tool_result"
    EventInferenceCompleted = "inference.completed"
    EventInferenceError     = "inference.error"

    // Compaction
    EventCompactionStarted   = "compaction.started"
    EventCompactionCompleted = "compaction.completed"

    // Session lifecycle
    EventSessionCreated = "session.created"
    EventSessionUpdated = "session.updated"
    EventSessionDeleted = "session.deleted"

    // System
    EventModelStatusChanged = "model.status_changed"
    EventSystemError        = "system.error"
    EventPing               = "ping"
    EventPong               = "pong"
)
```

### 7.3 Module Initialization Order

The binary's `main()` MUST initialize modules in this exact order (respecting the dependency graph):

```
1.  Config        ← parse flags, read file, read env vars
2.  Storage       ← open DB connection, run migrations
3.  EventBus      ← create in-memory pub/sub
4.  Auth          ← create authenticator based on config
5.  ModelRegistry ← probe configured providers, cache model list
6.  SessionManager← wire to Storage
7.  ToolRegistry  ← load manifests from tools_dir
8.  ToolExecutor  ← wire to ToolRegistry + config
9.  CompactionEngine ← wire to ModelRegistry (needs a provider to summarize)
10. Orchestrator  ← wire to SessionManager, ModelRegistry, ToolExecutor,
                     CompactionEngine, EventBus
11. API Gateway   ← wire routes, attach Auth middleware, attach Orchestrator
12. Web Server    ← mount embedded FS at /chat and /inspector
13. HTTP Server   ← start listening, block on shutdown signal
```

### 7.4 Directory Structure Proposal

```
forge/
├── cmd/
│   └── forge/
│       └── main.go              # Entry point, wiring, init order
├── internal/
│   ├── config/
│   │   ├── config.go            # Config struct + LoadConfig()
│   │   └── config_test.go
│   ├── storage/
│   │   ├── store.go             # Store interface
│   │   ├── sqlite.go            # SQLite implementation
│   │   ├── postgres.go          # PostgreSQL implementation
│   │   ├── migrations/          # SQL migration files
│   │   └── store_test.go
│   ├── auth/
│   │   ├── auth.go              # Authenticator interface
│   │   ├── noop.go              # NoOpAuthenticator
│   │   ├── apikey.go            # APIKeyAuthenticator
│   │   └── auth_test.go
│   ├── provider/
│   │   ├── provider.go          # InferenceProvider interface
│   │   ├── registry.go          # ModelRegistry implementation
│   │   ├── ollama/
│   │   │   ├── ollama.go        # Ollama provider
│   │   │   └── ollama_test.go
│   │   ├── openai/
│   │   │   ├── openai.go        # OpenAI provider
│   │   │   └── openai_test.go
│   │   └── anthropic/
│   │       ├── anthropic.go     # Anthropic provider
│   │       └── anthropic_test.go
│   ├── session/
│   │   ├── session.go           # SessionManager interface + impl
│   │   ├── context.go           # ContextWindow logic
│   │   └── session_test.go
│   ├── orchestrator/
│   │   ├── orchestrator.go      # State machine implementation
│   │   ├── states.go            # State definitions + transitions
│   │   └── orchestrator_test.go
│   ├── compaction/
│   │   ├── compaction.go        # CompactionEngine interface + impl
│   │   └── compaction_test.go
│   ├── tools/
│   │   ├── registry.go          # ToolRegistry (manifest loading)
│   │   ├── executor.go          # ToolExecutor (local + docker)
│   │   ├── manifest.go          # Manifest parsing
│   │   └── tools_test.go
│   ├── eventbus/
│   │   ├── eventbus.go          # EventBus interface + in-memory impl
│   │   └── eventbus_test.go
│   └── api/
│       ├── router.go            # Chi router setup, route registration
│       ├── handlers_chat.go     # /v1/chat/completions
│       ├── handlers_sessions.go # /v1/sessions CRUD
│       ├── handlers_models.go   # /v1/models
│       ├── handlers_ws.go       # /ws WebSocket
│       ├── middleware.go        # Logging, recovery, CORS
│       ├── sse.go               # SSE ResponseWriter implementation
│       └── api_test.go
├── ui/                          # React frontend (embedded)
│   ├── src/
│   ├── package.json
│   └── dist/                    # Built output (go:embed target)
├── tools/                       # Tool manifest directory
│   ├── terminal_exec.json
│   └── file_read.json
├── forge.yaml                   # Default config file
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── IMPLEMENTATION_PLAN.md
└── ARCHITECTURE_REVIEW.md       # This document
```

---

## 8. Summary of Recommendations

### Critical (Must Fix Before Coding)

| # | Issue | Recommendation |
|:--|:---|:---|
| 1 | **Module count too low** | Split from 6 → 10 modules as defined in Section 1.3 |
| 2 | **No formal contracts** | Adopt all 12 contracts from Section 2 before writing any handler code |
| 3 | **No state machine** | Implement the inference orchestrator as the formal state machine from Section 5 |
| 4 | **No tool loop limits** | Add `max_tool_rounds`, `max_tool_calls_per_round`, `max_total_tool_calls` from Section 5.4 |
| 5 | **Config is implicit** | Create the Config module as the very first thing built — every other module depends on it |
| 6 | **user_id missing** | Add `user_id` to all DB records from Day 1 (Section 6.2) |

### Important (Should Do)

| # | Issue | Recommendation |
|:--|:---|:---|
| 7 | Tool plugins | Adopt the stdin/stdout JSON manifest system (Section 3.3) |
| 8 | Event Bus | Elevate to first-class module — it connects everything (Section 7.1.9) |
| 9 | Session lifecycle | Implement the full state machine from Section 4.1 |
| 10 | Cache layer | Write-through LRU cache for active sessions (Section 4.4) |

### Future (Design For, Don't Build Yet)

| # | Issue | Recommendation |
|:--|:---|:---|
| 11 | Multi-user | Design interfaces to accept `UserContext` now, implement later |
| 12 | Horizontal scaling | Use interfaces (Store, EventBus) that can be swapped for distributed implementations |
| 13 | UI plugins | Not needed for v1 — single opinionated UI is correct |
| 14 | Queue-based scheduling | Use `sync.Mutex` for v1, design interface for future queue |

---

> **Final Assessment:** The IMPLEMENTATION_PLAN.md has the right vision but needs the structural rigor defined in this review before a single line of code is written. The phased approach is sound — but each phase should be building against the interfaces defined here, not discovering them as it goes. **Contract-first, implementation second.**
