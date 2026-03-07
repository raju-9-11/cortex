package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"forge/pkg/types"
)

// ANSI escape codes
const (
	Bold   = "\033[1m"
	Reset  = "\033[0m"
	Dim    = "\033[2m"
	Cyan   = "\033[36m"
	Yellow = "\033[33m"
	Green  = "\033[32m"
	Red    = "\033[31m"
)

// renderState tracks markdown formatting state across streaming deltas.
type renderState struct {
	inBold      bool
	inCodeBlock bool
	inInline    bool
	// pending holds a single character ('*' or '`') that might be the
	// start of a multi-character marker. We buffer it until the next
	// character arrives so we can decide how to handle it.
	pending byte
	// pendingCount tracks how many consecutive pending chars we've
	// accumulated (e.g. 1 backtick, 2 backticks, etc.)
	pendingCount int
}

// RenderStream reads events from the channel and writes content deltas
// to w in real-time with ANSI formatting. Returns the complete assembled
// response text (unformatted, for session persistence) and any error.
// Blocks until the channel is closed or ctx is cancelled.
func RenderStream(ctx context.Context, events <-chan types.StreamEvent, w io.Writer) (fullText string, err error) {
	var (
		raw     strings.Builder
		state   renderState
		lastCh  byte
		written bool // whether we wrote any content delta
	)

	for {
		select {
		case <-ctx.Done():
			// Flush any pending character before returning.
			if state.pendingCount > 0 {
				flushPending(&state, &raw, w)
			}
			// Reset any open ANSI formatting so the terminal is clean.
			if state.inBold || state.inCodeBlock || state.inInline {
				fmt.Fprint(w, Reset)
			}
			return raw.String(), ctx.Err()

		case ev, ok := <-events:
			if !ok {
				// Channel closed — flush pending and add trailing newline.
				if state.pendingCount > 0 {
					flushPending(&state, &raw, w)
				}
				// Reset any open ANSI formatting.
				if state.inBold || state.inCodeBlock || state.inInline {
					fmt.Fprint(w, Reset)
				}
				if written {
					lastCh = lastByte(raw.String())
					if lastCh != '\n' {
						fmt.Fprint(w, "\n")
					}
				}
				return raw.String(), err
			}

			switch ev.Type {
			case types.EventContentDelta:
				written = true
				processDelta(ev.Delta, &state, &raw, w)

			case types.EventToolCallStart:
				name := ""
				if ev.ToolCall != nil {
					name = ev.ToolCall.Name
				}
				fmt.Fprintf(w, "\n%s[Calling: %s]%s\n", Yellow, name, Reset)

			case types.EventToolCallDelta:
				args := ""
				if ev.ToolCall != nil {
					args = ev.ToolCall.Arguments
				}
				fmt.Fprintf(w, "%s%s%s", Dim, args, Reset)

			case types.EventToolCallComplete:
				fmt.Fprintf(w, "\n%s[Tool call complete]%s\n", Green, Reset)

			case types.EventError:
				msg := ev.ErrorMessage
				if msg == "" && ev.Error != nil {
					msg = ev.Error.Error()
				}
				fmt.Fprintf(w, "\n%sError: %s%s\n", Red, msg, Reset)
				if ev.Error != nil {
					err = ev.Error
				} else {
					err = fmt.Errorf("%s", msg)
				}

			case types.EventContentDone, types.EventStatus, types.EventToolResult:
				// Silent — no output.
			}
		}
	}
}

