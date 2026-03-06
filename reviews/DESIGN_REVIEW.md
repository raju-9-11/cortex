# 🎨 Forge: UI/UX Design Review

> **Reviewer:** Front-End Architect — Orchestrator Nexus
> **Date:** 2025-01-22
> **Status:** Complete
> **Scope:** Full design review of the Forge unified AI interface

---

## Executive Summary

The implementation plan is architecturally sound but **UI-anemic**. Phase 3 ("The Unified Web Interface") is described in two bullet points, which is insufficient for a product that will be *the* primary interface between the user and the AI engine. The backend is well-specified; the frontend needs equal rigor.

This review provides concrete, opinionated design specifications for every UI surface in Forge.

---

## 1. Chat UI Design (`/chat`)

### 1.1 Layout Architecture

Use a **three-column layout** that collapses responsively:

```
┌─────────────────────────────────────────────────────────┐
│  Topbar: Model Selector │ Session Title │ Theme Toggle  │
├──────────┬──────────────────────────────┬───────────────┤
│          │                              │               │
│ History  │     Message Thread           │  Inspector    │
│ Sidebar  │                              │  Panel        │
│ (260px)  │     (flex-1, min 400px)      │  (320px)      │
│          │                              │               │
│          │                              │  (collapsible │
│          │                              │   default off)│
│          ├──────────────────────────────┤               │
│          │  Composer Bar                │               │
└──────────┴──────────────────────────────┴───────────────┘
```

**Breakpoints (mobile-first):**
| Breakpoint | Behavior |
|:---|:---|
| `< 640px` (sm) | Sidebar hidden behind hamburger. Inspector as bottom sheet. Full-bleed messages. |
| `640–1024px` (md) | Sidebar as overlay drawer. Inspector as slide-over panel. |
| `≥ 1024px` (lg) | Full three-column. Sidebar persistent. Inspector toggle via keyboard. |
| `≥ 1440px` (xl) | Wider message column. Comfortable reading width capped at `720px` centered. |

### 1.2 Message Rendering

**Use flat messages, NOT chat bubbles.** Bubbles waste horizontal space and create visual noise in technical conversations. Follow the pattern established by ChatGPT/Claude — full-width message blocks with subtle sender differentiation.

```
┌──────────────────────────────────────────────────┐
│  [Avatar]  You                          12:34 PM │
│                                                  │
│  Can you explain the compaction algorithm?        │
│                                                  │
├──────────────────────────────────────────────────┤
│  [Avatar]  Forge · llama-3.1-70b        12:34 PM │
│                                                  │
│  The compaction engine works by...               │
│                                                  │
│  ```python                                       │
│  def compact(messages, max_tokens):              │
│      ...                                         │
│  ```                                             │
│                                                  │
│  [Copy] [Regenerate]                   142 tokens│
└──────────────────────────────────────────────────┘
```

**Design decisions:**

| Element | Specification |
|:---|:---|
| **User messages** | `bg-zinc-50 dark:bg-zinc-900` — subtle background, left-aligned. |
| **Assistant messages** | `bg-white dark:bg-zinc-950` — no background (default canvas). Visually "native." |
| **Avatar** | 28×28px rounded square. User: initials on `bg-blue-600`. Assistant: Forge icon (anvil/hammer glyph). |
| **Sender label** | `text-sm font-semibold text-zinc-900 dark:text-zinc-100`. Model name shown as muted badge next to assistant label. |
| **Timestamp** | `text-xs text-zinc-400`. Right-aligned. Relative ("2m ago") with absolute on hover via `title` attr. |
| **Message body** | `text-base leading-relaxed text-zinc-800 dark:text-zinc-200`. Max prose width: `65ch`. |
| **Action buttons** | Appear on hover (desktop) or always visible (mobile). Ghost buttons: `text-zinc-400 hover:text-zinc-700`. |
| **Token count** | Per-message, right-aligned, `text-xs text-zinc-400`. Only on assistant messages. |

### 1.3 Streaming Text Rendering

This is the most important UX detail in the entire app. Get it right.

**Strategy: Chunked DOM Append with CSS Cursor**

1. As SSE `content` deltas arrive, append text nodes to the current `<p>` or inline element.
2. Do NOT re-render the entire message on each token. Use a `ref` to the message container and append directly. React reconciliation on every token is a performance disaster.
3. Place a blinking cursor (`▊`) as a `::after` pseudo-element on the last text node via a `.streaming` class.

```css
/* Streaming cursor */
.message-streaming .prose > *:last-child::after {
  content: '▊';
  animation: cursor-blink 1s steps(2) infinite;
  color: theme('colors.blue.500');
  font-weight: 400;
  margin-left: 1px;
}

@keyframes cursor-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
```

**Markdown rendering during stream:**

- Use a lightweight incremental markdown parser. **Do not** run a full markdown pass on every token.
- Recommended approach: buffer tokens until a "block boundary" is detected (double newline, code fence opening/closing, list marker). Parse completed blocks. Render the trailing incomplete block as plain text with the cursor.
- Library choice: `marked` with custom renderer, or `react-markdown` with memoized block splitting. Prefer `marked` for performance — React's VDOM diffing on every token is unnecessary overhead.

**Scroll behavior:**

- Auto-scroll to bottom while streaming **only if** the user is already at the bottom (within 100px threshold).
- If the user scrolls up to read, **stop auto-scrolling** and show a "↓ New content below" pill button anchored above the composer.
- On click or pressing `End`, smooth-scroll to bottom and re-engage auto-scroll.

### 1.4 Code Block Rendering

