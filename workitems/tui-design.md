# forge-link — TUI Design Specification

> **Author:** UI/UX Design Architect  
> **Date:** 2025-07-15  
> **Status:** DESIGN — ready for implementation review  
> **Scope:** Complete TUI design for the `forge` interactive terminal client  
> **Backend:** Existing `internal/cli/` package (REPL, streaming, sessions, models)

---

## 0. Design Philosophy

### What we're building
A **conversational AI terminal client** that feels as natural as talking to a
colleague. Not a dashboard. Not an IDE. A conversation, augmented with
structured tool output when needed.

### Core principles (in priority order)

1. **Conversation first.** The chat transcript is always the hero. Everything
   else is subordinate.
2. **Progressive disclosure.** Show the minimum by default; reveal detail on
   demand. A spinner is better than a wall of JSON.
3. **Terminal-native.** Respect the terminal. No mouse required. Work over SSH,
   inside tmux, on 80×24. Degrade gracefully.
4. **Speed perception.** The first token matters more than the last. Show
   thinking indicators instantly, stream tokens the moment they arrive.
5. **Reversible actions.** Dangerous operations (file writes, command execution)
   require explicit confirmation. Everything else is instant.

### What we're NOT building
- A full-screen IDE (use your editor)
- A dashboard with multiple panels fighting for attention
- A mouse-driven GUI that happens to render in a terminal

---

## 1. Layout & Information Architecture

### 1.1 The Decision: Inline REPL (not full-screen)

**Recommendation: Inline scrolling REPL with a persistent status bar.**

Rationale:
- Claude Code, Aider, and GitHub Copilot CLI all use this pattern successfully
- Full-screen apps (like vim) create mode confusion — users expect their terminal
  to behave like a terminal
- Inline output preserves scrollback history, which users rely on for
  copy-paste and reference
- Works naturally in tmux panes, split terminals, and SSH sessions
- The existing `internal/cli/repl.go` already follows this pattern — we enhance
  rather than replace

Full-screen is reserved for exactly one scenario: reviewing multi-file diffs
(see §6.3).

### 1.2 Layout Option A — Minimal (Recommended for v1)

The "conversation flow" layout. This is what we ship first.

```
┌─────────────────────────────────────────────────────────────────────┐
│ 🔥 forge · claude-sonnet-4-20250514 · ses_01J… · 2.1k tokens     │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  You                                                                │
│  Explain the streaming architecture in this codebase                │
│                                                                     │
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │
│                                                                     │
│  Assistant · claude-sonnet-4-20250514 · 847 tokens · 3.2s                │
│                                                                     │
│  The streaming architecture follows a channel-based pipeline        │
│  pattern:                                                           │
│                                                                     │
│  1. **Provider goroutine** — Calls the upstream API and pushes      │
│     `StreamEvent` values into a buffered channel (cap 32)           │
│                                                                     │
│  2. **SSE pipeline** — Reads from the channel in an event loop,     │
│     formats each event as `data: {json}\n\n`, and writes to the     │
│     HTTP response writer                                            │
│                                                                     │
│  ```go                                                              │
│  │ events := make(chan types.StreamEvent, 32)                       │
│  │ go func() {                                                     │
│  │     errCh <- provider.StreamChat(ctx, req, events)              │
│  │ }()                                                             │
│  ```                                                                │
│                                                                     │
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │
│                                                                     │
│  forge [claude-sonnet-4-20250514]> █                                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

Status bar (bottom, persistent — only element using alternate screen buffer):
┌─────────────────────────────────────────────────────────────────────┐
│ ● connected  claude-sonnet-4-20250514  ses_01J…  2.1k/200k tokens  /help │
└─────────────────────────────────────────────────────────────────────┘
```

**Key elements:**
- **Status bar** (1 line, bottom): Connection state, model, session, token usage, hint
- **Prompt**: `forge [model]> ` — shows active model at a glance
- **Message blocks**: Clear visual separation between user/assistant turns
- **Metadata line**: Model name, token count, and latency on each assistant response

### 1.3 Layout Option B — With Tool Activity Panel

When tool use is active, a collapsible activity section appears inline:

```
│  Assistant · claude-sonnet-4-20250514                                        │
│                                                                     │
│  Let me read the streaming implementation...                        │
│                                                                     │
│  ┌─ Tool Use ──────────────────────────────────────────────────┐   │
│  │  ✓ read_file  internal/streaming/sse.go         0.3s  189L │   │
│  │  ✓ read_file  pkg/types/events.go               0.1s   33L │   │
│  │  ⟳ read_file  internal/inference/openai.go       ...        │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  The SSE pipeline in `sse.go` implements...                         │
```

**Design rules for tool panels:**
- Appears inline in the conversation flow (not a sidebar)
- Each tool call is one line: status icon + tool name + arguments + timing
- Collapsed by default after completion (show summary: "3 files read, 422 lines")
- Expandable with `Enter` or `/expand` to see full tool output
- During execution: animated spinner (⟳) on the active tool

### 1.4 Layout Option C — Split Pane (Deferred to v2)

For power users who want persistent context:

