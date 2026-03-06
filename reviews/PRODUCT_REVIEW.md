# 🔍 Forge — Product Manager Review

> **Reviewer:** Technical Product Manager (Orchestrator Nexus)
> **Date:** 2025-07-15
> **Document Under Review:** `IMPLEMENTATION_PLAN.md`
> **Verdict:** Strong technical foundation, but the plan reads like an **engine spec**, not a **product spec**. It describes *how* the system works internally but barely addresses *what the user sees and does*. This review fills those gaps.

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Feature Prioritization & Scope](#2-feature-prioritization--scope)
3. [User Stories & Personas](#3-user-stories--personas)
4. [Model Versatility & Provider Support](#4-model-versatility--provider-support)
5. [Chat Features Gap Analysis](#5-chat-features-gap-analysis)
6. [Deployment & Distribution](#6-deployment--distribution)
7. [Risk & Gaps](#7-risk--gaps)
8. [Prioritized Feature Backlog](#8-prioritized-feature-backlog)
9. [Final Recommendations](#9-final-recommendations)

---

## 1. Executive Summary

### What Forge Gets Right
- **Single-binary distribution** is a killer differentiator. The self-hosting market is drowning in Docker Compose files with 6+ services. A `curl | tar | ./forge` install story is *chef's kiss*.
- **OpenAI-compatible API** is the right choice — it makes Forge a drop-in replacement for existing tools.
- **Inspector UI** is a genuinely novel idea. No competitor offers a real-time token/context visualizer as a first-class feature. This is Forge's secret weapon for the developer audience.
- **Tool Sandbox with Pause-Execute-Resume** is architecturally ambitious and, if done well, will leapfrog competitors who bolt on tool use as an afterthought.

### What Forge Gets Wrong (or Doesn't Address)
- **No conversation management at all.** The plan has no mention of: conversation list, rename, delete, search, folders, tags, or export. This is table-stakes for any chat UI.
- **No user-facing model selection.** How does a user pick a model? Switch mid-conversation? Compare outputs?
- **No settings UI.** Temperature, system prompt, max tokens — where does the user configure these?
- **Phases are backend-heavy, frontend-light.** The Chat UI is a single bullet point in Phase 3. It needs to be decomposed into 15+ sub-features.
- **No multi-user story.** Even for self-hosting, users want separate workspaces. The plan goes from "single user SQLite" to "PostgreSQL hosted" with nothing in between.
- **Provider list is too narrow.** Missing Google Gemini, Groq, Mistral, Together AI, Azure OpenAI, AWS Bedrock — all of which are table-stakes for 2025.

### The Core Product Question
> **Who is Forge for, and why would they choose it over Open WebUI?**

The plan doesn't answer this. I'll answer it below.

---

## 2. Feature Prioritization & Scope

### 2.1 Phase Ordering — Critique

The current phases are ordered by *architectural dependency*, which is fine for engineering but wrong for product delivery. Here's why:

| Current Phase | Problem |
|:---|:---|
| Phase 1: Foundation | ✅ Correct — you need plumbing first. |
| Phase 2: Inference & Tooling | ⚠️ Tool execution is complex and risky. Shipping tool use *before* a usable chat UI means the team spends weeks on tool plumbing while no one can actually use Forge for basic chat. |
| Phase 3: Web Interface | 🚩 The chat UI is Phase 3?! Users can't even *use* Forge until Phase 3. This should be Phase 2. |
| Phase 4: Context Compaction | ✅ Correct placement — this is an optimization. |
| Phase 5: Production | ⚠️ Auth and Postgres should be partially addressed earlier. At minimum, a `FORGE_API_KEY` env var should gate the API from Phase 1. |

### 2.2 Recommended Phase Reordering

```
Phase 1: Skeleton & Streaming (unchanged — API + mock provider)
Phase 2: Chat UI MVP (usable chat with real model, conversation CRUD)
Phase 3: Multi-Provider (Ollama, OpenAI, Anthropic, Gemini at minimum)
Phase 4: Context Management (persistence, compaction, token counting)
Phase 5: Tool Execution (the Pause-Execute-Resume loop)
Phase 6: Inspector & Observability (the Inspector UI, event bus)
Phase 7: Production Hardening (auth, Postgres, Docker, deployment)
Phase 8: Power Features (branching, templates, sharing, multi-modal)
```

**Rationale:** A user should be able to `./forge` and chat with a model by the end of Phase 2. Everything else is layered on top of a working product.

### 2.3 MVP Definition (What Ships First)

The MVP is **Phases 1-3** above. It must include:

| Feature | Priority | Notes |
|:---|:---|:---|
| Single binary that starts an HTTP server | P0 | The brand promise |
| Connect to at least one real provider (Ollama) | P0 | Can't demo without it |
| Chat UI with streaming responses | P0 | Core UX |
| Conversation list (create, rename, delete) | P0 | Table stakes |
| Markdown + code block rendering | P0 | Every competitor has this |
| Model selector dropdown | P0 | Users need to pick a model |
| System prompt configuration | P1 | Power users expect this |
| Dark/light mode | P1 | Self-hosters care about this |
| Basic settings page (API keys, provider URLs) | P0 | Can't use cloud providers without this |
| SQLite persistence | P0 | Conversations must survive restart |

**What is NOT in the MVP:**
- Tool execution (Phase 5)
- Inspector UI (Phase 6)
- PostgreSQL (Phase 7)
- Multi-user / auth (Phase 7)
- Context compaction (Phase 4 — use simple truncation as a stopgap)

### 2.4 Competitive Landscape

| Product | Strengths | Weaknesses | Forge Opportunity |
|:---|:---|:---|:---|
| **Open WebUI** | Feature-rich, large community, Ollama-native | Heavy (Docker + multiple services), complex config, UI is cluttered | Simplicity. Single binary. Clean UI. |
| **LibreChat** | Multi-provider, good UI | Node.js + MongoDB + Meilisearch = complex stack | Single binary with no dependencies |
| **AnythingLLM** | Desktop app, document embedding | Electron bloat, limited model support | Lighter, faster, web-native |
| **Jan** | Desktop-first, offline | Limited to local models, no web UI | Web-first with local+cloud models |
| **LM Studio** | Beautiful UI, easy model downloads | Closed source, desktop only, no tool use | Open source, tool execution, self-hostable |
| **Msty** | Multi-provider, clean UI | Closed source, desktop only | Open source, self-hostable |

### 2.5 Forge's Differentiators (The Pitch)

> **Forge is the fastest way to self-host a multi-model AI chat.** Download one binary. Run it. You're chatting. No Docker. No databases to manage. No YAML files. Just `./forge`.

The three pillars of differentiation:
1. **Zero-dependency deployment** — Single static binary, SQLite embedded, frontend embedded.
2. **Inspector UI** — See exactly what the model sees. Token counts, raw context, tool execution traces. No other chat UI does this.
3. **Tool execution built-in** — Not a plugin, not an extension. First-class Pause-Execute-Resume tool loop with sandboxing.

---

## 3. User Stories & Personas

### 3.1 Target Personas

#### Persona 1: **Solo Developer ("Dev Dave")**
- Runs Ollama locally with a 7B model
- Wants a better UI than the Ollama terminal
- Cares about: speed, keyboard shortcuts, code highlighting, tool use
- Doesn't want: Docker, databases, accounts, complexity

#### Persona 2: **AI Power User ("Power-User Priya")**
- Uses Claude, GPT-4, and local models depending on the task
- Wants to switch models mid-conversation
- Wants to compare outputs, edit messages, regenerate responses
- Cares about: model flexibility, conversation management, prompt templates

#### Persona 3: **Self-Hosting Enthusiast ("Homelab Hank")**
- Runs everything on a home server or VPS
- Wants a private, self-hosted alternative to ChatGPT
- Cares about: privacy, easy deployment, low resource usage, stability
- Doesn't want: telemetry, cloud dependencies, complex setup

#### Persona 4: **Small Team Lead ("Team Tanya")** *(Phase 7+)*
- Wants to share a Forge instance with 3-5 team members
- Needs basic auth, separate conversation spaces, shared prompts
- Cares about: access control, cost tracking per user, audit logs

### 3.2 Key User Stories by Phase

#### Phase 1-2: MVP Stories

**US-001: First Run**
> **As** Dev Dave,
> **I want to** download a single binary, run it, and immediately start chatting with my local Ollama model,
> **So that** I don't waste time on setup and configuration.

**Acceptance Criteria:**
- [ ] `./forge` starts and opens a browser tab to `http://localhost:8080/chat`
- [ ] If Ollama is running on `localhost:11434`, Forge auto-detects it and lists available models
- [ ] User can select a model and send a message within 30 seconds of first run
- [ ] No configuration files required for local Ollama usage

---

**US-002: Basic Chat**
> **As** Dev Dave,
> **I want to** send messages and receive streamed responses with proper Markdown rendering,
> **So that** code snippets and formatted text display correctly.

**Acceptance Criteria:**
- [ ] Messages stream token-by-token with visible typing indicator
- [ ] Markdown renders: headers, bold, italic, lists, links, tables
- [ ] Code blocks render with syntax highlighting and a "Copy" button
- [ ] LaTeX/math notation renders correctly (KaTeX)
- [ ] User can stop generation mid-stream with a "Stop" button

---

**US-003: Conversation Management**
> **As** Power-User Priya,
> **I want to** create, rename, delete, and search my conversations,
> **So that** I can organize my chat history.

**Acceptance Criteria:**
- [ ] Left sidebar shows conversation list, sorted by last-modified
- [ ] New conversation created via "New Chat" button or `Ctrl+N`
- [ ] Conversations auto-titled based on first message (via LLM summarization)
- [ ] User can rename by clicking on conversation title
- [ ] User can delete with confirmation dialog
- [ ] Search bar filters conversations by title and content

---

**US-004: Model Selection**
> **As** Power-User Priya,
> **I want to** switch between models (local and cloud) from the chat UI,
> **So that** I can use the right model for each task.

**Acceptance Criteria:**
- [ ] Model selector dropdown in the chat header
- [ ] Shows provider icon + model name (e.g., 🦙 Ollama / llama3.2, 🤖 OpenAI / gpt-4o)
- [ ] Switching models mid-conversation carries forward the chat history
- [ ] Models that require API keys show a "Configure" link if key is missing
- [ ] Favorite/pin frequently used models to the top of the list

---

**US-005: Settings & Provider Configuration**
> **As** Homelab Hank,
> **I want to** configure API keys and provider URLs through a settings page,
> **So that** I don't have to edit config files or restart the server.

**Acceptance Criteria:**
- [ ] Settings page at `/settings` accessible from the sidebar
- [ ] Provider section: Add/edit/remove providers (Ollama URL, OpenAI key, Anthropic key, etc.)
- [ ] API keys are stored encrypted in the database, never exposed in the UI after entry
- [ ] Changes take effect immediately without server restart
- [ ] "Test Connection" button validates provider connectivity

---

#### Phase 4: Context Management Stories

**US-006: Long Conversations**
> **As** Dev Dave,
> **I want** Forge to handle long conversations without errors or degraded quality,
> **So that** I can have extended coding sessions without losing context.

**Acceptance Criteria:**
- [ ] Token usage indicator shows current/max tokens for the active model
- [ ] When context approaches 90% capacity, Forge automatically compacts older messages
- [ ] Compaction preserves the system prompt and last 5 messages verbatim
- [ ] User is notified when compaction occurs (subtle toast notification)
- [ ] Original messages remain accessible in a "Full History" view

---

#### Phase 5: Tool Execution Stories

**US-007: Tool Use**
> **As** Dev Dave,
> **I want** the model to be able to run shell commands, search the web, and read files on my behalf,
> **So that** I can use Forge as a coding assistant.

**Acceptance Criteria:**
- [ ] Tool execution shows a real-time progress card in the chat (tool name, status, duration)
- [ ] User can approve/deny tool execution (configurable: always-ask, auto-approve, deny-all)
- [ ] Tool output is rendered in a collapsible card below the progress indicator
- [ ] If a tool fails, the error is shown to both the user and the model
- [ ] Tool execution timeout is configurable (default: 30s)

---

#### Phase 6: Inspector Stories

**US-008: Context Inspector**
> **As** Dev Dave,
> **I want to** see exactly what the model sees — the full context window, token counts, and tool call traces,
> **So that** I can debug prompt issues and understand model behavior.

**Acceptance Criteria:**
- [ ] Inspector accessible via `/inspector` or a toggle button in the chat UI
- [ ] Shows the raw message array sent to the model (system, user, assistant, tool messages)
- [ ] Real-time token count per message and total
- [ ] Highlights which messages will be compacted next
- [ ] Tool call/result pairs are visually linked
- [ ] Can be opened side-by-side with the chat (split-pane layout)

---

### 3.3 Onboarding Experience

The onboarding flow is **critical** and completely absent from the plan. Here's what it should look like:

```
1. User downloads `forge` binary (or `brew install forge`)
2. User runs `./forge`
3. Terminal prints:
   ┌──────────────────────────────────────┐
   │  🔥 Forge v0.1.0                     │
   │  Chat UI:  http://localhost:8080/chat │
   │  API:      http://localhost:8080/v1   │
   │  Inspector: http://localhost:8080/inspector │
   │                                       │
   │  ✅ Detected Ollama at localhost:11434│
   │     Models: llama3.2, codellama       │
   │  ⚠️  No OpenAI key configured         │
   │     Set via: Settings UI or           │
   │     OPENAI_API_KEY env var            │
   └──────────────────────────────────────┘
4. Browser opens automatically to /chat
5. User sees a welcome screen:
   "Welcome to Forge 🔥
    Select a model to get started →  [Model Dropdown]
    Or configure providers in [Settings]"
6. User selects a model, types a message, gets a streamed response.
7. Total time from download to first response: < 60 seconds.
```

---

## 4. Model Versatility & Provider Support

### 4.1 Provider Coverage — Current vs Required

| Provider | In Plan? | Priority | Notes |
|:---|:---|:---|:---|
| **Ollama** | ✅ Yes | P0 (MVP) | Primary local provider. Auto-detect on startup. |
| **OpenAI** | ✅ Yes | P0 (MVP) | GPT-4o, o3, etc. Most users have an OpenAI key. |
| **Anthropic** | ✅ Yes | P0 (MVP) | Claude is the #2 cloud model. Extended thinking support needed. |
| **Local llama.cpp** | ✅ Yes | P1 | For users running llama-server directly (no Ollama). |
| **Google Gemini** | ❌ No | P0 (MVP) | Gemini 2.5 Pro is a top-tier model. Cannot ship without it. |
| **Groq** | ❌ No | P1 | Fastest inference API. Power users love it. OpenAI-compatible, so nearly free to add. |
| **Mistral** | ❌ No | P2 | Growing provider. OpenAI-compatible API. |
| **Together AI** | ❌ No | P2 | Popular for open models. OpenAI-compatible. |
| **Azure OpenAI** | ❌ No | P2 | Enterprise users. Different auth model. |
| **AWS Bedrock** | ❌ No | P3 | Enterprise. Complex auth (IAM). Defer. |
| **OpenRouter** | ❌ No | P1 | Meta-provider. One key, all models. Power users love it. |
| **Any OpenAI-compatible** | ❌ No | P0 (MVP) | Custom endpoint + API key. Covers Groq, Together, LM Studio, vLLM, etc. |

### 4.2 Provider Architecture Recommendation

**The plan's `Go Interfaces` approach is correct**, but the implementation should prioritize a **"Custom OpenAI-Compatible" provider** that lets users point Forge at *any* endpoint. This single feature covers 80% of the provider landscape because most providers clone the OpenAI API format.

```
Provider Interface:
├── OllamaProvider          (auto-detected, dedicated)
├── OpenAIProvider          (openai.com, dedicated)
├── AnthropicProvider       (anthropic.com, dedicated — different API format)
├── GeminiProvider          (Google, dedicated — different API format)
├── OpenAICompatibleProvider (generic — covers Groq, Together, Mistral, 
│                             OpenRouter, LM Studio, vLLM, LocalAI, etc.)
└── LlamaCppProvider        (direct llama-server, dedicated)
```

**Key insight:** Only Anthropic and Google Gemini have meaningfully different APIs. Everyone else is OpenAI-compatible. So the provider work is really:
- 3 dedicated providers (OpenAI, Anthropic, Gemini)
- 1 auto-detecting provider (Ollama)
- 1 generic provider (OpenAI-compatible, covers everything else)
- 1 local provider (llama.cpp direct)

### 4.3 Model Switching UX

**Product requirements for model selection:**

1. **Model Selector** — Dropdown in the chat header showing all available models grouped by provider
2. **Per-Conversation Model** — Each conversation remembers which model was used
3. **Mid-Conversation Switching** — User can switch models and continue the same conversation. Previous messages are carried forward.
4. **Model Capabilities Badges** — Visual indicators for model capabilities:
   - 👁️ Vision (accepts images)
   - 🔧 Tools/Function Calling
   - 🧠 Extended Thinking (Anthropic, o3)
   - 📎 File Upload
   - 🎨 Image Generation
5. **Model Favorites** — Pin frequently used models to the top
6. **Model Info Tooltip** — Hover to see: context window size, pricing (if cloud), provider

### 4.4 Model Registry (Not a Marketplace)

**Recommendation: No.** A "model marketplace" implies downloading models, which is Ollama's job. Forge should not duplicate Ollama.

Instead, implement a **Model Registry** — an internal catalog of known models with metadata:
```json
{
  "id": "gpt-4o",
  "provider": "openai",
  "display_name": "GPT-4o",
  "context_window": 128000,
  "capabilities": ["vision", "tools", "json_mode"],
  "input_cost_per_mtok": 2.50,
  "output_cost_per_mtok": 10.00
}
```
This powers:
- Accurate token counting per model
- Capability-aware UI (hide image upload if model doesn't support vision)
- Cost estimation per conversation (nice-to-have)

The registry ships as a bundled JSON file, updatable via a `forge update-models` CLI command or a periodic fetch from a public GitHub-hosted registry.

### 4.5 Handling Different Model Capabilities

This is the biggest product design challenge. Different models support different features:

| Capability | Models | UX Impact |
|:---|:---|:---|
| **Vision** | GPT-4o, Claude 3.5, Gemini, Llava | Show image upload button only when model supports it |
| **Tool/Function Calling** | GPT-4o, Claude, Gemini, some local | Enable/disable tool execution per model |
| **Extended Thinking** | Claude 3.5 (thinking), o3 | Show thinking blocks in UI |
| **JSON Mode** | GPT-4o, Gemini | Internal use for structured outputs |
| **Streaming** | Most | Fallback to polling if not supported |
| **System Prompt** | Most (except some older models) | Hide system prompt field if unsupported |

**Rule:** The UI should **gracefully degrade**. If a model doesn't support vision, the image upload button is hidden — not shown with an error. The capability metadata from the model registry drives this.

---

## 5. Chat Features Gap Analysis

The current plan describes the chat UI in **one bullet point**. Here's what a competitive chat UI actually requires:

### 5.1 Feature Matrix: Forge vs Competitors

| Feature | ChatGPT | Claude.ai | Open WebUI | Forge (Planned) | Forge (Needed) |
|:---|:---|:---|:---|:---|:---|
| Streaming responses | ✅ | ✅ | ✅ | ✅ | — |
| Markdown + code highlighting | ✅ | ✅ | ✅ | ✅ | — |
| Conversation list | ✅ | ✅ | ✅ | ❌ | **P0** |
| Conversation search | ✅ | ✅ | ✅ | ❌ | **P1** |
| Conversation folders/tags | ✅ | ❌ | ✅ | ❌ | **P2** |
| Message editing | ✅ | ✅ | ✅ | ❌ | **P0** |
| Response regeneration | ✅ | ✅ | ✅ | ❌ | **P0** |
| Conversation branching | ✅ | ❌ | ✅ | ❌ | **P2** |
| System prompt per chat | ✅ | ✅ | ✅ | ❌ | **P0** |
| Model selector | ✅ | ❌ | ✅ | ❌ | **P0** |
| Dark/light mode | ✅ | ✅ | ✅ | ❌ | **P0** |
| Image upload (vision) | ✅ | ✅ | ✅ | ❌ | **P1** |
| File upload (RAG) | ✅ | ✅ | ✅ | ❌ | **P3** |
| Export conversation | ❌ | ❌ | ✅ | ❌ | **P1** |
| Prompt templates | ✅ | ❌ | ✅ | ❌ | **P2** |
| Artifacts/Canvas | ❌ | ✅ | ❌ | ❌ | **P2** |
| Tool use visualization | ❌ | ✅ | ❌ | ✅ | — |
| Context inspector | ❌ | ❌ | ❌ | ✅ | — (unique!) |
| Keyboard shortcuts | ✅ | ✅ | ❌ | ❌ | **P1** |
| Mobile responsive | ✅ | ✅ | ⚠️ | ❌ | **P1** |
| Stop generation | ✅ | ✅ | ✅ | ❌ | **P0** |
| Copy message | ✅ | ✅ | ✅ | ❌ | **P0** |
| Thinking/reasoning blocks | ❌ | ✅ | ❌ | ❌ | **P1** |

### 5.2 Deep Dive: Critical Missing Features

#### 5.2.1 Message Editing & Regeneration (P0)

**Why it matters:** This is the #1 feature users expect. Without it, a typo means starting over.

**Spec:**
- Hover over any user message → "Edit" button appears
- Editing a message resubmits from that point (messages after it are removed)
- "Regenerate" button on assistant messages re-sends the same context
- Regeneration keeps the previous response accessible (carousel: ← 1/3 →)

#### 5.2.2 Conversation Branching/Forking (P2)

**Why it matters:** Power users (Priya) want to explore different paths from the same conversation point.

**Spec:**
- When user edits a message, a "branch" is created
- Branch indicator in the sidebar (conversation shows branch icon)
- Branch navigator in the chat (← Branch 1 / Branch 2 →)
- Can be deferred to Phase 8, but the **data model must support it from Phase 1** (tree structure, not flat array)

**⚠️ Critical Architecture Note:** If the message storage schema uses a flat array, branching becomes a painful migration later. **Design the schema as a tree from day one**, even if the UI only shows linear conversations initially.

```
messages table:
  id          TEXT PRIMARY KEY
  conversation_id  TEXT
  parent_id   TEXT (nullable — null = root)
  role        TEXT (system | user | assistant | tool)
  content     TEXT
  model       TEXT
  created_at  TIMESTAMP
  metadata    JSON
```

#### 5.2.3 Conversation Export (P1)

**Spec:**
- Export as: Markdown, JSON (ChatGPT-compatible format), Plain Text
- Export single conversation or bulk export all
- Import from: ChatGPT export JSON, Claude export

#### 5.2.4 Prompt Templates / Library (P2)

**Spec:**
- Pre-built library of system prompts (e.g., "Code Reviewer", "Technical Writer", "Translator")
- User can create and save custom templates
- Templates can include: system prompt, model selection, temperature
- "Use Template" button when creating a new conversation

#### 5.2.5 Artifacts / Canvas Mode (P2-P3)

**Why it matters:** Claude's Artifacts feature is extremely popular. Users want to see generated code/documents in a side panel they can iterate on.

**Spec (simplified):**
- When model generates a code block or document, user can "Open as Artifact"
- Artifact opens in a right-side panel
- User can ask the model to modify the artifact in-place
- Artifact has: Copy, Download, Version History
- **Defer full implementation** but reserve UI space for it

#### 5.2.6 Multi-Modal Support (P1 for images, P3 for files/audio)

**Image Upload (P1):**
- Paste image from clipboard (Ctrl+V)
- Drag & drop image into chat input
- Image preview before sending
- Only shown when selected model supports vision

**File Upload (P3 — requires RAG pipeline):**
- Upload PDF, code files, text files
- Files are embedded/chunked and added to context
- This is a large feature — defer to a dedicated phase

**Audio (P3+):**
- Voice input via browser API
- Whisper transcription
- Defer significantly

---

## 6. Deployment & Distribution

### 6.1 Single Binary — Trade-offs Analysis

| Advantage | Disadvantage |
|:---|:---|
| Zero dependencies — just download and run | Binary size grows with embedded frontend assets |
| No Node.js, no Python, no Docker required | Updating frontend requires rebuilding entire binary |
| Easy to distribute via GitHub Releases | Can't customize frontend without rebuilding |
| Simple process model (one PID to manage) | SQLite limits concurrent write throughput |
| Trivial to run behind a reverse proxy | No horizontal scaling (single instance only) |

**Verdict:** The single binary approach is **the right call for MVP and the core audience** (Dev Dave, Homelab Hank). The disadvantages only matter at scale, which is a Phase 7+ problem.

**Mitigation for frontend updates:** Consider a `--ui-dir` flag that optionally serves from a local directory instead of the embedded filesystem. This allows frontend development without rebuilding the Go binary, and lets power users customize the UI.

### 6.2 Desktop App — Should Forge Build One?

**Recommendation: No. Not now. Maybe never.**

**Reasoning:**
- Forge already opens in a browser. For Dev Dave, that *is* the desktop app.
- Electron/Tauri adds massive build complexity and a second distribution channel.
- Jan, LM Studio, and Msty already own the "native desktop AI chat" space.
- Forge's differentiator is *self-hosting* and *web-native*, not desktop packaging.
- If there's demand later, a simple Tauri wrapper around `http://localhost:8080` could be added with minimal effort. But it's not a priority.

**Alternative:** Provide a `.desktop` file for Linux and a Launch Agent plist for macOS so users can run Forge as a background service that auto-starts on boot. This gives the "native app" feel without the Electron baggage.

### 6.3 Plugin / Extension Ecosystem

**Recommendation: Design for it, but don't build it yet.**

The Tool Manifest System (Phase 2 in the current plan) is the natural extension point. If tools are defined as JSON files in a `~/.forge/tools/` directory, that *is* a plugin system — just without a fancy registry.

**Phase 1-7:** Tools are JSON manifests pointing to local scripts. Users share tools as GitHub gists.
**Phase 8+:** Formal plugin registry, `forge install <plugin>` CLI command, community marketplace.

**Key design decision:** Tools should be the **only** extension point. Don't build separate systems for "plugins", "extensions", and "tools". One system. One format.

### 6.4 Distribution Channels

| Channel | Priority | Notes |
|:---|:---|:---|
| GitHub Releases (binary) | P0 | Linux amd64, arm64. macOS amd64, arm64. Windows amd64. |
| Docker Hub image | P0 | `docker run -p 8080:8080 forge` |
| Homebrew tap | P1 | `brew install user/tap/forge` |
| AUR package | P2 | Arch Linux users will ask for this day one |
| Nix flake | P2 | Nix community is vocal and loyal |
| `curl` one-liner | P0 | `curl -fsSL https://forge.dev/install.sh \| sh` |

---

## 7. Risk & Gaps

### 7.1 What's Missing from the Plan

| Gap | Severity | Impact |
|:---|:---|:---|
| **No conversation CRUD** | 🔴 Critical | Users can't manage conversations. This is the core UX. |
| **No model selection UI** | 🔴 Critical | Users can't pick a model. Showstopper. |
| **No settings/configuration UI** | 🔴 Critical | API keys must be configurable from the browser. |
| **No onboarding flow** | 🟡 High | First-run experience is undefined. |
| **No message editing/regeneration** | 🟡 High | Table-stakes feature missing from spec. |
| **No error handling spec** | 🟡 High | What happens when: model is offline? API key is invalid? Context overflows? Network drops mid-stream? |
| **No keyboard shortcuts spec** | 🟢 Medium | Dev Dave expects `Ctrl+N`, `Ctrl+Enter`, `Ctrl+/`, etc. |
| **No accessibility spec** | 🟡 High | Screen readers, keyboard navigation, ARIA labels, color contrast. |
| **No mobile responsiveness spec** | 🟢 Medium | Not critical for MVP but expected long-term. |
| **No telemetry/analytics decision** | 🟢 Medium | Self-hosters will want explicit "no telemetry" guarantee. Make this a design principle. |
| **No data model / schema spec** | 🔴 Critical | The database schema for conversations, messages, settings is undefined. This must be designed before Phase 1 coding begins. |
| **No versioning/migration strategy** | 🟡 High | How does the SQLite schema evolve across releases? Need migration system. |

### 7.2 What Could Go Wrong

| Risk | Likelihood | Impact | Mitigation |
|:---|:---|:---|:---|
| **Tool execution security** | High | Critical | Default to "ask before executing" mode. Deny-list dangerous commands. Never auto-approve in hosted mode. |
| **SQLite concurrent writes** | Medium | High | Use WAL mode. Consider a write-ahead queue. Test with 5+ concurrent users. |
| **Streaming reliability** | Medium | High | Implement heartbeat pings. Auto-reconnect on SSE drop. Buffer partial tokens. |
| **Context compaction quality** | High | Medium | Bad summaries degrade conversation quality. Allow user to undo compaction. Test extensively. |
| **Frontend bundle size** | Low | Medium | React + Tailwind + syntax highlighter + KaTeX can bloat. Set a budget: < 500KB gzipped. |
| **Provider API changes** | Medium | Medium | Isolate provider code. Good integration tests. Pin API versions. |
| **Scope creep into RAG** | High | High | RAG (document embedding, vector search) is a separate product. Explicitly defer it. Do not let it leak into the core. |
| **Trying to compete with Open WebUI on features** | High | High | Don't. Compete on simplicity and developer experience. Every feature added is complexity. Be ruthlessly selective. |

### 7.3 Biggest Unknowns

1. **Performance of embedded SQLite under concurrent load** — How many simultaneous conversations before it becomes a bottleneck? (Mitigate: benchmark early, Phase 1.)
2. **Token counting accuracy across providers** — Each provider counts tokens differently. Inaccurate counts break compaction. (Mitigate: use provider-specific tokenizers or accept ±10% margin.)
3. **Tool execution UX** — How to make the Pause-Execute-Resume loop feel seamless? A 10-second tool pause feels like a hang. (Mitigate: rich progress indicators, streaming tool output.)
4. **Upgrade path** — How does a user upgrade Forge without losing conversations? (Mitigate: SQLite migrations, backup command, version-tagged schema.)
5. **Community adoption** — Single-person projects rarely sustain open-source momentum. (Mitigate: focus on a small, polished MVP. Get 100 happy users before adding features.)

---

## 8. Prioritized Feature Backlog

### Tier 1: MVP (Must ship in v0.1)
| # | Feature | Effort | User Value |
|:---|:---|:---|:---|
| 1 | Streaming chat with Ollama provider | Medium | Critical |
| 2 | Conversation list (create, rename, delete) | Medium | Critical |
| 3 | SQLite persistence | Medium | Critical |
| 4 | Model selector dropdown | Small | Critical |
| 5 | Markdown + code block rendering | Medium | Critical |
| 6 | Dark/light mode toggle | Small | High |
| 7 | Settings page (provider config, API keys) | Medium | Critical |
| 8 | Message copy button | Small | High |
| 9 | Stop generation button | Small | Critical |
| 10 | OpenAI provider | Medium | High |
| 11 | Anthropic provider | Medium | High |
| 12 | Auto-detect Ollama on startup | Small | High |
| 13 | Welcoming first-run experience | Small | High |
| 14 | Message schema as tree (for future branching) | Small | Low (infra) |

### Tier 2: Core (v0.2 — makes it competitive)
| # | Feature | Effort | User Value |
|:---|:---|:---|:---|
| 15 | Message editing & resubmission | Medium | High |
| 16 | Response regeneration (with carousel) | Medium | High |
| 17 | Google Gemini provider | Medium | High |
| 18 | OpenAI-compatible generic provider | Small | High |
| 19 | System prompt per conversation | Small | High |
| 20 | Token count display | Small | Medium |
| 21 | Keyboard shortcuts | Small | Medium |
| 22 | Image paste/upload (vision models) | Medium | Medium |
| 23 | Thinking/reasoning block display | Medium | Medium |
| 24 | Conversation export (Markdown, JSON) | Small | Medium |
| 25 | Context compaction engine | Large | High |
| 26 | Conversation search | Medium | Medium |

### Tier 3: Differentiators (v0.3 — makes it special)
| # | Feature | Effort | User Value |
|:---|:---|:---|:---|
| 27 | Tool Manifest System + execution | Large | High |
| 28 | Inspector UI (context visualizer) | Large | High |
| 29 | Tool progress cards in chat | Medium | Medium |
| 30 | Event bus (WebSocket) | Medium | Medium (infra) |
| 31 | Conversation branching UI | Large | Medium |
| 32 | Prompt templates / library | Medium | Medium |

### Tier 4: Scale (v0.4+ — production readiness)
| # | Feature | Effort | User Value |
|:---|:---|:---|:---|
| 33 | API key authentication | Small | High (hosting) |
| 34 | PostgreSQL adapter | Medium | Medium (hosting) |
| 35 | Docker image + compose | Small | High (hosting) |
| 36 | Multi-user support | Large | Medium |
| 37 | Artifacts / canvas mode | Large | Medium |
| 38 | OpenRouter provider | Small | Medium |
| 39 | Cost tracking per conversation | Medium | Low |
| 40 | Plugin/tool registry | Large | Low |

---

## 9. Final Recommendations

### Do These Immediately
1. **Define the database schema** before writing any code. The message tree structure, conversation model, settings storage, and provider config must be designed upfront. A bad schema is the #1 cause of painful rewrites.
2. **Reorder the phases** to deliver a usable chat UI in Phase 2, not Phase 3. Users need to see value immediately.
3. **Add Google Gemini** to the MVP provider list. It's 2025 — a multi-model chat app that doesn't support Gemini feels incomplete.
4. **Write the first-run onboarding spec.** The "download → run → chatting in 60 seconds" experience is the product's core promise.

### Don't Do These (Yet)
1. **RAG / document embedding** — This is a separate product. Don't let it creep in.
2. **Desktop app** — The browser is the desktop app.
3. **Multi-user / teams** — Solve for one user first.
4. **Plugin marketplace** — Tool manifests in a directory are sufficient for v0.x.
5. **Voice input / TTS** — Cool but not core. Phase 9+.

### Design Principles to Codify
1. **Zero Config Start** — `./forge` must work with zero flags, zero env vars, zero config files if Ollama is running locally.
2. **No Telemetry, Ever** — Make this an explicit promise. Self-hosters choose Forge because they don't trust clouds.
3. **Progressive Disclosure** — Simple by default, powerful when needed. The settings page is there for those who want it; the defaults work for everyone else.
4. **Inspector as Superpower** — The Inspector UI should be treated as a first-class citizen, not an afterthought. It's the feature no competitor has, and it's what will get Forge featured on Hacker News.
5. **Tools as the Only Extension Point** — One extension system, not three. Tools are plugins. Plugins are tools.

---

> **Bottom line:** Forge has a strong architectural vision and a genuine differentiator (single binary + Inspector). But the implementation plan needs to shift from "engine-first" to "user-first." Ship something people can *use* (chat + conversations + model switching) before shipping something clever (tool execution + context compaction). The technical plumbing is means, not ends.

---

*Review complete. Next step: Update the implementation plan to reflect these recommendations, starting with the database schema design and phase reordering.*