Use **Shiki** for syntax highlighting — it uses the same grammars as VS Code and produces superior output compared to Prism or highlight.js. Since Forge targets developers, this quality matters.

```
┌─ python ──────────────────────────────── [Copy] ─┐
│                                                  │
│  1 │ def compact(messages, max_tokens):           │
│  2 │     threshold = max_tokens * 0.9             │
│  3 │     if count_tokens(messages) > threshold:   │
│  4 │         summary = summarize(messages[1:-5])  │
│  5 │         return [messages[0], summary]         │
│  6 │         + messages[-5:]                       │
│                                                  │
└──────────────────────────────────────────────────┘
```

| Element | Specification |
|:---|:---|
| **Container** | `rounded-lg border border-zinc-200 dark:border-zinc-800 overflow-hidden` |
| **Header bar** | `bg-zinc-100 dark:bg-zinc-800 px-4 py-2 text-xs font-mono` — language label left, copy button right. |
| **Code body** | `bg-zinc-50 dark:bg-zinc-900 p-4 text-sm font-mono overflow-x-auto` |
| **Line numbers** | `text-zinc-400 select-none pr-4 text-right` — present for blocks > 3 lines. |
| **Copy button** | Lucide `Copy` icon → transitions to `Check` icon for 2s after click. `aria-label="Copy code"`. |
| **Horizontal scroll** | Custom thin scrollbar: `scrollbar-thin scrollbar-thumb-zinc-300 dark:scrollbar-thumb-zinc-700`. |

**During streaming:** Render code blocks with plain `<pre>` styling until the closing fence is detected. Then apply Shiki highlighting in one pass. This avoids re-highlighting on every token.

### 1.5 Tool Execution Cards

When Forge pauses the stream to execute a tool, show an **inline status card** within the message flow:

```
┌──────────────────────────────────────────────────┐
│  ⚡ Tool: terminal_exec                          │
│  ┌────────────────────────────────────────────┐  │
│  │  $ grep -r 'todo' .                        │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  ● Running...                          2.3s      │
│  ━━━━━━━━━━━━━━━━━━━━━━━━░░░░░░░░░░░░          │
└──────────────────────────────────────────────────┘
```

After completion:

```
┌──────────────────────────────────────────────────┐
│  ✓ Tool: terminal_exec               3.1s       │
│  ┌────────────────────────────────────────────┐  │
│  │  $ grep -r 'todo' .                        │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  ▸ Output (10 lines)                  [Expand]   │
└──────────────────────────────────────────────────┘
```

**States:**

| State | Icon | Color | Behavior |
|:---|:---|:---|:---|
| Pending | `Loader2` (spinning) | `text-blue-500` | Pulse animation on card border. |
| Running | `Loader2` (spinning) | `text-blue-500` | Show elapsed time counter. Indeterminate progress bar. |
| Success | `CheckCircle2` | `text-green-500` | Output collapsed by default. Expandable with `<details>`. |
| Error | `XCircle` | `text-red-500` | Error message shown inline. Stderr in expandable block. |
| Timeout | `Clock` | `text-amber-500` | Show timeout value from tool manifest. |

**Output rendering:** Tool output should be rendered in a `<pre>` block with a max-height of `200px` and scroll. Long outputs get a "Show full output" button.

### 1.6 Error States

**Connection errors (SSE/WebSocket drop):**
- Banner at top of message thread: `bg-red-50 dark:bg-red-950 border-l-4 border-red-500`.
- Text: "Connection lost. Retrying..." with a countdown timer.
- After 3 failed retries: "Unable to connect to Forge server. [Retry Now]".

**Inference errors (model failure, OOM, etc.):**
- Inline in the message thread as a system message:
  ```
  ┌──────────────────────────────────────────────────┐
  │  ⚠ Error: Model returned status 500             │
  │  The inference backend is not responding.        │
  │                                                  │
  │  [Retry] [Change Model]                          │
  └──────────────────────────────────────────────────┘
  ```
- Error messages must be **actionable**. Never show "Something went wrong" without a next step.

**Empty states:**
- New conversation: centered illustration (simple SVG anvil icon) + "Start a conversation with Forge" + suggested prompts as clickable chips.
- No conversation history: "No conversations yet" in the sidebar.

### 1.7 Loading Indicators

| Context | Indicator |
|:---|:---|
| Initial page load | Skeleton screen: 3 placeholder message blocks with shimmer animation. |
| Waiting for first token | Three-dot bounce animation below the last user message, left-aligned with assistant avatar. Accessible: `aria-label="Forge is thinking"` + `role="status"`. |
| Model loading (cold start) | Full-width thin progress bar at top of viewport (like YouTube/GitHub). `aria-label="Loading model"`. |
| History loading | Skeleton list items in sidebar. |

---

## 2. Chat Features

### 2.1 Conversation Management

**Sidebar layout:**

```
┌─ Conversations ─────────── [+ New] ─┐
│                                      │
│  🔍 Search conversations...          │
│                                      │
│  TODAY                               │
│  ● Compaction algorithm debug   2m   │
│  ○ Docker compose setup        1h    │
│                                      │
│  YESTERDAY                           │
│  ○ Go embedding strategies     23h   │
│  ○ React streaming patterns    24h   │
│                                      │
│  PREVIOUS 7 DAYS                     │
│  ○ Initial project scaffolding  3d   │
│  ○ SSE protocol design          5d   │
│                                      │
│──────────────────────────────────────│
│  ⚙ Settings     📊 Inspector        │
└──────────────────────────────────────┘
```