```
┌──────────────────────────────┬──────────────────────────────────────┐
│  Sessions                    │                                      │
│                              │  You                                 │
│  ● Current: Streaming arch   │  Explain the streaming architecture  │
│    ses_01J…7mK               │                                      │
│                              │  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ │
│    API refactoring           │                                      │
│    ses_01J…3pQ               │  Assistant · claude-sonnet-4-20250514          │
│                              │  The streaming architecture...       │
│    Bug investigation         │                                      │
│    ses_01J…9rT               │                                      │
│                              │                                      │
│                              │  forge [claude-sonnet-4-20250514]> █            │
├──────────────────────────────┴──────────────────────────────────────┤
│ ● connected  claude-sonnet-4-20250514  ses_01J…  2.1k/200k tokens           │
└─────────────────────────────────────────────────────────────────────┘
```

**Why defer:** Split panes require a full-screen TUI framework (bubbletea),
significantly increase complexity, and conflict with tmux workflows (users
already have their own splits). Ship Option A first, measure demand.

### 1.5 Information Density Guidelines

| Terminal Width | Behavior |
|:---|:---|
| < 60 cols | Warn on startup, compact mode (no decorative borders) |
| 60–80 cols | Standard layout, wrapped text, abbreviated status bar |
| 80–120 cols | Full layout with metadata, tool panels, code blocks |
| > 120 cols | Same as 80–120 (don't stretch — readability degrades past ~100 chars) |
| Height < 15 | Suppress status bar, minimal chrome |

**Text wrapping:** Wrap response text at `min(terminal_width - 4, 100)` columns.
Long lines in code blocks are NOT wrapped — they scroll horizontally or are
truncated with `…` and a "view full" hint.

---

## 2. Interaction Patterns

### 2.1 Input: Multi-line with Smart Send

**Current behavior** (`repl.go:309-344`): Blank line sends. This is correct.

**Enhanced behavior for forge-link:**

| Action | Result |
|:---|:---|
| Type text + `Enter` | New line in input buffer |
| `Enter` on blank line | Send message (double-Enter to send) |
| `Ctrl+D` on blank line | Send message (alternative) |
| `Ctrl+D` on empty prompt | Exit REPL |
| `Ctrl+C` during input | Clear current input buffer |
| `Ctrl+C` during streaming | Cancel current generation |
| `/command` + `Enter` | Execute command immediately (no double-Enter needed) |
| Paste multi-line text | Captured as single input block |

**Prompt rendering:**

```
forge [claude-sonnet-4-20250514]> Tell me about the
... streaming pipeline
... in this codebase
...
⏎ Sending (3 lines)
```

The `...` continuation prompt aligns with the main prompt. The `⏎ Sending`
confirmation appears briefly (200ms) on send to give tactile feedback.

### 2.2 Streaming Response Rendering

**Current behavior** (`stream.go`): Character-by-character with ANSI markdown
formatting. This is already well-implemented.

**Enhanced behavior:**

```
Phase 1: THINKING INDICATOR (0-2 seconds)
┌─────────────────────────────────────────┐
│  Assistant · claude-sonnet-4-20250514            │
│  ⠋ Thinking...                          │
└─────────────────────────────────────────┘

Phase 2: FIRST TOKEN ARRIVES
┌─────────────────────────────────────────┐
│  Assistant · claude-sonnet-4-20250514            │
│  The streaming█                         │
└─────────────────────────────────────────┘

Phase 3: STREAMING (tokens arrive)
┌─────────────────────────────────────────┐
│  Assistant · claude-sonnet-4-20250514            │
│  The streaming architecture follows     │
│  a channel-based pipeline pattern:      │
│                                         │
│  1. **Provider goroutine** — Calls█     │
└─────────────────────────────────────────┘

Phase 4: COMPLETE
┌─────────────────────────────────────────┐
│  Assistant · claude-sonnet-4-20250514 · 847tk · 3.2s │
│  The streaming architecture follows...  │
│  ...                                    │
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │
└─────────────────────────────────────────┘
```

**Key streaming rules:**
- Show spinner within **100ms** of sending message (before first token)
- Render tokens the instant they arrive from the channel — no batching
- The existing `processDelta()` in `stream.go` handles this well already
- After completion, append metadata line (token count, latency)
- Use `content.done` event's `Usage` field for accurate token counts
- Cursor block (█) shown at insertion point during streaming

### 2.3 During Long Responses

| Action | Behavior |
|:---|:---|
| Scroll up | Normal terminal scrollback (we don't fight it) |
| `Ctrl+C` | Cancel generation, keep partial output, print `[cancelled]` |
| `Ctrl+L` | Clear screen, reprint last response (if any) |
| `q` during streaming | Ignored (it's text, not a pager) |
| Terminal resize | Re-render status bar, text reflows naturally |

**The user can always scroll up** because we use inline output, not alternate
screen. This is a fundamental advantage over full-screen TUIs.

### 2.4 Slash Commands

Based on the existing commands in `repl.go` plus new ones:

```
Session Commands:
  /new                  Start a new session
  /sessions             List recent sessions (table format)
  /load <id>            Switch to an existing session
  /delete <id>          Delete a session
  /title <text>         Set current session title
  /export [file]        Export session as markdown

Model Commands:
  /model [name]         Show or switch model
  /models               List all available models
  /provider [name]      Show or filter by provider
  /system <prompt>      Set system prompt for this session

Display Commands:
  /clear                Clear screen (preserve history)
  /compact              Compact conversation (reduce tokens)
  /expand [n]           Show full output of last tool call(s)
  /history [n]          Show last n messages

Meta Commands:
  /help [command]       Show help (or help for specific command)
  /config               Show current configuration
  /status               Show connection status, token usage
  /exit, /quit          Exit (Ctrl+D also works)
```

**Autocomplete behavior:**

```
forge [claude-sonnet-4-20250514]> /mo█
                        /model    Switch to a different model
                        /models   List all available models
```

- Triggered on `/` followed by typing
- Tab completes the unambiguous prefix
- If ambiguous, show candidates inline (below prompt, not a popup)
- Fuzzy matching: `/mdl` matches `/model` (but exact prefix preferred)
- After command name, Tab-complete arguments contextually:
  - `/model ` → list available model names
  - `/load ` → list session IDs with titles
  - `/export ` → filesystem path completion

### 2.5 Tool Use Display

The backend already supports tool events (`tool.start`, `tool.progress`,
`tool.complete`, `tool.result`). Here's how to render them:

**During execution:**
```
  ┌─ Tools ───────────────────────────────────────────────────┐
  │  ✓ read_file    internal/streaming/sse.go      0.3s 189L  │
  │  ⟳ search_code  "StreamEvent" in pkg/types/     ...       │
  └───────────────────────────────────────────────────────────┘
```

**After completion (collapsed — default):**
```
  ┌─ Tools (3 calls, 0.8s) ──────────────────────────────────┐
  │  ✓ read_file ×2  ✓ search_code ×1                        │
  └───────────────────────────────────────────────────────────┘
```

**After `/expand` (expanded):**
```
  ┌─ read_file: internal/streaming/sse.go ───────────────────┐
  │   14 │ func NewPipeline(provider InferenceProvider) *Pip…│
  │   15 │     return &Pipeline{provider: provider}          │
  │   16 │ }                                                 │
  │      │ ... (174 more lines — press Enter for full)       │
  └──────────────────────────────────────────────────────────┘
```

**Status icons:**
- `⟳` (U+27F3) — In progress (animated via spinner cycle: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏)
- `✓` (U+2713) — Completed successfully (green)
- `✗` (U+2717) — Failed (red)
- `⏱` (U+23F1) — Timed out (yellow)

### 2.6 Permission Prompts

For destructive operations (file writes, command execution):

```
  ┌─ Permission Required ────────────────────────────────────┐
  │                                                           │
  │  The assistant wants to:                                  │
  │                                                           │
  │    write_file  internal/streaming/sse.go                  │
  │                                                           │
  │  ┌─ Changes ──────────────────────────────────────────┐  │
  │  │  @@ -14,3 +14,5 @@                                │  │
  │  │  - func NewPipeline(p InferenceProvider) *Pipeline {│  │
  │  │  + func NewPipeline(                               │  │
  │  │  +     p InferenceProvider,                        │  │
  │  │  +     opts ...PipelineOption,                     │  │
  │  │  + ) *Pipeline {                                   │  │
  │  └────────────────────────────────────────────────────┘  │
  │                                                           │
  │  [y]es  [n]o  [a]lways for this session  [v]iew full     │
  └───────────────────────────────────────────────────────────┘
```

**Permission modes:**
- **Ask** (default): Prompt for each destructive operation
- **Auto-allow reads**: File reads and searches proceed without asking
- **Trust session**: `/trust` command auto-approves for the remainder of session
- **Always ask for writes**: Even in trust mode, file writes show the diff first

**Implementation:** This is inline (not a modal). The prompt appears in the
conversation flow and blocks input until answered. Single keypress — no Enter
required.

---

## 3. Visual Design in Terminal

### 3.1 Color Palette

**Design constraint:** Must work on both dark and light terminal backgrounds.
Use ANSI 16-color base (not 256-color or truecolor) for maximum compatibility.
Enhanced colors available when `COLORTERM=truecolor` is detected.

**Base palette (ANSI 16):**

| Element | Dark terminal | Light terminal | ANSI code |
|:---|:---|:---|:---|
| User message label | White, bold | Black, bold | `\033[1m` |
| Assistant message label | Cyan, bold | Cyan, bold | `\033[1;36m` |
| Assistant body text | Default (white/black) | Default | (none) |
| Code blocks (fenced) | Dim | Dim | `\033[2m` |
| Inline code | Cyan | Cyan | `\033[36m` |
| Bold text | Bold | Bold | `\033[1m` |
| Tool call status | Yellow | Yellow | `\033[33m` |
| Success indicators | Green | Green | `\033[32m` |
| Error text | Red, bold | Red, bold | `\033[1;31m` |
| Dim/metadata | Dim | Dim | `\033[2m` |
| Status bar bg | Reverse video | Reverse video | `\033[7m` |
| Prompt text | Green, bold | Green, bold | `\033[1;32m` |
| Separator lines | Dim | Dim | `\033[2m` |

**Light/dark detection:** Check `COLORFGBG` env var (format: `fg;bg`).
If `bg > 8`, assume light background. Default to dark if undetectable.
Also respect `FORGE_THEME=light|dark|auto`.

**Color-blind safe:** The palette relies on **luminance contrast** (bold vs dim,
reverse video) rather than hue alone. Red/green are never the only
differentiator — they're always paired with icons (✓/✗) or text labels.

### 3.2 Typography (ANSI Formatting)

| Style | Usage | ANSI |
|:---|:---|:---|
| **Bold** | Headers, user labels, emphasis in LLM output | `\033[1m` |
| *Dim* | Metadata, timestamps, secondary info, code blocks | `\033[2m` |
| Underline | Links (if rendered), command hints | `\033[4m` |
| Reverse | Status bar, selected items in lists | `\033[7m` |
| ~~Strikethrough~~ | NOT USED (poor terminal support) | — |
| *Italic* | NOT USED (inconsistent terminal support) | — |

**Why no italic/strikethrough:** Terminal italic support varies wildly
(iTerm2 yes, Linux VT no, tmux sometimes). Bold and dim are universally
supported. We design for the lowest common denominator and enhance optionally.

### 3.3 Borders and Separators

**Message separators** — Light dashed lines between conversation turns:
```
  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
```
Using `─` (U+2500) with spaces. This is lighter than a solid line but clearly
delineates turns. Falls back to `- - - - -` (ASCII dashes) if Unicode
detection fails.

**Tool panels** — Light box-drawing for structure:
```
┌─ Tools ────────────────────────────────────┐
│  ✓ read_file  pkg/types/events.go    0.1s  │
└────────────────────────────────────────────┘
```
Using Unicode box-drawing: `┌ ─ ┐ │ └ ┘` (U+250C, U+2500, U+2510, U+2502,
U+2514, U+2518). Falls back to `+`, `-`, `|` on non-Unicode terminals.

**No heavy borders:** We never use double-line (`╔═╗`) or heavy (`┏━┓`)
box-drawing. They're visually noisy and look dated.

### 3.4 Code Block Rendering

**Current behavior** (`stream.go`): Code blocks render with `Dim` ANSI.

**Enhanced behavior:**

```
  ┌─ go ──────────────────────────────────────────────┐
  │  events := make(chan types.StreamEvent, 32)        │
  │  go func() {                                      │
  │      errCh <- provider.StreamChat(ctx, req, events)│
  │  }()                                              │
  │                                    ── 4 lines ──  │
  └───────────────────────────────────────────────────┘
```

**Design decisions:**
- Language label in the top border (extracted from ``` marker)
- Line count in the bottom-right
- No line numbers by default (reduces noise)
- Background: dim text, no actual background color (background colors break
  in many terminals)
- **No syntax highlighting in v1.** Syntax highlighting in terminals requires
  a tokenizer library (chroma for Go) and significantly increases complexity.
  The dim styling already differentiates code from prose. Add syntax
  highlighting in v2 using `github.com/alecthomas/chroma`.

### 3.5 Diff Display

For file modifications shown in permission prompts or tool results:

```
  ┌─ diff: internal/streaming/sse.go ─────────────────┐
  │  @@ -14,3 +14,5 @@ package streaming              │
  │                                                     │
  │    13 │  // NewPipeline creates a streaming pipeline │
  │  - 14 │  func NewPipeline(p InferenceProvider) *P…  │
  │  + 14 │  func NewPipeline(                          │
  │  + 15 │      p InferenceProvider,                   │
  │  + 16 │      opts ...PipelineOption,                │
  │  + 17 │  ) *Pipeline {                              │
  │    18 │      return &Pipeline{provider: p}          │
  │                                                     │
  └─────────────────────────────────────────────────────┘
```

**Color coding:**
- Removed lines: Red text, `- ` prefix (`\033[31m`)
- Added lines: Green text, `+ ` prefix (`\033[32m`)
- Context lines: Dim text, `  ` prefix (`\033[2m`)
- Hunk header: Cyan, dim (`\033[2;36m`)

**Accessibility:** Colors are paired with `+`/`-` prefix symbols. A color-blind
user can distinguish additions from removals by the prefix character alone.

### 3.6 Progress Indicators

**Braille spinner** for thinking/loading states:
```
⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
```
Cycles at 80ms intervals. Falls back to `- \ | /` on non-Unicode terminals.

**Why braille over other spinners:**
- Occupies exactly 1 character cell (important for layout stability)
- Smooth animation (10 frames vs 4 for `|/-\`)
- Universally recognizable as "loading"
- Widely supported since Unicode 6.0 (2010)

**Progress for known-length operations** (e.g., reading a large file):
```
  ⟳ read_file  data.json  ████████░░░░  67%  2.1s
```

**Token counter** (live during streaming):
```
  Assistant · claude-sonnet-4-20250514 · ⠹ 342 tokens...
```
Updates every ~10 tokens (not every token — avoids flicker).

### 3.7 Status Bar

One line, always visible at the bottom (using ANSI save/restore cursor or
terminal-specific mechanisms):

```
 ● connected  claude-sonnet-4-20250514  ses_01J…7mK  2.1k/200k tk  Ctrl+C cancel  /help
```

| Segment | Content | Update frequency |
|:---|:---|:---|
| Connection dot | `●` green = connected, `○` dim = disconnected, `●` yellow = reconnecting | On state change |
| Model name | Short model name (strip `:latest`) | On `/model` change |
| Session ID | Truncated ULID | On session change |
| Token usage | `current/max` with `k` suffix | After each response |
| Context hint | Changes based on state (streaming: "Ctrl+C cancel", idle: "/help") | On state change |

**Implementation:** The status bar uses ANSI cursor positioning to pin to the
bottom line. In raw terminal mode, save cursor → move to last line → render bar
→ restore cursor. This avoids the alternate screen buffer which would eat
scrollback.

---

## 4. Navigation & Keybindings

### 4.1 Design Philosophy: Emacs-style defaults, Readline compatible

**Rationale:** Most terminal users have muscle memory for readline keybindings
(bash, zsh, python REPL). We match those defaults. Vim keybindings are available
via `FORGE_KEYMAP=vi` (same as `set -o vi` in bash) but are not the default.

### 4.2 Core Keybindings

**Always active:**

| Key | Action | Context |
|:---|:---|:---|
| `Ctrl+C` | Cancel current operation / clear input | Universal |
| `Ctrl+D` | Send message (on content) / Exit (on empty) | Input |
| `Ctrl+L` | Clear screen, preserve history | Universal |
| `Ctrl+Z` | Suspend (standard Unix SIGTSTP) | Universal |

**During input editing:**

| Key | Action |
|:---|:---|
| `Enter` | Insert newline |
| `Enter` (blank line) | Send message |
| `Up` / `Ctrl+P` | Previous prompt from history |
| `Down` / `Ctrl+N` | Next prompt from history |
| `Ctrl+A` / `Home` | Move to beginning of line |
| `Ctrl+E` / `End` | Move to end of line |
| `Ctrl+W` | Delete word backward |
| `Ctrl+U` | Delete to beginning of line |
| `Ctrl+K` | Delete to end of line |
| `Ctrl+R` | Reverse search through prompt history |
| `Tab` | Autocomplete (slash commands, file paths, model names) |
| `Shift+Tab` | Previous autocomplete suggestion |

**During streaming:**

| Key | Action |
|:---|:---|
| `Ctrl+C` | Cancel current generation |
| (scrollback) | Standard terminal scroll (mouse wheel, Shift+PgUp) |

**During permission prompts:**

| Key | Action |
|:---|:---|
| `y` | Yes (approve this action) |
| `n` | No (deny this action) |
| `a` | Always (approve all similar actions this session) |
| `v` | View full diff/details |
| `Escape` | Same as `n` |

### 4.3 Prompt History

- History is per-session and persists across REPL restarts (stored in SQLite
  alongside session data, or in `~/.forge/history`)
- `Up`/`Down` cycle through previous user prompts (not assistant responses)
- `Ctrl+R` opens inline reverse search:
  ```
  (reverse-i-search)`stream': Explain the streaming architecture
  ```
- History is deduplicated (consecutive identical prompts stored once)
- Maximum 1000 entries per session, 10000 global

### 4.4 Tab Completion

**Context-aware completions:**

| Context | Completes |
|:---|:---|
| `/` prefix | Slash command names |
| `/model ` | Available model names from `registry.ListAllModels()` |
| `/load ` | Session IDs with title preview |
| `/export ` | Filesystem paths |
| General text | Not completed (we don't auto-complete natural language) |

**Completion display:**

Single match → inline completion (like bash):
```
forge [claude-sonnet-4-20250514]> /mod → /model
```

Multiple matches → list below prompt:
```
forge [claude-sonnet-4-20250514]> /mo
  /model    Show or switch model
  /models   List all available models
```

---

## 5. Accessibility

### 5.1 Screen Reader Considerations

Terminal screen readers (JAWS, NVDA with terminal support, VoiceOver) read
text as it appears in the terminal buffer.

**Design rules:**
- All status icons have text equivalents:
  - `✓ read_file` → also works as "check read_file" for screen readers
  - `● connected` → the word "connected" is always present
- No information conveyed by color alone (always paired with text or symbol)
- Spinner animation doesn't add semantic content — the word "Thinking..."
  appears alongside it
- Tool panels use ASCII-representable structure (screen readers can parse
  the text content even if box-drawing characters are garbled)

**ANSI and screen readers:** Most screen readers ignore ANSI escape sequences
and read the underlying text. Our use of ANSI is purely decorative (bold, color,
dim) and never carries semantic meaning. The raw text is always meaningful.

### 5.2 Color-Blind Safe Design

The palette is designed for the three most common forms of color vision
deficiency:

| Pair | How we differentiate | CVD-safe? |
|:---|:---|:---|
| Added (green) vs Removed (red) | `+` and `-` prefix characters | ✓ |
| Success (green) vs Error (red) | `✓` and `✗` icons + text labels | ✓ |
| Connected (green) vs Disconnected (dim) | Luminance difference (bright vs dim) | ✓ |
| Warning (yellow) vs Error (red) | `⏱` vs `✗` icons + text labels | ✓ |

**Testing:** Verify with Sim Daltonism or similar CVD simulator.
Run `FORGE_NO_COLOR=1` to test the fully monochrome experience.

### 5.3 Minimum Terminal Size

| Size | Behavior |
|:---|:---|
| < 40 cols | Error: "Terminal too narrow (need 40+ columns)" then exit |
| 40–59 cols | Compact mode: no borders on tool panels, truncated status bar |
| 60–79 cols | Standard mode: all features, shorter separators |
| 80+ cols | Full mode: all features, full-width separators |
| < 10 rows | Error: "Terminal too short (need 10+ rows)" then exit |
| 10–14 rows | No status bar, minimal chrome |
| 15+ rows | Full mode with status bar |

### 5.4 SSH / Remote Terminal Compatibility

| Concern | Mitigation |
|:---|:---|
| High latency | Batch terminal writes (write full line at once, not char-by-char) |
| Mosh | Works — Mosh handles ANSI fine, but we avoid clearing large screen regions |
| tmux/screen | Detect `$TERM` containing `screen` or `tmux`; avoid alternate screen |
| Serial console | `TERM=dumb` → disable all ANSI, plain text output |
| Windows Terminal | Works — modern Windows Terminal supports ANSI natively |
| PuTTY | Works — basic ANSI support; Unicode may need UTF-8 translation enabled |
| Terminal.app (macOS) | Works — but braille spinner may render incorrectly; fall back to `|/-\` |

**Detection hierarchy for capabilities:**
1. `TERM=dumb` → plain text mode, no ANSI
2. `NO_COLOR=1` or `FORGE_NO_COLOR=1` → no color, but bold/dim allowed
3. `COLORTERM=truecolor` → enable 24-bit colors (future)
4. Check `$TERM` for Unicode support → braille vs ASCII spinner
5. Default: ANSI 16-color + Unicode

### 5.5 tmux Compatibility

Specific tmux considerations:
- Don't use alternate screen buffer (it doesn't share scrollback)
- Use `\033[?7h` to ensure line wrapping is enabled
- Detect tmux via `$TMUX` env var
- Status bar pinning works via cursor positioning (tmux handles this correctly)
- Don't assume window size at startup — use SIGWINCH handler for resize events

---

## 6. Key Design Problems & Solutions

### 6.1 Problem: Tool Use Progress Without Overwhelming

**Current state** (`stream.go:97-104`): Tool calls print `[Calling: name]`
and arguments inline. This works but is noisy for multi-tool sequences.

**Solution: Progressive disclosure with three levels:**

**Level 1 — Inline summary (default):**
```
  ⟳ Reading 3 files...
```
Single line, updates in place (carriage return), disappears when tools complete.

**Level 2 — Tool panel (auto-shown for 2+ tool calls):**
```
  ┌─ Tools ───────────────────────────────────────────┐
  │  ✓ read_file  sse.go                  0.3s  189L  │
  │  ✓ read_file  events.go              0.1s   33L  │
  │  ⟳ read_file  openai.go               ...        │
  └───────────────────────────────────────────────────┘
```

**Level 3 — Expanded view (on demand via `/expand`):**
Shows full tool output with syntax-aware rendering.

**Transition rules:**
- 1 tool call → Level 1
- 2+ tool calls → Level 2
- User requests → Level 3
- After response completes → collapse to summary line

### 6.2 Problem: File Diffs That Are Easy to Review

**Solution: Unified diff with context, inside a bordered panel:**

```
  ┌─ write_file: internal/streaming/sse.go ──────────┐
  │                                                    │
  │  @@ -14,3 +14,5 @@                               │
  │    13 │  // NewPipeline creates a pipeline         │
  │  - 14 │  func NewPipeline(p Provider) *Pipeline {  │
  │  + 14 │  func NewPipeline(                         │
  │  + 15 │      p Provider,                           │
  │  + 16 │      opts ...Option,                       │
  │  + 17 │  ) *Pipeline {                             │
  │    18 │      return &Pipeline{provider: p}         │
  │                                                    │
  │  +3 lines, -1 line                                │
  └────────────────────────────────────────────────────┘
  [y]es  [n]o  [a]lways  [v]iew full
```

**Design rules for diffs:**
- Show 2 lines of context above and below each hunk
- Color: green for additions, red for removals (with +/- prefix for CVD)
- Line numbers on every line (not just changed lines)
- Summary line at bottom: "+N lines, -M lines"
- For large diffs (>30 lines), show first/last hunks with "[N more hunks]"
- `v` key opens full diff in `$PAGER` (or built-in pager)

### 6.3 Problem: Error Handling in TUI

**Error taxonomy and display:**

| Error Type | Display | Recovery |
|:---|:---|:---|
| Network error | `⚠ Connection lost. Retrying... (3/5)` | Auto-retry with backoff |
| Provider error | `✗ Provider error: rate limited (retry in 30s)` | Show countdown |
| Model not found | `✗ Model "xyz" not found. Available: ...` | List alternatives |
| Auth error | `✗ Authentication failed. Check FORGE_API_KEY` | Clear instruction |
| Timeout | `⏱ Request timed out after 60s` | Suggest retry |
| Stream error | Keep partial output + `[error: connection reset]` | Don't lose content |
| Invalid input | `✗ Message too large (102KB > 100KB limit)` | Show limit |

**Critical principle: Never lose user content.** If a stream fails mid-response,
keep the partial output visible. If the user typed a long message and submission
fails, the message stays in the input buffer for retry.

**Error styling:**
```
  ┌─ Error ──────────────────────────────────────────┐
  │  ✗  Provider error                               │
  │                                                   │
  │  Model "gpt-4o" rate limited.                     │
  │  Retry in 28s, or switch model: /model claude-sonnet-4-20250514  │
  └──────────────────────────────────────────────────┘
```

### 6.4 Problem: Making the TUI Feel Fast

**Perceived performance techniques:**

| Technique | Implementation |
|:---|:---|
| Instant spinner | Show `⠋ Thinking...` within 100ms of send (before any network round-trip) |
| Optimistic UI | Print `You: <message>` and separator immediately, don't wait for save confirmation |
| First-token focus | Stream SSE events with no batching — the moment a token arrives, print it |
| Typing ahead | Allow user to type next message while previous response is still streaming |
| Background saves | `sessionMgr.AddMessage()` runs in a goroutine — don't block the REPL loop |
| Connection pooling | Reuse HTTP connections to forge backend (already done via Go's default transport) |
| Startup pre-check | On launch, issue `GET /api/health` to verify connectivity before first prompt |

**What NOT to do:**
- Don't batch tokens for "smoother" rendering — users want to read as fast as the
  model generates
- Don't show a loading bar for unknown-length operations — use a spinner
- Don't clear the screen between turns — scrollback is the user's memory

### 6.5 Problem: Information Density vs. Clarity

**Solution: Three information modes, toggled with `/compact` and `/verbose`:**

**Compact mode** (`/compact` or `FORGE_DISPLAY=compact`):
```
You: Explain streaming
Assistant: The streaming architecture uses channels...
You: Show me the code
Assistant: Here's the relevant code...
```
No borders, no metadata, no tool panels. Pure conversation.

**Normal mode** (default):
```
  You
  Explain streaming

  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─

  Assistant · claude-sonnet-4-20250514 · 847tk · 3.2s
  The streaming architecture uses channels...

  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─
```
Separators, metadata, collapsed tool panels.

**Verbose mode** (`/verbose` or `FORGE_DISPLAY=verbose`):
```
  You [2025-07-15 14:23:01]
  Explain streaming

  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─

  Assistant · claude-sonnet-4-20250514 · msg_01J…
  847 tokens · TTFT 0.8s · total 3.2s · finish: stop

  The streaming architecture uses channels...

  ┌─ Tools (expanded) ────────────────────────────┐
  │  ✓ read_file  sse.go         0.3s  189L       │
  │    Line 14-189 shown                          │
  │  ✓ read_file  events.go      0.1s   33L       │
  └───────────────────────────────────────────────┘
```
Full timestamps, message IDs, TTFT, expanded tool output.

---

## 7. Anti-Patterns to Avoid

### 7.1 ✗ Don't Use Alternate Screen Buffer for Conversation

**Why:** Alternate screen (like vim, less, htop) destroys scrollback. When the
user exits, all conversation history vanishes. Inline REPL preserves scrollback
so users can scroll up, copy text, and reference previous turns.

**Exception:** Full-screen diff review mode (`/diff` command) may use alternate
screen because the user explicitly requested a focused view.

### 7.2 ✗ Don't Over-Animate

**Why:** Every animation consumes terminal bandwidth (matters over SSH), creates
accessibility issues (screen readers announce each update), and can cause
flicker in slow terminals.

**Rules:**
- Spinner: update at 80ms intervals (12.5 fps), not 16ms (60fps)
- Token counter: update every ~10 tokens, not every token
- Status bar: update only on state change, not periodically
- No blinking text. Ever. (`\033[5m` is banned.)

### 7.3 ✗ Don't Require Mouse

**Why:** Terminal users work via keyboard. Mouse support can be added (for
scrolling, clicking links) but must never be required. Every interaction must
have a keyboard equivalent.

### 7.4 ✗ Don't Fight the Terminal

**Why:** The terminal is not a canvas. Don't try to position arbitrary elements
at arbitrary coordinates (except the status bar). Don't try to create
"windows" or "dialogs" that overlap. Print text top-to-bottom, left-to-right,
the way terminals work.

**Specific mistakes to avoid:**
- Clearing the screen to redraw (use inline updates instead)
- Printing above the current cursor position (except status bar)
- Assuming a fixed terminal size (always handle SIGWINCH)
- Using `\r` to overwrite lines of different lengths (leaves garbage characters)

### 7.5 ✗ Don't Show Raw JSON or Stack Traces

**Why:** Users don't care about JSON. Parse everything into human-readable
format. If verbose debugging is needed, `FORGE_DEBUG=1` can enable raw output
to stderr.

```
BAD:  {"error":{"code":"model_not_found","message":"No provider...","type":"not_found"}}
GOOD: ✗ Model "xyz" not found. Try: /models to see available models
```

### 7.6 ✗ Don't Block on Non-Critical Operations

**Why:** Every blocking call is a moment the user can't type.

**Non-blocking operations (run in goroutines):**
- Saving messages to the database
- Updating session metadata (token count, last access)
- Health checks
- Model list refresh

**Blocking operations (acceptable):**
- Initial session creation (need the ID before prompting)
- Permission prompts (security-critical, must wait for answer)
- Provider resolution (need to know where to send the request)

### 7.7 ✗ Don't Assume Unicode

**Why:** Some terminals (serial consoles, very old xterm, Windows cmd.exe in
legacy mode) don't support Unicode.

**Detection and fallback:**
```go
// Check locale for UTF-8 support
lang := os.Getenv("LANG") + os.Getenv("LC_ALL") + os.Getenv("LC_CTYPE")
supportsUnicode := strings.Contains(strings.ToLower(lang), "utf")

// Also check TERM
if os.Getenv("TERM") == "dumb" {
    supportsUnicode = false
}
```

**Fallback table:**

| Unicode | ASCII fallback |
|:---|:---|
| `─` (U+2500) | `-` |
| `│` (U+2502) | `\|` |
| `┌┐└┘` | `+` |
| `✓` | `[ok]` |
| `✗` | `[FAIL]` |
| `⠋⠙⠹⠸` | `\|/-\\` |
| `●` | `*` |
| `○` | `o` |
| `⟳` | `...` |
| `█` (cursor) | `_` |

### 7.8 ✗ Don't Invent New Keybindings

**Why:** Every non-standard keybinding is a learning cost. Stick to readline
conventions. If a keybinding isn't in bash/zsh, it shouldn't be in forge.

**Specifically avoid:**
- `Ctrl+S` (freezes terminal on many systems — XOFF)
- `Ctrl+Q` (paired with Ctrl+S — XON)
- `Ctrl+\` (sends SIGQUIT with core dump)
- Function keys F1-F12 (inconsistent across terminals)
- Alt+key combinations (conflict with terminal/tmux prefix keys)

---

## 8. Implementation Roadmap

### 8.1 What Exists Today

The current `internal/cli/` package provides a solid foundation:

| File | What it does | Reuse? |
|:---|:---|:---|
| `repl.go` | Main REPL loop, slash commands, session management | ✓ Enhance in place |
| `stream.go` | SSE streaming with ANSI markdown rendering | ✓ Core renderer — enhance |
| `input.go` | stdin detection, prompt reading | ✓ Replace with raw terminal input |
| `run.go` | One-shot prompt execution | ✓ Keep as-is |
| `sessions.go` | Session list/show/delete CLI commands | ✓ Keep as-is |
| `models.go` | Model listing CLI command | ✓ Keep as-is |
| `help.go` | Usage/version/help printing | ✓ Enhance with new commands |
| `context.go` | Context window trimming | ✓ Keep as-is |
| `resolver.go` | ModelResolver interface | ✓ Keep as-is |

### 8.2 Phase 1: Enhanced REPL (Build on existing code)

**Goal:** Take the current working REPL and add polish.

1. **Raw terminal mode** — Replace `bufio.Scanner` with raw terminal input
   (use `golang.org/x/term` or `github.com/charmbracelet/bubbletea/v2` for
   input handling)
2. **Readline-style editing** — Line editing, history, Ctrl keybindings
3. **Status bar** — Persistent bottom line with connection/model/token info
4. **Thinking indicator** — Braille spinner before first token
5. **Response metadata** — Token count and latency after each response
6. **Tab completion** — For slash commands and model names
7. **Graceful Ctrl+C** — Already implemented in `repl.go:266-283`, enhance
   with cleanup

**Dependency:** `golang.org/x/term` for raw mode (in Go stdlib extended),
OR `github.com/charmbracelet/bubbletea/v2` (popular, well-tested, handles
all terminal edge cases).

### 8.3 Phase 2: Tool Use & Permissions

**Goal:** Rich tool use display with permission system.

1. **Tool panel rendering** — Bordered panels for tool call status
2. **Progressive disclosure** — Inline summary → panel → expanded
3. **Permission prompts** — Inline approval for destructive operations
4. **Diff display** — Colored unified diffs for file modifications

### 8.4 Phase 3: Power Features

**Goal:** Features that differentiate forge-link.

1. **Split pane mode** — Optional full-screen with session sidebar
2. **Syntax highlighting** — via chroma library
3. **Reverse search** — Ctrl+R through prompt history
4. **Session export** — Markdown/JSON export
5. **Pipe integration** — `cat file.go | forge run "explain this"`
   (already works via `input.go`)

### 8.5 Recommended Go Libraries

| Need | Library | Why |
|:---|:---|:---|
| Terminal input/TUI framework | `github.com/charmbracelet/bubbletea/v2` | Elm-architecture, handles raw mode, resize, mouse, all edge cases |
| Styling | `github.com/charmbracelet/lipgloss/v2` | Composable terminal styling, adapts to terminal capabilities |
| Spinner | `github.com/charmbracelet/bubbles` | Pre-built spinner, text input, viewport components |
| Markdown rendering | `github.com/charmbracelet/glamour` | Terminal markdown with themes |
| Syntax highlighting | `github.com/alecthomas/chroma/v2` | Comprehensive language support |
| Raw terminal | `golang.org/x/term` | Minimal dependency for raw mode only |

**Recommendation:** Use the Charm stack (bubbletea + lipgloss + bubbles).
It's the Go standard for TUI apps, handles all terminal edge cases, and
has excellent tmux/SSH compatibility. The existing `stream.go` rendering
can be wrapped as a bubbletea Model.

---

## 9. Summary of Key Decisions

| Decision | Choice | Rationale |
|:---|:---|:---|
| Layout model | Inline REPL | Preserves scrollback, works everywhere |
| Input method | Multi-line, blank-line-to-send | Already implemented, natural for prose |
| Streaming render | Character-by-character, ANSI markdown | Already implemented in stream.go |
| Color system | ANSI 16-color base | Maximum terminal compatibility |
| Keybindings | Readline/Emacs defaults | Muscle memory, no learning curve |
| Tool display | Progressive disclosure (3 levels) | Balances information vs. noise |
| Permission model | Inline prompts, single keypress | Security without friction |
| Full-screen mode | Deferred to v2 | Complexity vs. value for v1 |
| Framework | Charm stack (bubbletea) | Industry standard for Go TUI |
| Syntax highlighting | Deferred to v2 | Dim formatting is sufficient for v1 |