// processDelta handles a single content delta string, character by character,
// applying ANSI formatting and accumulating raw text.
func processDelta(delta string, state *renderState, raw *strings.Builder, w io.Writer) {
	for i := 0; i < len(delta); i++ {
		ch := delta[i]

		// If we have a pending character, resolve it.
		if state.pendingCount > 0 {
			resolved := resolvePending(ch, state, raw, w)
			if resolved {
				continue // ch was consumed as part of the marker
			}
			// ch was not part of the marker — fall through to process it normally.
		}

		// Inside code blocks, only triple-backtick can close — everything else is literal.
		if state.inCodeBlock && ch != '`' {
			emitChar(ch, state, raw, w)
			continue // skip * handling
		}

		switch ch {
		case '*':
			// Start buffering — might be the first of '**'.
			state.pending = '*'
			state.pendingCount = 1

		case '`':
			// Start buffering — might be the first of '```'.
			state.pending = '`'
			state.pendingCount = 1

		default:
			emitChar(ch, state, raw, w)
		}
	}
}

// resolvePending checks whether the new character continues or completes
// a pending marker sequence. Returns true if ch was consumed.
func resolvePending(ch byte, state *renderState, raw *strings.Builder, w io.Writer) bool {
	if ch == state.pending {
		state.pendingCount++

		// Handle completed markers.
		switch {
		case state.pending == '*' && state.pendingCount == 2:
			// '**' — toggle bold
			toggleBold(state, w)
			state.pendingCount = 0
			state.pending = 0
			return true

		case state.pending == '`' && state.pendingCount == 3:
			// '```' — toggle code block
			toggleCodeBlock(state, w)
			state.pendingCount = 0
			state.pending = 0
			return true

		default:
			// Keep accumulating (e.g., we have 1 backtick, wait for more).
			return true
		}
	}

	// ch is different from pending — flush the pending chars literally
	// and let ch be processed normally.
	flushPending(state, raw, w)
	return false
}

// flushPending emits the buffered pending characters as literal text.
func flushPending(state *renderState, raw *strings.Builder, w io.Writer) {
	if state.pendingCount == 0 {
		return
	}

	switch {
	case state.pending == '`' && state.pendingCount == 1:
		// Single backtick — toggle inline code.
		toggleInlineCode(state, w)
	case state.pending == '`' && state.pendingCount == 2:
		// Two backticks — emit them literally (unusual markdown, but handle gracefully).
		for j := 0; j < state.pendingCount; j++ {
			emitChar(state.pending, state, raw, w)
		}
	default:
		// Single '*' or other — emit literally.
		for j := 0; j < state.pendingCount; j++ {
			emitChar(state.pending, state, raw, w)
		}
	}

	state.pendingCount = 0
	state.pending = 0
}

// toggleBold toggles bold ANSI formatting.
func toggleBold(state *renderState, w io.Writer) {
	if state.inBold {
		fmt.Fprint(w, Reset)
		// Re-apply any outer formatting that was active.
		if state.inCodeBlock {
			fmt.Fprint(w, Dim)
		}
		state.inBold = false
	} else {
		state.inBold = true
		fmt.Fprint(w, Bold)
	}
}

// toggleCodeBlock toggles code block (dim) ANSI formatting.
// Also consumes the rest of the opening line (language hint) silently for raw text.
func toggleCodeBlock(state *renderState, w io.Writer) {
	if state.inCodeBlock {
		fmt.Fprint(w, Reset)
		state.inCodeBlock = false
	} else {
		state.inCodeBlock = true
		fmt.Fprint(w, Dim)
	}
}

// toggleInlineCode toggles inline code (cyan) ANSI formatting.
func toggleInlineCode(state *renderState, w io.Writer) {
	if state.inInline {
		fmt.Fprint(w, Reset)
		// Re-apply any outer formatting.
		if state.inCodeBlock {
			fmt.Fprint(w, Dim)
		}
		state.inInline = false
	} else {
		state.inInline = true
		fmt.Fprint(w, Cyan)
	}
}

// emitChar writes a single character to both the ANSI writer and the raw accumulator.
func emitChar(ch byte, state *renderState, raw *strings.Builder, w io.Writer) {
	raw.WriteByte(ch)
	fmt.Fprintf(w, "%c", ch)
}

// lastByte returns the last byte of s, or 0 if s is empty.
func lastByte(s string) byte {
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}