- **Auto-titling:** After the first assistant response, fire a background request to the LLM: `"Generate a 4-6 word title for this conversation: {first_exchange}"`. Update sidebar entry. User can rename inline by clicking the title.
- **Grouped by time:** Today / Yesterday / Previous 7 Days / Older. Use `Intl.RelativeTimeFormat`.
- **Context menu (right-click or `⋯` button):** Rename, Pin, Export as Markdown, Delete. Delete requires confirmation via a popover, NOT a modal. Modals are hostile.
- **Search:** Client-side fuzzy search on titles. Server-side full-text search across message content (SQLite FTS5).
- **Active indicator:** `bg-zinc-100 dark:bg-zinc-800` background + `border-l-2 border-blue-500` left accent.

### 2.2 Model Selector

Place in the **topbar**, visually prominent. This is a critical workflow control.

```
┌──────────────────────────────────────┐
│  [🔥 llama-3.1-70b ▾]  │  Untitled  │
└──────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│  LOCAL MODELS                        │
│  ● llama-3.1-70b       ✓ Connected  │
│  ○ codestral-22b       ✓ Connected  │
│  ○ llama-3.1-8b        ✗ Offline    │
│                                      │
│  REMOTE PROVIDERS                    │
│  ○ gpt-4o (OpenAI)     ✓ API Key    │
│  ○ claude-3.5 (Anthro) ✗ No Key     │
│                                      │
│  [Manage Providers →]                │
└──────────────────────────────────────┘
```

- **Status badges:** Real-time health check via the WebSocket event bus. Green dot = healthy. Red dot = unreachable. Grey dot = not configured.
- **Keyboard:** `Cmd/Ctrl+M` opens selector. Arrow keys navigate. Enter selects. Escape closes.
- **Persistence:** Selected model persists per-conversation in the session DB. Global default in settings.
- **Provider grouping:** Local models first (they're free and private — align with Forge's ethos).

### 2.3 System Prompt Editor

Accessible via a **collapsible panel** at the top of the message thread, above the first message.

```
┌──────────────────────────────────────────────────┐
│  📝 System Prompt                     [Collapse] │
│  ┌────────────────────────────────────────────┐  │
│  │ You are a helpful coding assistant.        │  │
│  │ Always explain your reasoning step by step.│  │
│  └────────────────────────────────────────────┘  │
│  [Save as Default]        Token count: 24        │
└──────────────────────────────────────────────────┘
```

- **Textarea** with auto-resize (min 2 rows, max 10 rows, then scroll).
- **Token count** displayed live as the user types (debounced 300ms).
- **Presets dropdown:** "Coding Assistant", "Technical Writer", "General Chat", "Custom". Stored in localStorage.
- **Per-conversation override:** Each conversation stores its system prompt. Changing it mid-conversation appends a system message marker.
- **"Save as Default"** persists to settings.

### 2.4 File & Image Upload

**Trigger:** Paperclip icon in the composer bar + drag-and-drop on the message area.

```
┌──────────────────────────────────────────────────┐
│  ┌──────┐ ┌──────┐                               │
│  │ 📄   │ │ 🖼️   │                               │
│  │main  │ │screen│  Explain this code and the    │
│  │.go   │ │shot  │  screenshot...                 │
│  │2.1KB │ │340KB │                               │
│  │  [✕] │ │  [✕] │                               │
│  └──────┘ └──────┘                               │
│                                        [Send ↑]  │
└──────────────────────────────────────────────────┘
```

- **File chips** appear above the text input as removable thumbnails.
- **Images:** Thumbnail preview (max 80×80px). Sent as base64 in the multimodal message format if the model supports vision. If not, show a warning: "Current model doesn't support images."
- **Text files:** Read content and inject as a code block in the message. Show filename and size.
- **Size limit:** 10MB client-side validation. Larger files rejected with clear feedback.
- **Drag-and-drop:** Visual feedback — message area gets `ring-2 ring-blue-500 ring-dashed bg-blue-50/10` on dragover.

### 2.5 Copy / Share / Export

| Action | Trigger | Behavior |
|:---|:---|:---|
| **Copy message** | Hover action button on each message | Copy raw markdown to clipboard. Toast: "Copied!" |
| **Copy code block** | Button in code block header | Copy code content only (no line numbers). |
| **Export conversation** | Context menu on sidebar → "Export" | Download as `.md` file. Filename: `{title}-{date}.md`. |
| **Share conversation** | Future feature (Phase 5+) | Generate a read-only link. Requires hosted mode with auth. |

### 2.6 Keyboard Shortcuts

Implement a **global shortcut layer** using a custom hook. Display available shortcuts via `Cmd/Ctrl+/` (help overlay).

| Shortcut | Action |
|:---|:---|
| `Cmd/Ctrl+N` | New conversation |
| `Cmd/Ctrl+M` | Open model selector |
| `Cmd/Ctrl+K` | Focus search in sidebar |
| `Cmd/Ctrl+Shift+I` | Toggle inspector panel |
| `Cmd/Ctrl+Shift+D` | Toggle dark/light theme |
| `Cmd/Ctrl+Enter` | Send message (when in multi-line mode) |
| `Enter` | Send message (single-line mode, default) |
| `Shift+Enter` | New line in composer |
| `Escape` | Close any open panel/modal/dropdown; stop generation |
| `Cmd/Ctrl+Shift+C` | Copy last assistant response |
| `↑` (in empty composer) | Edit last user message |
| `Cmd/Ctrl+/` | Show keyboard shortcut overlay |

### 2.7 Theme Toggle

**Two modes:** Light and Dark. No "system" auto-detect as a third option — just follow system preference as the initial default, then respect the user's explicit toggle.

