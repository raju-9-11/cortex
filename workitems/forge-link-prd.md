# forge-link — Product Requirements Document

**Status:** Draft v1.0
**Author:** Product Management
**Date:** 2025-07-15
**Stakeholders:** Engineering, Design, Open Source Community

---

## Table of Contents

1. [Vision & Positioning](#1-vision--positioning)
2. [User Stories & Personas](#2-user-stories--personas)
3. [Feature Scope (MVP vs. V2)](#3-feature-scope-mvp-vs-v2)
4. [Key Product Decisions](#4-key-product-decisions)
5. [Risks & Concerns](#5-risks--concerns)
6. [Success Metrics](#6-success-metrics)
7. [Appendix: Forge Backend Capabilities](#appendix-forge-backend-capabilities)

---

## 1. Vision & Positioning

### 1.1 The Problem

Developers today have fragmented options for AI-assisted terminal workflows:

| Tool | Lock-in | Runtime | Privacy | Session Persistence | Self-Hosted |
|------|---------|---------|---------|---------------------|-------------|
| **Claude Code** | Anthropic only | Node.js | Cloud only | No cross-tool sessions | ❌ |
| **GitHub Copilot CLI** | GitHub/OpenAI only | Node.js | Cloud only | Stateless | ❌ |
| **Aider** | Multi-provider | Python 3.9+ | Cloud only (no Ollama agentic) | Git-based | ❌ |
| **Cursor/Continue** | IDE-bound | Electron/Node | Cloud defaults | Editor sessions | ❌ |
| **Ollama CLI** | Ollama only | Go | Local only | Stateless | ✅ |

**The gap:** No tool gives you a rich terminal AI experience that is simultaneously:
- **Provider-agnostic** (use Ollama locally OR OpenAI/Qwen/Llama cloud, switch mid-session)
- **Self-contained** (single static binary, no Python/Node/Docker runtime)
- **Session-persistent** (conversations survive terminal closes, are resumable, searchable)
- **Privacy-respecting** (can run 100% local with zero data leaving your machine)

### 1.2 What forge-link Is

**forge-link** is a rich terminal client for the forge AI backend. It's a single Go binary that provides an interactive, session-aware AI assistant in your terminal — with support for any LLM provider forge can reach.

**One-liner:** _"Your terminal's AI brain — any model, fully local or cloud, conversations that persist."_

### 1.3 What Makes forge-link Different

| Differentiator | Why It Matters |
|----------------|----------------|
| **Zero-dependency binary** | `curl -L | tar xz` and you're running. No `pip install`, no `npm`, no Docker. Works on air-gapped machines. |
| **Provider-agnostic from day one** | Switch from `ollama/llama3.2` to `openai/gpt-4o` mid-conversation. No config file changes, no restarts. |
| **Persistent, resumable sessions** | Close your terminal, come back tomorrow, type `forge-link --resume` and pick up exactly where you left off — with full message history. |
| **Privacy spectrum** | Run 100% local with Ollama (zero network calls) or use cloud APIs — your choice, per-session. |
| **Backend-as-a-service architecture** | forge-link talks to forge's API — meaning your sessions are also accessible from a future web UI, mobile app, or other clients. Shared state, not siloed. |
| **Built on an OpenAI-compatible API** | forge's backend speaks the OpenAI protocol. Any tooling that works with OpenAI works with forge. |

### 1.4 What forge-link Is NOT

- **Not an IDE.** It's a terminal tool. No LSP, no file tree, no editor tabs.
- **Not a Claude Code clone.** We won't ship agentic file-editing or shell execution in MVP. That's V2+.
- **Not a wrapper around `ollama run`.** It adds sessions, multi-provider routing, rich rendering, and a persistent conversation layer.

### 1.5 Target Users

**Primary:** Developers who spend significant time in the terminal and want an AI assistant that fits their workflow without leaving it.

**Secondary:** DevOps/SRE engineers, data scientists working on remote servers, and privacy-conscious developers who want local-first AI.

**Tertiary:** Teams running a shared forge backend who want a standardized CLI experience across the org.

### 1.6 Competitive Positioning Map

```
                    Provider-Locked ◄──────────────────► Provider-Agnostic
                         │                                      │
               ┌─────────┤                                      │
    Stateless  │ Copilot  │         Ollama CLI                   │
               │  CLI     │                                      │
               └─────────┤                                      │
                         │                                      │
                         │              Aider                    │
                         │                                ┌─────┤
    Persistent │ Claude   │                               │forge-│
    Sessions   │ Code     │                               │ link │
               │         │                               └─────┤
               └─────────┤                                      │
                         │                                      │
                    Cloud Only ◄───────────────────► Local-First
```

forge-link occupies the **provider-agnostic + persistent + local-first** quadrant, which is currently empty.

---

## 2. User Stories & Personas

### 2.1 Personas

#### Persona 1: **Dev — "Terminal Tara"**
- **Role:** Full-stack developer, 5 years experience
- **Environment:** macOS, iTerm2, VS Code (but prefers terminal for quick tasks)
- **Pain:** Uses `ollama run` for quick questions but loses all conversation history. Switches to ChatGPT web when she needs to reference a prior conversation. Frustrated by context switching.
- **Goal:** An AI assistant that lives in her terminal, remembers conversations, and works with her local Ollama models AND her company's OpenAI key.

#### Persona 2: **Ops — "Server Sam"**
- **Role:** SRE / DevOps engineer
- **Environment:** SSH into production boxes, minimal tooling, can't install Python/Node
- **Pain:** Wants AI help debugging logs and writing scripts on remote servers. Claude Code requires Node.js. Aider requires Python. Neither installs cleanly on hardened prod boxes.
- **Goal:** A single binary he can `scp` to any server and immediately use. Must work with local Ollama or a remote forge instance.

#### Persona 3: **Privacy — "Paranoid Priya"**
- **Role:** Security engineer at a defense contractor
- **Environment:** Air-gapped or restricted network, strict data classification
- **Pain:** Cannot use any cloud AI tool. Ollama CLI is stateless and renders markdown poorly. Needs an AI workflow that never phones home.
- **Goal:** Rich AI terminal experience, fully local, with session history she can audit and delete.

#### Persona 4: **Team — "Lead Leo"**
- **Role:** Engineering team lead, manages 8 developers
- **Environment:** Shared development infrastructure, internal forge server
- **Pain:** Each developer uses a different AI tool. No shared context. When someone solves a tricky debugging problem with AI, that knowledge is lost.
- **Goal:** A team-wide CLI that connects to a centralized forge server. Eventually: shared sessions, auditability, cost tracking.

### 2.2 User Stories

#### P0 — Must Have (MVP Blockers)

| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-001** | **As a** developer, **I want to** start a new conversation from my terminal **so that** I can ask an AI questions without leaving my workflow. | **Given** forge-link is installed and a forge backend is reachable, **When** I run `forge-link`, **Then** I see a prompt and can type a message and receive a streamed AI response within 2 seconds of the first token. |
| **US-002** | **As a** developer, **I want to** have a multi-turn conversation **so that** I can iterate on a problem with full context. | **Given** I'm in an active session, **When** I send a follow-up message, **Then** the AI response incorporates all previous messages in the session as context. |
| **US-003** | **As a** developer, **I want to** see streaming responses with proper markdown rendering **so that** code blocks, bold text, and lists are readable. | **Given** the AI is generating a response, **When** tokens arrive, **Then** they render in real-time with ANSI-formatted code blocks (syntax-highlighted), bold, italic, inline code, headers, and bullet lists. |
| **US-004** | **As a** developer, **I want to** resume a previous conversation **so that** I don't lose context between terminal sessions. | **Given** I had a conversation yesterday, **When** I run `forge-link --resume` or `/load <session-id>`, **Then** I see the last N messages and can continue the conversation. |
| **US-005** | **As a** developer, **I want to** switch models mid-conversation **so that** I can use the right model for the right task. | **Given** I'm in a session using `ollama/llama3.2`, **When** I type `/model openai/gpt-4o`, **Then** subsequent messages use GPT-4o while retaining full conversation history. |
| **US-006** | **As a** developer, **I want to** list and manage my sessions **so that** I can organize my conversation history. | **Given** I have multiple sessions, **When** I type `/sessions`, **Then** I see a list of recent sessions with titles, models, message counts, and timestamps. I can `/load`, `/delete`, or `/rename` them. |

#### P1 — Should Have (Competitive Parity)

| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-007** | **As a** developer, **I want to** pipe input to forge-link **so that** I can use it in shell pipelines. | **Given** I run `cat error.log \| forge-link "explain this error"`, **When** stdin is not a TTY, **Then** forge-link reads stdin as context, sends the prompt, prints the response to stdout (no TUI chrome), and exits. |
| **US-008** | **As a** developer, **I want to** configure which forge server to connect to **so that** I can switch between local and remote backends. | **Given** I set `FORGE_URL=https://forge.internal.co`, **When** I start forge-link, **Then** it connects to the remote forge server using my `FORGE_API_KEY`. |
| **US-009** | **As a** developer, **I want to** see which model and session I'm using at all times **so that** I have confidence in what I'm doing. | **Given** I'm in a session, **When** I look at the prompt area, **Then** I see the active model name, session title, and message count in a status bar or prompt decoration. |
| **US-010** | **As a** developer, **I want to** set a system prompt **so that** I can customize the AI's behavior per session. | **Given** I start a session, **When** I run `forge-link --system "You are a Go expert"` or `/system <prompt>`, **Then** that system prompt is stored with the session and used for all subsequent messages. |
| **US-011** | **As an** ops engineer, **I want to** run a one-shot prompt without entering interactive mode **so that** I can script AI calls. | **Given** I run `forge-link -p "Write a bash script to rotate logs"`, **When** the response completes, **Then** the output is printed to stdout and the process exits with code 0. Session is optionally persisted. |

#### P2 — Nice to Have (Delight)

| ID | Story | Acceptance Criteria |
|----|-------|---------------------|
| **US-012** | **As a** developer, **I want to** copy the last response to my clipboard **so that** I can quickly paste code into my editor. | **Given** the AI just responded with a code block, **When** I type `/copy` or press a keybinding, **Then** the last assistant response (or code block) is copied to the system clipboard. |
| **US-013** | **As a** developer, **I want to** search across my conversation history **so that** I can find a solution I discussed previously. | **Given** I've had many sessions, **When** I type `/search "kubernetes pod restart"`, **Then** I see matching messages across all sessions with session IDs and timestamps. |
| **US-014** | **As a** developer, **I want to** see token usage and cost estimates **so that** I can be mindful of API costs. | **Given** I'm using a cloud provider, **When** a response completes, **Then** I see token count (prompt + completion) and optionally estimated cost. |

---

## 3. Feature Scope (MVP vs. V2)

### 3.1 MVP (v0.1.0) — "Useful Day One"

**Goal:** Replace `ollama run` and basic ChatGPT usage for terminal-native developers. Must feel polished, not alpha.

| Category | Feature | Rationale |
|----------|---------|-----------|
| **Core Chat** | Multi-turn streaming conversation | Table stakes. Without this, there's no product. |
| **Core Chat** | Rich markdown rendering (code blocks with language labels, bold, italic, headers, lists, inline code) | The #1 visual differentiator vs. `ollama run`. Must look beautiful. |
| **Core Chat** | Multi-line input (double-Enter to send, Shift+Enter for newline, or configurable) | Developers paste code snippets. Single-line input is a dealbreaker. |
| **Sessions** | Create / resume / list / delete / rename sessions | The #1 functional differentiator. Persistence is the killer feature. |
| **Sessions** | Auto-resume last session on launch (configurable) | Reduces friction. `forge-link` with no args should "just work." |
| **Models** | List available models (`/models`) | Users need to know what's available. |
| **Models** | Switch model mid-session (`/model <name>`) | Provider-agnostic is the value prop; switching must be effortless. |
| **Config** | Environment-based config (`FORGE_URL`, `FORGE_API_KEY`, `FORGE_MODEL`) | Must connect to local or remote forge with zero config files for quick start. |
| **Config** | Config file support (`~/.config/forge-link/config.yaml` or `forge-link.yaml`) | Power users need persistent config. |
| **UX** | Status bar showing model, session, connection status | Users must always know what's happening. |
| **UX** | Slash commands (`/help`, `/exit`, `/new`, `/sessions`, `/load`, `/model`, `/system`, `/clear`) | Proven UX pattern from Discord, Slack, existing `forge chat`. |
| **UX** | Input history (up/down arrows recall previous messages) | Standard terminal UX. Absence is jarring. |
| **UX** | Graceful error handling (connection lost, model not found, auth failure) | Errors must be clear and actionable, not stack traces. |
| **Pipe Mode** | Non-interactive stdin/stdout mode for scripting | Enables `cat file | forge-link "explain this"` and shell integration. |
| **One-Shot** | `forge-link -p "prompt"` exits after response | Scriptability. |

### 3.2 V2 (v0.2.0) — "Power User"

| Category | Feature | Rationale |
|----------|---------|-----------|
| **Context** | File injection: `/file path/to/code.go` attaches file content as context | Core developer workflow — "explain this code" — without manual copy-paste. |
| **Context** | Project awareness: auto-detect `.git` root, optionally include `README.md` or file tree as context | Competitive with Aider's codebase awareness. |
| **Agentic** | Read-only file operations: the AI can request to read files (with user approval) | Step 1 toward agentic — safe, read-only. |
| **Sessions** | Session search across all conversations | US-013 deferred from MVP. |
| **Sessions** | Session export (Markdown, JSON) | Share conversations, paste into docs. |
| **UX** | Keyboard shortcuts (`Ctrl+C` cancel generation, `Ctrl+L` clear, `Ctrl+R` search history) | Polish. |
| **UX** | Syntax highlighting in code blocks (tree-sitter or similar) | Visual quality — competitors do this. |
| **UX** | Token usage display per response | US-014 deferred from MVP. |
| **UX** | `/copy` to clipboard | US-012 deferred from MVP. |
| **UX** | Theme support (dark/light, custom colors) | Accessibility and personalization. |
| **Provider** | Connection health indicator (reconnect on failure) | Resilience for remote forge servers. |

### 3.3 V3+ — "Agentic"

| Category | Feature | Rationale |
|----------|---------|-----------|
| **Agentic** | File write operations (with diff preview and user approval) | Competitive with Claude Code / Aider. Requires forge FB-006 (Tool Sandbox). |
| **Agentic** | Shell command execution (sandboxed, approval required) | High risk, high reward. Requires careful security model. |
| **Agentic** | Multi-step tool chains (AI plans → executes → reports) | Full agentic loop. Depends on forge's tool calling maturity. |
| **Team** | Multi-user sessions (shared forge server) | Depends on forge WI-205 (user identity). |
| **Team** | Session sharing and collaboration | Requires auth + RBAC on forge side. |

### 3.4 Anti-Scope (Explicitly NOT Doing)

| Feature | Reason |
|---------|--------|
| **LSP integration** | We're a terminal tool, not an IDE. Use Cursor/Continue for that. |
| **Git integration (auto-commit, diff generation)** | Aider does this well. We'd be a worse version. Stay focused. |
| **Web UI** | forge itself will have a web UI (FB-001). forge-link is terminal-only. |
| **Model download/management** | That's Ollama's job. We consume models, not manage them. |
| **Fine-tuning or training** | Out of scope entirely. We're an inference client. |
| **Plugin/extension system** | Premature. Build the right features first, extract extension points later. |
| **Voice input/output** | Cool but niche. Not MVP, not V2, maybe never. |

---

## 4. Key Product Decisions

### 4.1 Architecture: Client vs. Embedded

**Decision: forge-link MUST support both embedded mode (default) AND remote client mode.**

| Mode | How It Works | When to Use |
|------|-------------|-------------|
| **Embedded (default)** | forge-link imports forge's Go packages directly. No HTTP server needed. Starts SQLite, registers providers, calls inference — all in-process. | Single-user local usage. Zero config. Just `forge-link` and go. |
| **Remote client** | forge-link connects to a running forge HTTP server via `FORGE_URL`. Uses REST + SSE. | Team usage, remote servers, when forge is already running for its web UI. |

**Rationale:**

- **Embedded-first removes the #1 adoption barrier.** If users have to run `forge` in one terminal and `forge-link` in another, we've already lost 60% of them. Claude Code doesn't require running a server. Neither should we.
- **Remote mode enables team use cases** and is trivially supported since forge already has a full HTTP API. It's a `net/http` client, not a new feature.
- **The existing `forge chat` code already uses embedded mode** — it calls `app.New()`, creates a session manager, and talks to providers directly. forge-link extends this pattern with a richer TUI.
- **Data integrity:** In embedded mode, forge-link owns the SQLite file. In remote mode, the server owns it. Never both simultaneously (the SQLite write lock prevents it, but we should warn clearly).

**Implementation note:** The core chat logic should be written against an interface that can be satisfied by either embedded direct calls or HTTP client calls. Something like:

```go
type ForgeBackend interface {
    CreateSession(ctx, params) (*Session, error)
    GetSession(ctx, id) (*Session, error)
    ListSessions(ctx) ([]SessionListItem, error)
    SendMessage(ctx, sessionID, content string) (<-chan StreamEvent, error)
    ListModels(ctx) ([]ModelInfo, error)
    // ...
}

// Embedded: calls session.Manager + inference.Registry directly
// Remote: calls forge HTTP API via net/http
```

**⚠️ Open question:** Should embedded mode also start the HTTP server so the web UI can connect? **My recommendation: No.** Keep modes separate. If you want both, run `forge` (server) and `forge-link --url localhost:8080` (client). Hybrid mode adds complexity for minimal value.

### 4.2 Agentic Capabilities: Chat-First, Agent-Later

**Decision: MVP is chat-only. No file read/write, no shell execution.**

**Rationale:**
- forge's Tool Sandbox (FB-006) doesn't exist yet. Building agentic features in the client before the backend supports tool execution creates a fragile, client-side implementation that will be rewritten.
- Chat-only is a clear, testable, shippable product. Agentic is a 3-month feature with security implications.
- We can still be *useful* without agentic — persistent sessions + multi-provider + rich rendering is a strong value prop that `ollama run` and basic ChatGPT can't match.
- **Exception:** The `/file` command (V2) is read-only context injection, not agentic. It's a client-side `ioutil.ReadFile()` that prepends file contents to the user message. Low risk, high value. But it's V2, not MVP.

### 4.3 Project Context / Codebase Awareness

**Decision: Not in MVP. V2 feature with opt-in design.**

When implemented (V2):
- `/file <path>` manually injects a file's contents into the conversation context
- `/tree` shows project file tree and optionally injects it as context
- Auto-detection of `.git` root to establish "project boundary"
- **No automatic indexing or embedding.** Too complex, too slow, too surprising. Users should explicitly choose what context to provide.

**Rationale against auto-context:**
- Automatic codebase indexing (like Cursor does) requires an embedding model, vector store, and retrieval pipeline. That's an entire product, not a feature.
- Terminal users value predictability. "Why did the AI suddenly know about my secrets.env?" is a trust-breaking moment.
- Explicit `/file` injection is simple, predictable, and covers 80% of use cases.

### 4.4 Session Persistence Across Invocations

**Decision: Yes, always. Sessions persist in SQLite and survive process exit.**

- On launch with no args: resume the most recent active session (configurable: can default to new session instead)
- On launch with `--session <id>`: resume that specific session
- On launch with `--new`: always create a new session
- Sessions are stored in `~/.local/share/forge-link/forge-link.db` (embedded mode) or on the forge server (remote mode)

**This is the killer feature.** No competitor does persistent, resumable, searchable sessions well in the terminal.

### 4.5 Input Paradigm: Slash Commands + Keybindings

**Decision: Slash commands as primary, keybindings as accelerators.**

**Slash commands (MVP):**

| Command | Action | Notes |
|---------|--------|-------|
| `/help` | Show all commands | |
| `/exit` or `/quit` | Exit forge-link | |
| `/new [title]` | Create new session | Optional title |
| `/sessions` | List recent sessions | |
| `/load <id>` | Switch to session | Tab-completion on ID |
| `/delete <id>` | Delete session | Confirmation prompt |
| `/rename <title>` | Rename current session | |
| `/model [name]` | Switch model (or show current) | |
| `/models` | List available models | |
| `/system [prompt]` | Set/show system prompt | |
| `/clear` | Clear terminal (keep session) | |
| `/history` | Show session message history | |

**Keybindings (MVP):**

| Key | Action |
|-----|--------|
| `Enter Enter` (double) | Send message |
| `Ctrl+C` | Cancel current generation / exit if idle |
| `Ctrl+D` | Send message (alternative to double-Enter) |
| `Ctrl+L` | Clear screen |
| `Up/Down` | Scroll input history |
| `Esc` | Cancel current input |

**Rationale:**
- Slash commands are discoverable (`/help`). Keybindings are not.
- Slash commands work over SSH where keybindings may be intercepted.
- `/` prefix is universally understood (Discord, Slack, IRC, existing `forge chat`).
- Double-Enter to send is already established in the existing `forge chat` REPL — maintain continuity.

### 4.6 Streaming Response UX

**Decision: Character-by-character streaming with progressive markdown rendering.**

The streaming experience is the most important UX detail. It must feel *fast* and *polished*.

**Requirements:**
1. **First token latency < 200ms** (after network). The prompt should "react" immediately — show a spinner or blinking cursor while waiting for the first token.
2. **Progressive rendering:** Markdown formatting is applied as tokens arrive, not after completion. Code blocks open with a language header, content streams in, and the closing fence completes it.
3. **No flicker/reflow:** Once a line is rendered, it should not visually change. This means formatting decisions must be conservative — wait to confirm `**` is bold before rendering, but don't wait so long that there's visible buffering delay.
4. **Cancellation:** `Ctrl+C` during streaming stops generation immediately. Partial response is kept and saved to session. User can continue the conversation.
5. **Completion signal:** When streaming finishes, show a subtle separator (thin line or timestamp) and return to the input prompt cleanly.
6. **Thinking indicator:** Before first token arrives, show a pulsing `⠋ Thinking...` spinner. Replace it in-place when tokens start flowing.

**Technical note:** The existing `internal/cli/stream.go` handles basic bold/code rendering with a state machine. forge-link must significantly expand this — adding headers, lists, links, and ideally syntax highlighting (V2). The current approach of raw ANSI codes is correct for streaming; a library like `glamour` (full-document renderer) won't work for progressive rendering.

### 4.7 Configuration Hierarchy

**Decision: Environment variables > config file > flags > interactive defaults.**

```
Priority (highest to lowest):
1. CLI flags:        forge-link --model gpt-4o --url http://localhost:8080
2. Environment vars: FORGE_URL, FORGE_API_KEY, FORGE_MODEL, FORGE_LINK_THEME
3. Config file:      ~/.config/forge-link/config.yaml (or $XDG_CONFIG_HOME)
4. Defaults:         embedded mode, auto-detect Ollama, new session
```

**Config file format (YAML):**
```yaml
# ~/.config/forge-link/config.yaml
url: ""                          # Empty = embedded mode
api_key: ""                      # For remote forge server
model: "ollama/llama3.2"         # Default model
system_prompt: ""                # Default system prompt
theme: "auto"                    # auto | dark | light
auto_resume: true                # Resume last session on launch
send_key: "double-enter"         # double-enter | ctrl-d
history_display: 10              # Messages to show on session resume
data_dir: "~/.local/share/forge-link"  # SQLite location (embedded mode)
```

---

## 5. Risks & Concerns

### 5.1 Technical Risks

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| **TUI streaming complexity** | High | High | The existing `stream.go` state machine handles only bold/code. Adding headers, lists, nested formatting while streaming character-by-character is genuinely hard. Budget 40% of TUI effort here. Consider using `goldmark` for post-completion re-render. |
| **Embedded mode SQLite locking** | Medium | Medium | If a user runs `forge` (server) and `forge-link` (embedded) pointing at the same `.db` file, the write lock will cause failures. Detect this (check for lock file) and warn clearly. Use separate default paths. |
| **Bubble Tea vs. raw ANSI** | High | High | **This is the hardest architectural decision.** Bubble Tea gives us a proper TUI (status bar, scrolling, panels) but conflicts with long-running streaming. Raw ANSI codes are simple but can't do split-pane, scroll-back, or resize handling. **Recommendation:** Use Bubble Tea with a custom viewport that handles streaming — this is non-trivial but achievable (see `charm/glow` for precedent). If this proves too complex, fall back to enhanced raw-mode with `lipgloss` for styling only. |
| **Cross-platform terminal compatibility** | Medium | Medium | ANSI codes behave differently on Windows (cmd.exe, PowerShell, Windows Terminal). Use `muesli/termenv` for detection and adapt. |
| **forge backend API instability** | Low | Medium | forge has 14 open P0/P1 work items. Some (WI-010: missing SSE fields, WI-004: streaming context lifecycle) directly affect forge-link. Coordinate: forge-link MVP should launch *after* forge sprint-1 is complete. |

### 5.2 Product Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| **"Why not just use Claude Code?"** | Critical | Claude Code is locked to Anthropic and requires a subscription. forge-link works with *any* model including free local ones. Lead with the "bring your own model" message. But acknowledge: if the UX isn't at least 70% as polished as Claude Code, users won't switch. **The bar is high.** |
| **"Why not just use the web UI?"** | High | Because the web UI doesn't exist yet (FB-001). And when it does — terminal users want to stay in the terminal. But if the web UI ships first and is great, forge-link becomes less urgent. **Recommendation:** Ship forge-link first as the flagship client experience. |
| **Scope creep into agentic** | High | Every user will ask "can it edit files?" on day one. Resist until V3. The agentic surface area is massive (security, undo, sandboxing, approval flows). Ship a great chat client first. |
| **"I already have my own AI workflow"** | Medium | The switching cost from an existing tool is real. Focus on the *zero-to-one* user (has no AI CLI yet) rather than trying to convert Claude Code power users. |

### 5.3 What Would Make Users NOT Adopt This

1. **Slow first-token latency.** If there's a perceptible delay between pressing Enter and seeing the thinking spinner, the tool feels broken. Target <100ms for UI response, even if the model takes seconds.
2. **Ugly output.** If code blocks look worse than `ollama run`, we've failed. Rich rendering is the table stakes.
3. **Losing a conversation.** If a session is corrupted, lost, or un-resumable even once, trust is destroyed. Persistence must be bulletproof.
4. **Difficult installation.** If it takes more than `brew install forge-link` or `curl | tar`, adoption drops off a cliff.
5. **Can't connect to their model.** If someone has an Ollama setup and forge-link can't auto-detect it, they'll uninstall within 60 seconds.
6. **Input jankiness.** Multi-line input, paste handling, and Unicode support must work flawlessly. A broken paste experience (mangled characters, missing lines) is unforgivable in 2025.

### 5.4 Hardest Product Decisions

1. **Bubble Tea or not Bubble Tea?** A full TUI framework gives us status bars, scrolling, panels — but conflicts with streaming. Raw mode is simpler but uglier. This decision defines the product's ceiling. **My recommendation: Bubble Tea, with investment in a custom streaming viewport component.** The upfront cost is worth the long-term extensibility.

2. **When to go agentic?** Too early = security risk + half-baked UX. Too late = "this is just a fancy chat, why bother?" The V2/V3 boundary for file read/write is the right call, but it means MVP must be *so good* at chat that users stay without agentic features.

3. **Embedded vs. client as default?** Embedded is the right default for adoption, but it means forge-link ships with a significant chunk of forge inside it. Binary size will be 15-25MB instead of 5MB. Acceptable for the UX benefit.

---

## 6. Success Metrics

### 6.1 North Star Metric

**Weekly Active Sessions** — Number of unique sessions that had at least one message sent in the last 7 days. This measures both adoption (new users) and retention (returning users).

### 6.2 Quantitative Metrics

| Metric | Target (3 months post-launch) | How to Measure |
|--------|-------------------------------|----------------|
| **GitHub stars** | 500+ | GitHub API |
| **Homebrew installs** | 200+/month | Homebrew analytics |
| **Weekly active sessions** | 100+ (across all users) | Optional anonymous telemetry (opt-in) or local-only estimate |
| **Session resume rate** | >40% of sessions are resumed at least once | Local analytics |
| **Average session length** | >5 messages per session | Local analytics |
| **First-token latency (P95)** | <2 seconds (local Ollama), <4 seconds (cloud) | Client-side timing |
| **Crash rate** | <0.1% of sessions | Error reporting (opt-in) |
| **Multi-provider usage** | >20% of users use 2+ providers | Local analytics |

### 6.3 Qualitative Metrics

| Signal | What It Means |
|--------|---------------|
| Users requesting agentic features | Validated product-market fit — they want more |
| Users connecting to remote forge servers | Team use case is real |
| PRs from community | Open source engagement |
| "I switched from Claude Code" reports | Competitive displacement — strong signal |
| "I use it on remote servers" reports | Server Sam persona is validated |

### 6.4 Anti-Metrics (Things We Do NOT Optimize For)

| Anti-Metric | Why |
|-------------|-----|
| **Total messages sent** | Vanity metric. A user sending 1000 low-quality messages isn't success. |
| **Number of providers configured** | Config complexity ≠ value. One provider used well > five configured and ignored. |
| **Binary download count** | Downloads without retention = hype, not product-market fit. |

---

## Appendix: Forge Backend Capabilities (Current State)

### What forge-link Can Leverage Today

| Capability | forge Status | forge-link Impact |
|------------|-------------|-------------------|
| OpenAI-compatible `/v1/chat/completions` | ✅ Working | Remote mode uses this directly |
| Session CRUD (`/api/sessions/*`) | ✅ Working | Full session management |
| Streaming SSE | ✅ Working (missing `id`/`created` per WI-010) | Core streaming UX |
| Multi-provider registry | ✅ Working | Model switching, provider listing |
| Ollama auto-detection | ✅ Working | Zero-config local mode |
| SQLite persistence | ✅ Working | Embedded mode session storage |
| Auth middleware | ✅ Working | Remote mode authentication |
| Health endpoint | ✅ Working | Connection status display |

### What forge-link Needs from forge (Dependencies)

| Dependency | forge Work Item | Blocking? |
|------------|----------------|-----------|
| SSE `id` and `created` fields | WI-010 | No (embedded mode bypasses SSE) but Yes for remote mode |
| Streaming context lifecycle fix | WI-004 | Yes — partial responses lost on disconnect |
| Message count race condition | WI-005 | Low — cosmetic inaccuracy |
| Request body size limits | WI-001 | No — forge-link controls its own input |
| Context window truncation | WI-007 | Yes for long conversations — without this, sessions will fail after ~20 messages with small models |

### Recommended forge Sprint-1 Items to Complete Before forge-link MVP

1. **WI-004** (Critical) — Fix streaming context lifecycle. Partial messages must be saved.
2. **WI-007** (High) — Context window truncation. Without this, long sessions break.
3. **WI-010** (High) — SSE `id`/`created` fields. Required for remote mode.
4. **WI-005** (Medium) — Message count atomicity. Cosmetic but erodes trust.

---

## Next Steps

1. **Architecture Review** — Validate embedded+remote dual-mode architecture with engineering.
2. **TUI Spike** — 2-day spike to prototype streaming in Bubble Tea. If it works, commit. If not, fall back to enhanced raw mode with lipgloss.
3. **Scope Lock** — Freeze MVP scope per Section 3.1. Any additions require explicit trade-offs.
4. **forge Backend Coordination** — Ensure WI-004, WI-007, WI-010 are completed before forge-link beta.
5. **Design Sprint** — Define exact visual layout, color scheme, and streaming animations.
