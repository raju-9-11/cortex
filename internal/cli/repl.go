package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"forge/internal/session"
	"forge/internal/store"
	"forge/pkg/types"
)

// RunREPL starts an interactive chat session.
// Creates a new session or resumes an existing one.
// Blocks until the user exits (/exit, Ctrl+D, or EOF).
func RunREPL(ctx context.Context, registry ModelResolver,
	sessionMgr session.Manager, model string,
	sessionID string, systemPrompt string,
	in io.Reader, out io.Writer) error {

	var sess *store.Session
	var err error

	// 1. Create or resume session.
	if sessionID != "" {
		sess, err = sessionMgr.Get(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load session %q: %w", sessionID, err)
		}
		model = sess.Model

		// Display message history for resumed sessions.
		msgs, err := sessionMgr.GetMessages(ctx, sess.ID)
		if err != nil {
			return fmt.Errorf("failed to load messages: %w", err)
		}
		if len(msgs) > 0 {
			fmt.Fprintf(out, "\n--- Resuming session (last %d messages) ---\n\n", min(len(msgs), 10))
			start := 0
			if len(msgs) > 10 {
				start = len(msgs) - 10
			}
			for _, m := range msgs[start:] {
				switch m.Role {
				case "user":
					fmt.Fprintf(out, "You: %s\n\n", m.Content)
				case "assistant":
					fmt.Fprintf(out, "Assistant: %s\n\n", m.Content)
				case "system":
					// Skip system messages in display
				}
			}
			fmt.Fprintf(out, "--- End of history ---\n\n")
		}
	} else {
		sess, err = sessionMgr.Create(ctx, session.CreateParams{
			Model:        model,
			SystemPrompt: systemPrompt,
			UserID:       "default",
		})
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		// If a system prompt was provided, save it as the first message.
		if systemPrompt != "" {
			if addErr := sessionMgr.AddMessage(ctx, sess.ID, &store.Message{
				Role:    "system",
				Content: systemPrompt,
			}); addErr != nil {
				fmt.Fprintf(out, "Warning: failed to save system prompt: %v\n", addErr)
			}
		}
	}

	// 2. Print welcome banner.
	fmt.Fprintf(out, "\n🔥 Forge Chat\n")
	fmt.Fprintf(out, "  Model:   %s\n", model)
	fmt.Fprintf(out, "  Session: %s\n", sess.ID)
	fmt.Fprintf(out, "\nType your message and press Enter twice (blank line) to send.\n")
	fmt.Fprintf(out, "Type /help for commands, /exit to quit.\n\n")

	// 3. Main loop.
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line
	for {
		fmt.Fprintf(out, "forge [%s]> ", shortModel(model))

		input, eof := readInput(scanner)
		if eof {
			// Ctrl+D / EOF — exit cleanly.
			fmt.Fprintf(out, "\nGoodbye! Session %s saved.\n", sess.ID)
			return nil
		}

		input = strings.TrimSpace(input)

		// Empty input → re-prompt.
		if input == "" {
			continue
		}

		// Slash commands.
		if strings.HasPrefix(input, "/") {
			cmd, arg := parseSlashCommand(input)
			switch cmd {
			case "/exit", "/quit":
				fmt.Fprintf(out, "Goodbye! Session %s saved.\n", sess.ID)
				return nil

			case "/help":
				printHelp(out)
				continue

			case "/new":
				newSess, err := sessionMgr.Create(ctx, session.CreateParams{
					Model:        model,
					SystemPrompt: systemPrompt,
					UserID:       "default",
				})
				if err != nil {
					fmt.Fprintf(out, "Error creating session: %v\n", err)
					continue
				}
				sess = newSess
				if systemPrompt != "" {
					if addErr := sessionMgr.AddMessage(ctx, sess.ID, &store.Message{
						Role:    "system",
						Content: systemPrompt,
					}); addErr != nil {
						fmt.Fprintf(out, "Warning: failed to save system prompt: %v\n", addErr)
					}
				}
				fmt.Fprintf(out, "New session created: %s\n", sess.ID)
				continue

			case "/sessions":
				sessions, err := sessionMgr.List(ctx, "default")
				if err != nil {
					fmt.Fprintf(out, "Error listing sessions: %v\n", err)
					continue
				}
				if len(sessions) == 0 {
					fmt.Fprintf(out, "No sessions found.\n")
				} else {
					fmt.Fprintf(out, "%-16s  %-20s  %s\n", "ID", "Title", "Messages")
					for _, s := range sessions {
						fmt.Fprintf(out, "%-16s  %-20s  %d\n", s.ID, truncate(s.Title, 20), s.MessageCount)
					}
				}
				continue

			case "/load":
				if arg == "" {
					fmt.Fprintf(out, "Usage: /load <session-id>\n")
					continue
				}
				loaded, err := sessionMgr.Get(ctx, arg)
				if err != nil {
					fmt.Fprintf(out, "Error loading session: %v\n", err)
					continue
				}
				sess = loaded
				model = sess.Model
				fmt.Fprintf(out, "Switched to session %s (model: %s)\n", sess.ID, model)

				// Show last few messages.
				msgs, err := sessionMgr.GetMessages(ctx, sess.ID)
				if err != nil {
					fmt.Fprintf(out, "Warning: could not load messages: %v\n", err)
				} else if len(msgs) > 0 {
					start := 0
					if len(msgs) > 5 {
						start = len(msgs) - 5
					}
					fmt.Fprintf(out, "\n--- Last messages ---\n")
					for _, m := range msgs[start:] {
						switch m.Role {
						case "user":
							fmt.Fprintf(out, "You: %s\n", m.Content)
						case "assistant":
							fmt.Fprintf(out, "Assistant: %s\n", m.Content)
						}
					}
					fmt.Fprintf(out, "---\n\n")
				}
				continue

			case "/model":
				if arg == "" {
					fmt.Fprintf(out, "Current model: %s\nUsage: /model <name>\n", model)
					continue
				}
				model = arg
				_, err := sessionMgr.Update(ctx, sess.ID, session.UpdateParams{
					Model: &model,
				})
				if err != nil {
					fmt.Fprintf(out, "Error updating model: %v\n", err)
					continue
				}
				fmt.Fprintf(out, "Model switched to: %s\n", model)
				continue

			default:
				fmt.Fprintf(out, "Unknown command: %s (type /help for commands)\n", cmd)
				continue
			}
		}

		// 4. Save user message.
		if err := sessionMgr.AddMessage(ctx, sess.ID, &store.Message{
			Role:    "user",
			Content: input,
		}); err != nil {
			fmt.Fprintf(out, "Error saving message: %v\n", err)
			continue
		}

		// 5. Build full message context.
		msgs, err := sessionMgr.GetMessages(ctx, sess.ID)
		if err != nil {
			fmt.Fprintf(out, "Error loading context: %v\n", err)
			continue
		}

		chatMessages := make([]types.ChatMessage, len(msgs))
		for i, m := range msgs {
			chatMessages[i] = types.ChatMessage{
				Role:    m.Role,
				Content: m.Content,
			}
		}

		// 6. Resolve provider and stream response.
		provider, resolvedModel, err := registry.Resolve(model)
		if err != nil {
			fmt.Fprintf(out, "Error: %v\n", err)
			continue
		}

		req := &types.ChatCompletionRequest{
			Model:    resolvedModel,
			Messages: chatMessages,
			Stream:   true,
		}

		events := make(chan types.StreamEvent, 32)

		// Create a cancellable context for this generation so Ctrl+C
		// cancels only the current streaming, not the whole REPL.
		genCtx, genCancel := context.WithCancel(ctx)

		errCh := make(chan error, 1)
		go func() {
			errCh <- provider.StreamChat(genCtx, req, events)
		}()

		fmt.Fprintf(out, "\n")
		assistantContent, streamErr := RenderStream(genCtx, events, out)

		// Cancel the generation context first so the provider goroutine
		// can exit, then drain remaining events to unblock channel sends.
		genCancel()
		for range events {
		}

		providerErr := <-errCh

		if errors.Is(streamErr, context.Canceled) || errors.Is(providerErr, context.Canceled) {
			fmt.Fprintf(out, "[cancelled]\n")
			continue
		}

		if streamErr != nil || providerErr != nil {
			if providerErr != nil && streamErr == nil {
				fmt.Fprintf(out, "\nError: %v\n", providerErr)
			}
			continue
		}

		fmt.Fprintf(out, "\n")

		// 7. Save assistant message.
		if assistantContent != "" {
			if err := sessionMgr.AddMessage(ctx, sess.ID, &store.Message{
				Role:    "assistant",
				Content: assistantContent,
				Model:   model,
			}); err != nil {
				fmt.Fprintf(out, "Warning: failed to save response: %v\n", err)
			}
		}
	}
}