- **Toggle location:** Topbar, right side. Sun/Moon icon (`Sun`/`Moon` from Lucide).
- **Implementation:** `<html class="dark">` toggled via React context. Persisted to `localStorage`.
- **Transition:** `transition-colors duration-200` on `body`. No flash of unstyled content — read preference from `localStorage` in a blocking `<script>` in `<head>` before React hydrates.

**Color palette:**

| Token | Light | Dark |
|:---|:---|:---|
| `--bg-primary` | `#ffffff` (white) | `#09090b` (zinc-950) |
| `--bg-secondary` | `#f4f4f5` (zinc-100) | `#18181b` (zinc-900) |
| `--bg-tertiary` | `#e4e4e7` (zinc-200) | `#27272a` (zinc-800) |
| `--text-primary` | `#18181b` (zinc-900) | `#fafafa` (zinc-50) |
| `--text-secondary` | `#71717a` (zinc-500) | `#a1a1aa` (zinc-400) |
| `--accent` | `#2563eb` (blue-600) | `#3b82f6` (blue-500) |
| `--border` | `#e4e4e7` (zinc-200) | `#27272a` (zinc-800) |

All pairs exceed **4.5:1 contrast ratio** (WCAG AA for normal text).

---

## 3. Inspector UI (`/inspector`)

The Inspector should be available as **both** a standalone page (`/inspector`) and an **inline panel** within the chat view (toggled via `Cmd/Ctrl+Shift+I`).

### 3.1 Layout

```
┌──────────────────────────────────────────────────┐
│  Inspector                    [Dock to Chat ↗]   │
├──────────────────────────────────────────────────┤
│                                                  │
│  ┌─ Token Usage ─────────────────────────────┐   │
│  │                                           │   │
│  │  Context: ████████████████░░░░  78%       │   │
│  │           6,240 / 8,192 tokens            │   │
│  │                                           │   │
│  │  Prompt: 1,200  Completion: 340           │   │
│  │  System: 24     History: 4,676            │   │
│  └───────────────────────────────────────────┘   │
│                                                  │
│  ┌─ Context Viewer ──────────────────────────┐   │
│  │  [System] [Messages] [Raw JSON]           │   │
│  │                                           │   │
│  │  ▸ system (24 tokens)                     │   │
│  │  ▸ user: "Can you explain..." (18 tok)    │   │
│  │  ▾ assistant: "The compaction..." (142)   │   │
│  │    The compaction engine works by          │   │
│  │    monitoring the total token count...     │   │
│  │  ▸ user: "Show me the code" (6 tok)       │   │
│  └───────────────────────────────────────────┘   │
│                                                  │
│  ┌─ Event Stream ────────────────────────────┐   │
│  │  12:34:01.234  SSE    content delta       │   │
│  │  12:34:01.456  SSE    content delta       │   │
│  │  12:34:02.001  TOOL   terminal_exec START │   │
│  │  12:34:05.123  TOOL   terminal_exec DONE  │   │
│  │  12:34:05.200  SSE    content delta       │   │
│  │  12:34:06.100  SSE    stream end          │   │
│  │                                           │   │
│  │  [Clear] [Pause] [Export]                 │   │
│  └───────────────────────────────────────────┘   │
└──────────────────────────────────────────────────┘
```

### 3.2 Token Usage Visualization

**Primary display: Segmented horizontal bar.**

- Segments: System prompt (purple), Conversation history (blue), Current prompt (green), Remaining capacity (zinc-200 empty).
- Hover on any segment shows a tooltip with exact count and percentage.
- **Warning threshold:** Bar turns amber at 80%, red at 95%.
- **Compaction indicator:** When compaction fires, show a brief flash animation on the bar + a timeline event: `"Context compacted: 6,240 → 2,100 tokens"`.

**Secondary display: Numeric breakdown below the bar.**

```
Prompt: 1,200 tokens   │   Completion: 340 tokens   │   Total: 1,540 tokens
Cost estimate: ~$0.0023  (if using a remote provider with known pricing)
```

### 3.3 Raw Context Viewer

An **accordion list** of every message in the current context window.

- Each item shows: role icon, truncated content (first 60 chars), token count badge.
- Click to expand: full message content in a read-only code block.
- **"Raw JSON" tab:** The exact payload being sent to the inference provider, pretty-printed with syntax highlighting. Developers will love this for debugging.
- **Diff view:** After compaction, optionally show what was removed/summarized (before/after toggle).

### 3.4 Tool Call Timeline

A **vertical timeline** showing tool executions within the current conversation.

```
──●── terminal_exec            3.1s    ✓ Success
  │   $ grep -r 'todo' .
  │   10 results
  │
──●── web_search               1.2s    ✓ Success
  │   "rust async patterns"
  │   3 results
  │
──●── file_read                0.1s    ✗ Error
  │   /etc/shadow
  │   Permission denied
```

- Click any node to see full input/output in a slide-over detail panel.
- Color-coded by status: green (success), red (error), amber (timeout), blue (running).
- Elapsed time shown on the right.

### 3.5 Real-Time Event Stream

A **log-style scrolling feed** of WebSocket events.

- Each event: timestamp (ms precision), event type badge (color-coded), summary text.
- **Filterable** by event type: `content`, `status`, `tool_result`, `error`, `system`.
- **Pause button:** Stops auto-scroll but keeps buffering events. Resume replays buffered entries.
- **Export:** Download as JSONL file for external analysis.
- **Max buffer:** Keep last 1,000 events in memory. Older events evicted.

---

## 4. Accessibility (WCAG AA Compliance)

