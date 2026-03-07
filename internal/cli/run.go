package cli

import (
	"context"
	"fmt"
	"io"

	"forge/internal/inference"
	"forge/pkg/types"
)

// RunOnce executes a single prompt against the resolved provider and streams
// the response to w with ANSI formatting via RenderStream.
// Returns the complete response text (unformatted).
func RunOnce(ctx context.Context, registry *inference.ProviderRegistry,
	model string, systemPrompt string, prompt string, w io.Writer) (string, error) {

	provider, resolvedModel, err := registry.Resolve(model)
	if err != nil {
		return "", fmt.Errorf("resolving model %q: %w", model, err)
	}

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

	events := make(chan types.StreamEvent, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.StreamChat(ctx, req, events)
	}()

	fullText, renderErr := RenderStream(ctx, events, w)

	// Drain remaining events so the provider goroutine can finish
	// and close the channel. Without this, a cancelled context leaves
	// the goroutine blocked on out<-event permanently.
	if ctx.Err() != nil {
		go func() { for range events {} }()
	}

	if providerErr := <-errCh; providerErr != nil {
		return fullText, fmt.Errorf("provider error: %w", providerErr)
	}

	if renderErr != nil {
		return fullText, renderErr
	}

	return fullText, nil
}
