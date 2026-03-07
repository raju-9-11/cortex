package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"forge/internal/inference"
	"forge/pkg/types"
)

// RunOnce executes a single prompt against the resolved provider and streams
// the response to w. Returns the complete response text.
func RunOnce(ctx context.Context, registry *inference.ProviderRegistry,
	model string, systemPrompt string, prompt string, w io.Writer) (string, error) {

	// 1. Resolve provider from model string
	provider, resolvedModel, err := registry.Resolve(model)
	if err != nil {
		return "", fmt.Errorf("resolving model %q: %w", model, err)
	}

	// 2. Build ChatCompletionRequest
	messages := []types.ChatMessage{}
	if systemPrompt != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, types.ChatMessage{Role: "user", Content: prompt})

	req := &types.ChatCompletionRequest{
		Model:    resolvedModel,
		Messages: messages,
		Stream:   true,
	}

	// 3. Create event channel and start streaming
	events := make(chan types.StreamEvent, 32)
	errCh := make(chan error, 1)

	go func() {
		errCh <- provider.StreamChat(ctx, req, events)
	}()

	// 4. Consume events, write deltas to w, accumulate full text
	var fullText strings.Builder
	for event := range events {
		switch event.Type {
		case types.EventContentDelta:
			fmt.Fprint(w, event.Delta)
			fullText.WriteString(event.Delta)
		case types.EventError:
			errMsg := event.ErrorMessage
			if event.Error != nil {
				errMsg = event.Error.Error()
			}
			return fullText.String(), fmt.Errorf("stream error: %s", errMsg)
		case types.EventToolCallStart:
			if event.ToolCall != nil {
				fmt.Fprintf(w, "\n[Calling: %s]\n", event.ToolCall.Name)
			}
		case types.EventToolCallComplete:
			fmt.Fprintln(w, "\n[Tool call complete]")
		}
	}

	// 5. Add trailing newline
	fmt.Fprintln(w)

	// 6. Wait for provider goroutine
	if err := <-errCh; err != nil {
		return fullText.String(), fmt.Errorf("provider error: %w", err)
	}

	return fullText.String(), nil
}