### 4.1 Screen Reader Support for Streaming Content

This is the hardest accessibility challenge in the app. Streaming tokens arriving 10–50 times per second will cause screen reader chaos if handled naively.

**Strategy: Debounced `aria-live` Region**

1. The message thread container is a `<main role="main">`.
2. Each assistant message has a **visually hidden** `<div aria-live="polite" aria-atomic="false">` companion.
3. As tokens stream in, **do NOT** update the live region on every token. Instead, buffer tokens and flush to the live region every **3 seconds** or on sentence boundaries (period + space).
4. When streaming completes, announce: "Forge response complete. {N} tokens."
5. Tool execution cards: announce "Executing tool: {name}" on start, "Tool {name} completed" on finish.

```tsx
// Debounced screen reader announcer
const announceRef = useRef<HTMLDivElement>(null);
const bufferRef = useRef('');

useEffect(() => {
  const interval = setInterval(() => {
    if (bufferRef.current && announceRef.current) {
      announceRef.current.textContent = bufferRef.current;
      bufferRef.current = '';
    }
  }, 3000);
  return () => clearInterval(interval);
}, []);
```

### 4.2 Keyboard Navigation

| Area | Behavior |
|:---|:---|
| **Sidebar** | `Tab` enters sidebar. Arrow keys navigate conversation list. `Enter` selects. `Delete` prompts removal. |
| **Message thread** | `Tab` moves focus through messages. Each message is a focusable `<article>` with `tabindex="0"`. Action buttons within each message are reached via nested `Tab`. |
| **Composer** | Always reachable via `Cmd/Ctrl+Shift+Enter` from anywhere. `Tab` from last message reaches composer. |
| **Model selector** | Opens as a listbox (`role="listbox"`). `ArrowUp`/`ArrowDown` navigate. `Enter` selects. `Escape` closes. |
| **Inspector** | Tab order: Token bar → Context list → Event stream. Each accordion item in the context viewer is keyboard-operable. |

**Focus trapping:** When a dropdown, popover, or the mobile sidebar is open, trap focus within it. Restore focus to the trigger element on close.

**Skip navigation:** Add a visually-hidden "Skip to main content" link as the first focusable element.

### 4.3 Color Contrast

All text/background pairs validated against WCAG AA (4.5:1 for normal text, 3:1 for large text and UI components):

| Pair | Ratio | Pass? |
|:---|:---|:---|
| `zinc-900` on `white` | 15.4:1 | ✅ AAA |
| `zinc-50` on `zinc-950` | 17.4:1 | ✅ AAA |
| `zinc-500` on `white` | 4.6:1 | ✅ AA |
| `zinc-400` on `zinc-950` | 5.3:1 | ✅ AA |
| `blue-600` on `white` | 4.7:1 | ✅ AA |
| `blue-500` on `zinc-950` | 4.6:1 | ✅ AA |
| `red-500` on `white` | 4.0:1 | ⚠️ Fails AA for small text |

**Fix for red:** Use `red-600` (`#dc2626`, ratio 4.6:1) for error text on light backgrounds. On dark backgrounds, `red-400` (`#f87171`, ratio 4.6:1 on `zinc-950`) is fine.

**Non-color indicators:** Never convey information through color alone. Tool status uses icon + color + text label. Token usage bar has numeric labels alongside the visual fill.

### 4.4 Focus Management

- **New message arrival:** Do NOT steal focus from the composer. Users expect to keep typing.
- **Conversation switch:** Move focus to the first message in the new conversation, or the composer if the conversation is empty.
- **Error dismissal:** Return focus to the element that was focused before the error appeared.
- **Modal/popover close:** Return focus to the trigger button.
- **Visible focus ring:** `focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2`. Use `focus-visible` (not `focus`) to avoid showing rings on mouse clicks.

### 4.5 Reduced Motion

Respect `prefers-reduced-motion`:

```css
@media (prefers-reduced-motion: reduce) {
  .message-streaming .prose > *:last-child::after {
    animation: none;
    opacity: 1;
  }
  * {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}
```

---

## 5. Design System

### 5.1 Component Library: Radix UI Primitives + Tailwind