// maxInputSize is the maximum accumulated multi-line input size (1 MB).
// If exceeded, input is truncated and a warning is printed to stderr.
const maxInputSize = 1024 * 1024

// readInput reads lines from the scanner until a blank line is encountered
// (double Enter) or EOF. Slash commands (lines starting with "/") are returned
// immediately without waiting for a blank line.
// Returns the accumulated input and whether EOF was reached.
func readInput(scanner *bufio.Scanner) (string, bool) {
	var lines []string
	totalSize := 0
	for {
		if !scanner.Scan() {
			// EOF or error — return whatever we have.
			if len(lines) > 0 {
				return strings.Join(lines, "\n"), false
			}
			return "", true
		}

		line := scanner.Text()

		// If this is the first line and it's a slash command, return immediately.
		if len(lines) == 0 && strings.HasPrefix(strings.TrimSpace(line), "/") {
			return strings.TrimSpace(line), false
		}

		// Blank line = send.
		if line == "" {
			break
		}

		// Check accumulated size before appending.
		totalSize += len(line) + 1 // +1 for the newline separator
		if totalSize > maxInputSize {
			fmt.Fprintf(os.Stderr, "Warning: input exceeds 1 MB limit, truncating.\n")
			break
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n"), false
}

// parseSlashCommand splits "/cmd arg1 arg2" into ("/cmd", "arg1 arg2").
func parseSlashCommand(input string) (string, string) {
	parts := strings.SplitN(input, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}
	return cmd, arg
}

// printHelp prints the available slash commands.
func printHelp(out io.Writer) {
	fmt.Fprintln(out, "Available commands:")
	fmt.Fprintln(out, "  /help              Show this help message")
	fmt.Fprintln(out, "  /exit, /quit       Exit the REPL")
	fmt.Fprintln(out, "  /new               Start a new session")
	fmt.Fprintln(out, "  /sessions          List recent sessions")
	fmt.Fprintln(out, "  /load <id>         Switch to an existing session")
	fmt.Fprintln(out, "  /model <name>      Switch to a different model")
}

// shortModel returns a shorter display name for the model
// (e.g., "llama3.2:latest" → "llama3.2").
func shortModel(model string) string {
	if idx := strings.LastIndex(model, ":latest"); idx > 0 {
		return model[:idx]
	}
	return model
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
