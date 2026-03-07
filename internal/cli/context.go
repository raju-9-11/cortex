package cli

import (
	"forge/internal/inference"
	"forge/pkg/types"
)

// trimHistory reduces the message list to fit within the provider's context window.
// It preserves the system prompt (messages with role="system") and the most recent
// non-system message, trimming oldest non-system messages first.
// Returns the trimmed message list and the number of messages removed.
func trimHistory(messages []types.ChatMessage, provider inference.InferenceProvider, model string) ([]types.ChatMessage, int) {
	caps := provider.Capabilities(model)
	maxTokens := caps.MaxContextTokens
	if maxTokens <= 0 {
		return messages, 0 // no limit known, send everything
	}

	// Reserve ~25% for the response.
	budget := maxTokens * 3 / 4

	// Count current tokens.
	tokenCount, err := provider.CountTokens(messages)
	if err != nil || tokenCount <= budget {
		return messages, 0 // fits fine
	}

	// Separate system prompt(s) from the rest.
	var systemMsgs []types.ChatMessage
	var historyMsgs []types.ChatMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			historyMsgs = append(historyMsgs, m)
		}
	}

	// Always keep at least the last non-system message.
	if len(historyMsgs) == 0 {
		return messages, 0
	}

	// Trim oldest history messages until we fit within budget.
	removed := 0
	for len(historyMsgs) > 1 { // keep at least the last message
		candidate := make([]types.ChatMessage, 0, len(systemMsgs)+len(historyMsgs))
		candidate = append(candidate, systemMsgs...)
		candidate = append(candidate, historyMsgs...)
		count, err := provider.CountTokens(candidate)
		if err != nil || count <= budget {
			break
		}
		historyMsgs = historyMsgs[1:] // drop oldest
		removed++
	}

	result := make([]types.ChatMessage, 0, len(systemMsgs)+len(historyMsgs))
	result = append(result, systemMsgs...)
	result = append(result, historyMsgs...)
	return result, removed
}