**Do NOT build custom dropdown menus, dialogs, popovers, or tooltips from scratch.** Use [Radix UI Primitives](https://www.radix-ui.com/primitives) — they are unstyled, fully accessible, and compose perfectly with Tailwind.

**Required Radix primitives:**

| Component | Usage |
|:---|:---|
| `@radix-ui/react-dropdown-menu` | Model selector, conversation context menu |
| `@radix-ui/react-dialog` | Settings dialog, keyboard shortcuts overlay |
| `@radix-ui/react-popover` | Delete confirmation, system prompt editor |
| `@radix-ui/react-tooltip` | Timestamp details, token breakdowns |
| `@radix-ui/react-accordion` | Inspector context viewer, tool output |
| `@radix-ui/react-tabs` | Inspector view tabs (Context / Events / Timeline) |
| `@radix-ui/react-scroll-area` | Custom scrollbars on sidebar and message thread |
| `@radix-ui/react-toggle` | Theme toggle |
| `@radix-ui/react-visually-hidden` | Screen reader announcements |

**NOT recommended:** shadcn/ui. It's popular but adds an abstraction layer that's unnecessary when you're building a focused product. Radix primitives + Tailwind is the lower-level, more controllable approach. You want to own every pixel in this UI.

### 5.2 Typography Scale

Use Tailwind's default scale with a constrained subset for consistency:

| Token | Tailwind | Size | Usage |
|:---|:---|:---|:---|
| `--text-xs` | `text-xs` | 12px | Timestamps, token counts, metadata |
| `--text-sm` | `text-sm` | 14px | Sidebar items, labels, secondary UI |
| `--text-base` | `text-base` | 16px | Message body, input text |
| `--text-lg` | `text-lg` | 18px | Conversation title in topbar |
| `--text-xl` | `text-xl` | 20px | Empty state headings |

**Font stack:**
- **UI text:** `font-sans` → `Inter, system-ui, -apple-system, sans-serif`. Inter is the standard for modern web apps. Load via Google Fonts with `font-display: swap`, or self-host (preferred for single-binary — embed the woff2 in the Go binary).
- **Code:** `font-mono` → `'JetBrains Mono', 'Fira Code', 'Cascadia Code', ui-monospace, monospace`. Self-host JetBrains Mono (OFL license allows redistribution).
- **Line height:** `leading-relaxed` (1.625) for message body. `leading-normal` (1.5) for UI text. `leading-tight` (1.25) for headings.

### 5.3 Spacing Scale

Stick to Tailwind's 4px base grid: `1` (4px), `2` (8px), `3` (12px), `4` (16px), `6` (24px), `8` (32px), `12` (48px), `16` (64px).

**Specific conventions:**
- **Card padding:** `p-4` (16px).
- **Section gaps:** `gap-6` (24px) between major sections.
- **Inline element gaps:** `gap-2` (8px) between buttons, badges.
- **Message vertical spacing:** `py-6` (24px) between messages. Generous spacing reduces cognitive load.

### 5.4 Animation & Transition Guidelines

**Philosophy:** Animations should be **functional**, not decorative. They should communicate state changes, not delight.

| Animation | Duration | Easing | Purpose |
|:---|:---|:---|:---|
| Theme switch | 200ms | `ease-in-out` | Color transitions on all elements |
| Sidebar open/close | 250ms | `ease-out` | Slide from left + fade overlay |
| Dropdown open | 150ms | `ease-out` | Scale from 95% + fade in |
| Dropdown close | 100ms | `ease-in` | Fade out (faster than open — feels snappy) |
| Toast notification | 300ms in, 200ms out | `ease-out` / `ease-in` | Slide up from bottom-right |
| Streaming cursor | 1000ms | `steps(2)` | Blink on/off |
| Tool progress pulse | 2000ms | `ease-in-out` | Subtle border pulse on tool cards |
| Skeleton shimmer | 1500ms | `linear` | Left-to-right gradient sweep |
| Copy confirmation | Instant swap, 2000ms hold | — | Icon swap: Copy → Check → Copy |

**Spring physics:** NOT recommended. They add visual flair but increase JS bundle size (need framer-motion) and are distracting in a productivity tool. Use CSS transitions and `@keyframes` exclusively.

### 5.5 Icon System

Lucide Icons (already specified in the plan). Consistent usage:

| Icon | Context |
|:---|:---|
| `MessageSquare` | Chat / Conversation |
| `Plus` | New conversation |
| `Search` | Search |
| `Settings` | Settings |
| `Activity` | Inspector |
| `Sun` / `Moon` | Theme toggle |
| `Copy` / `Check` | Copy action / success |
| `Send` | Send message |
| `Paperclip` | File attachment |
| `ChevronDown` | Dropdowns |
| `Loader2` | Spinning loader |
| `AlertTriangle` | Warning |
| `XCircle` | Error |
| `CheckCircle2` | Success |
| `Terminal` | Tool execution |
| `Zap` | Lightning bolt for tool calls |

**Size convention:** `16px` for inline, `20px` for buttons, `24px` for primary navigation. Always pass `aria-hidden="true"` when the icon is decorative (adjacent to text label). Use `aria-label` when the icon is the sole content of a button.

---

## 6. Missing UI Concerns in the Plan

### 6.1 Settings Page

The plan mentions `FORGE_API_KEY` and `DATABASE_URL` as env vars but provides no UI for configuration. A settings page is essential.

**Proposed Settings Page (`/settings` or modal dialog):**

```
┌─ Settings ───────────────────────────────────────┐
│                                                  │
│  PROVIDERS                                       │
│  ┌────────────────────────────────────────────┐  │
│  │ Ollama          http://localhost:11434      │  │
│  │                 ✓ Connected     [Test] [✏] │  │
│  ├────────────────────────────────────────────┤  │
│  │ OpenAI          sk-...redacted             │  │
│  │                 ✓ Valid Key     [Test] [✏] │  │
│  ├────────────────────────────────────────────┤  │
│  │ Anthropic       Not configured             │  │
│  │                              [Add API Key] │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  DEFAULTS                                        │
│  Default Model:    [llama-3.1-70b      ▾]        │
│  System Prompt:    [Coding Assistant   ▾]        │
│  Theme:            [System ○] [Light ○] [Dark ●] │
│  Send on Enter:    [● Yes] [○ No (Cmd+Enter)]   │
│                                                  │
│  TOOL SANDBOX                                    │
│  Execution Mode:   [Local ●] [Docker ○]         │
│  Timeout:          [10] seconds                  │
│  Allowed Dirs:     /home/user/projects           │
│                    [Add Directory]                │
│                                                  │
│  DANGER ZONE                                     │
│  [Clear All Conversations]                       │
│  [Reset to Defaults]                             │
│                                                  │
└──────────────────────────────────────────────────┘
```

**API key handling:**
- Keys entered in the UI should be stored in the SQLite database, encrypted at rest.
- Show only last 4 characters after entry: `sk-...a1b2`.
- **"Test" button** fires a lightweight API call (e.g., list models) and shows ✓/✗.
- Env vars (`FORGE_API_KEY`, `OPENAI_API_KEY`, etc.) take precedence over UI-configured values. Show a badge: "Set via environment variable" with the field disabled.

### 6.2 Onboarding / First-Run Experience

On first launch (no conversations in DB, no models configured), show a **guided setup wizard**, NOT just an empty chat.

**Step 1: Welcome**
```
┌──────────────────────────────────────────────────┐
│                                                  │
│                🔨                                │
│            Welcome to Forge                      │
│                                                  │
│    A unified AI interface that runs on           │
│    your machine, your terms.                     │
│                                                  │
│              [Get Started →]                     │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Step 2: Connect a Model**
```
┌──────────────────────────────────────────────────┐
│                                                  │
│  Connect Your First Model                        │
│                                                  │
│  ┌────────────────────────────────────────────┐  │
│  │ 🖥️  Ollama (Local)                         │  │
│  │ Run models on your own hardware.           │  │
│  │ Auto-detected at localhost:11434  ✓        │  │
│  │                                [Connect]   │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  ┌────────────────────────────────────────────┐  │
│  │ 🌐  OpenAI                                 │  │
│  │ Use GPT-4o and other OpenAI models.        │  │
│  │ Requires API key.                          │  │
│  │ API Key: [________________________] [Save] │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  ┌────────────────────────────────────────────┐  │
│  │ 🌐  Anthropic                              │  │
│  │ Use Claude and other Anthropic models.     │  │
│  │ Requires API key.                          │  │
│  │ API Key: [________________________] [Save] │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│                              [Skip for now →]    │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Auto-detection:** On first run, Forge should probe `localhost:11434` (Ollama default) and `localhost:8080` (llama.cpp default). If found, pre-populate and show ✓.

**Step 3: Ready**
```
┌──────────────────────────────────────────────────┐
│                                                  │
│            You're all set! 🎉                    │
│                                                  │
│  Connected to: llama-3.1-70b via Ollama          │
│                                                  │
│  Try these:                                      │
│  [Explain how Forge's compaction works]          │
│  [Write a Go HTTP handler with streaming]        │
│  [Debug this error message: ...]                 │
│                                                  │
│             [Start Chatting →]                    │
│                                                  │
└──────────────────────────────────────────────────┘
```

### 6.3 Connection Status Indicator

The plan mentions SSE and WebSocket but has no UI for connection health. Critical for local-first tools where the server might crash or restart.

**Implementation:**

- **Topbar, right side:** Small colored dot next to the model name.
  - 🟢 Green: Connected, model healthy.
  - 🟡 Amber: Reconnecting (WebSocket dropped, auto-retrying).
  - 🔴 Red: Disconnected for >10 seconds.

- **WebSocket heartbeat:** Send a ping every 15 seconds. If no pong within 5 seconds, mark as disconnected and begin reconnection with exponential backoff (1s, 2s, 4s, 8s, max 30s).

- **Reconnection banner:**
  ```
  ┌──────────────────────────────────────────────────┐
  │ ⚠ Connection lost. Reconnecting in 4s... [Retry]│
  └──────────────────────────────────────────────────┘
  ```
  Sticky at the top of the message thread. `role="alert"` for screen readers.

- **On reconnect:** Re-subscribe to the event stream. If a generation was in progress, show: "Connection restored. The previous response may be incomplete. [Regenerate]".

### 6.4 Rate Limiting Feedback

For remote providers (OpenAI, Anthropic) that enforce rate limits:

- **429 responses:** Parse `Retry-After` header. Show an inline message:
  ```
  ⏱ Rate limited by OpenAI. Retrying in 32 seconds... [Cancel]
  ```
  Include a countdown timer. Auto-retry when the timer expires.

- **Token quota warnings:** If the API returns quota information in headers, show a persistent banner when approaching limits:
  ```
  ⚠ OpenAI usage: 92% of monthly quota consumed.
  ```

- **Per-request cost estimate:** For remote providers with known pricing, show estimated cost in the inspector:
  ```
  This request: ~$0.003 (1,200 input + 340 output tokens)
  Session total: ~$0.047
  ```

### 6.5 Additional Missing Concerns

**a) Stop Generation Button**

Not mentioned anywhere in the plan. When the LLM is streaming, the Send button must transform into a **Stop button** (square icon). Pressing it:
1. Sends an abort signal to the backend (cancel the inference context — the plan mentions this in Risk Mitigation but not in the UI).
2. Terminates the SSE stream.
3. Keeps the partial response in the conversation.
4. Re-enables the composer.
5. Keyboard: `Escape` while streaming should trigger this.

**b) Regenerate Response**

A "Regenerate" button (Lucide `RefreshCw`) on the last assistant message. Removes the last assistant response and re-sends the conversation to the LLM. Essential for non-deterministic outputs.

**c) Edit & Resend**

Clicking "Edit" on a user message should:
1. Populate the composer with that message's content.
2. Fork the conversation at that point (messages after the edited one are removed or moved to a "branch").
3. Re-send with the edited content.

**d) Toast Notification System**

Needed for non-blocking feedback: "Copied!", "Conversation deleted", "Settings saved", "Export downloaded". Use a stack in the bottom-right corner with auto-dismiss after 3 seconds. Maximum 3 visible toasts.

**e) Mobile Gesture Support**

- Swipe right from left edge → open sidebar.
- Swipe left on a conversation in sidebar → reveal delete/pin actions.
- Pull down on message thread → no action (prevent accidental refresh).
- Long-press on a message → show action menu (copy, regenerate).

**f) URL Routing**

The plan implies `/chat` and `/inspector` as routes but doesn't discuss:
- `/chat/:conversationId` — deep links to specific conversations.
- `/settings` — settings page.
- `/` — redirect to `/chat`.
- Use React Router with `BrowserRouter`. The Go server must serve `index.html` for all unmatched routes (SPA fallback).

---

## 7. Summary of Recommendations

### Critical (Must have for Phase 3)

| # | Item | Priority |
|:---|:---|:---|
| 1 | Flat message layout with proper spacing and action buttons | P0 |
| 2 | Performant streaming renderer (DOM append, not React re-render) | P0 |
| 3 | Stop generation button | P0 |
| 4 | Code block syntax highlighting (Shiki) | P0 |
| 5 | Tool execution inline cards with status states | P0 |
| 6 | Connection status indicator + reconnection logic | P0 |
| 7 | Dark/light theme with no FOUC | P0 |
| 8 | Keyboard shortcuts (at minimum: new chat, send, stop) | P0 |
| 9 | `aria-live` region for streaming with debounced updates | P0 |
| 10 | Error states with actionable recovery | P0 |

### Important (Should have for Phase 3)

| # | Item | Priority |
|:---|:---|:---|
| 11 | Conversation sidebar with search and grouping | P1 |
| 12 | Model selector with health status | P1 |
| 13 | System prompt editor with token count | P1 |
| 14 | Inspector panel (token usage + context viewer) | P1 |
| 15 | Copy message / copy code block | P1 |
| 16 | Regenerate response | P1 |
| 17 | Settings page for provider configuration | P1 |
| 18 | First-run onboarding wizard | P1 |
| 19 | Radix UI primitives for all interactive components | P1 |
| 20 | Mobile responsive layout with gesture support | P1 |

### Nice to Have (Phase 4+)

| # | Item | Priority |
|:---|:---|:---|
| 21 | File/image upload | P2 |
| 22 | Export conversations as Markdown | P2 |
| 23 | Tool call timeline in inspector | P2 |
| 24 | Edit & resend messages (conversation branching) | P2 |
| 25 | Rate limiting feedback with countdown | P2 |
| 26 | Per-request cost estimation | P2 |
| 27 | Full keyboard shortcut overlay (`Cmd+/`) | P2 |
| 28 | Event stream export as JSONL | P2 |

---

## 8. File Structure Recommendation

```
ui/
├── public/
│   └── index.html              # Blocking theme script in <head>
├── src/
│   ├── main.tsx                 # Entry point
│   ├── App.tsx                  # Router setup
│   ├── components/
│   │   ├── chat/
│   │   │   ├── MessageThread.tsx
│   │   │   ├── MessageBubble.tsx      # (despite name, flat layout)
│   │   │   ├── StreamingRenderer.tsx  # DOM-append based renderer
│   │   │   ├── CodeBlock.tsx          # Shiki-powered
│   │   │   ├── ToolCard.tsx           # Inline tool execution UI
│   │   │   ├── Composer.tsx           # Input bar + attachments
│   │   │   └── EmptyState.tsx         # First conversation prompt
│   │   ├── inspector/
│   │   │   ├── InspectorPanel.tsx
│   │   │   ├── TokenUsageBar.tsx
│   │   │   ├── ContextViewer.tsx
│   │   │   ├── EventStream.tsx
│   │   │   └── ToolTimeline.tsx
│   │   ├── sidebar/
│   │   │   ├── ConversationList.tsx
│   │   │   ├── ConversationItem.tsx
│   │   │   └── SearchBar.tsx
│   │   ├── settings/
│   │   │   ├── SettingsDialog.tsx
│   │   │   ├── ProviderConfig.tsx
│   │   │   └── DefaultsForm.tsx
│   │   ├── onboarding/
│   │   │   ├── WelcomeWizard.tsx
│   │   │   └── ModelConnect.tsx
│   │   └── shared/
│   │       ├── Topbar.tsx
│   │       ├── ModelSelector.tsx
│   │       ├── ThemeToggle.tsx
│   │       ├── ConnectionStatus.tsx
│   │       ├── Toast.tsx
│   │       ├── SkipNavigation.tsx
│   │       └── KeyboardShortcuts.tsx
│   ├── hooks/
│   │   ├── useSSE.ts              # SSE connection + reconnection
│   │   ├── useWebSocket.ts        # WebSocket for event bus
│   │   ├── useTheme.ts            # Theme context + persistence
│   │   ├── useKeyboardShortcuts.ts
│   │   ├── useScrollAnchor.ts     # Auto-scroll with user override
│   │   └── useScreenReaderAnnounce.ts
│   ├── lib/
│   │   ├── api.ts                 # API client
│   │   ├── markdown.ts            # Incremental markdown parser
│   │   ├── tokens.ts              # Client-side token estimation
│   │   └── shortcuts.ts           # Shortcut definitions
│   ├── stores/
│   │   ├── conversation.ts        # Zustand store for chat state
│   │   ├── settings.ts            # Zustand store for user prefs
│   │   └── inspector.ts           # Zustand store for inspector data
│   └── styles/
│       ├── globals.css            # Tailwind directives + custom props
│       └── streaming.css          # Cursor animation + reduced motion
└── tailwind.config.ts
```

**State management:** Use **Zustand** — lightweight (~1KB), no boilerplate, works great with React 18+. Redux is overkill. React Context is fine for theme/auth but causes unnecessary re-renders for high-frequency updates like streaming tokens.

---

*End of design review. This document should be treated as the UI specification for Phase 3 implementation.*
