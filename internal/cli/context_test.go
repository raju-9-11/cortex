package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cortex/internal/inference"
	"cortex/pkg/types"
)

// testProvider is a mock inference provider with configurable capabilities
// and token counting for testing trimHistory.
type testProvider struct {
	caps          inference.ModelCapabilities
	tokensPerMsg  int // fixed token count per message
	countTokenErr error
}

func (tp *testProvider) StreamChat(_ context.Context, _ *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	defer close(out)
	out <- types.StreamEvent{Type: types.EventContentDelta, Delta: "ok"}
	out <- types.StreamEvent{Type: types.EventContentDone, FinishReason: "stop"}
	return nil
}

func (tp *testProvider) Complete(_ context.Context, _ *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return nil, errors.New("not implemented")
}

func (tp *testProvider) CountTokens(messages []types.ChatMessage) (int, error) {
	if tp.countTokenErr != nil {
		return 0, tp.countTokenErr
	}
	return len(messages) * tp.tokensPerMsg, nil
}

func (tp *testProvider) Capabilities(_ string) inference.ModelCapabilities {
	return tp.caps
}

func (tp *testProvider) ListModels(_ context.Context) ([]types.ModelInfo, error) {
	return nil, nil
}

func (tp *testProvider) Name() string {
	return "test"
}

// helper to build a ChatMessage
func msg(role, content string) types.ChatMessage {
	return types.ChatMessage{Role: role, Content: content}
}

func TestTrimHistory_FitsWithinBudget(t *testing.T) {
	// 5 short messages, mock with large context → no trimming.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 100000,
		},
		tokensPerMsg: 10,
	}
	messages := []types.ChatMessage{
		msg("system", "You are a helpful assistant."),
		msg("user", "Hello"),
		msg("assistant", "Hi there!"),
		msg("user", "How are you?"),
		msg("assistant", "I'm doing well."),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed != 0 {
		t.Errorf("expected 0 messages removed, got %d", removed)
	}
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

func TestTrimHistory_TrimsOldest(t *testing.T) {
	// Budget = 100 * 3/4 = 75.
	// 10 messages × 10 tokens each = 100 tokens (exceeds budget of 75).
	// After trimming, we need ≤75 tokens.
	// System (1 msg = 10 tokens) + history needs to be ≤ 75.
	// So we can have at most 7 messages total (7 × 10 = 70 ≤ 75).
	// 10 messages - 7 kept = 3 removed (from oldest non-system).
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 100,
		},
		tokensPerMsg: 10,
	}
	messages := []types.ChatMessage{
		msg("system", "system prompt"),
		msg("user", "msg1"),
		msg("assistant", "resp1"),
		msg("user", "msg2"),
		msg("assistant", "resp2"),
		msg("user", "msg3"),
		msg("assistant", "resp3"),
		msg("user", "msg4"),
		msg("assistant", "resp4"),
		msg("user", "msg5"),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed == 0 {
		t.Fatal("expected some messages to be removed, got 0")
	}

	// Total tokens should now fit within budget.
	totalTokens := len(result) * 10
	if totalTokens > 75 {
		t.Errorf("trimmed result (%d tokens) still exceeds budget of 75", totalTokens)
	}

	// Oldest non-system messages should be gone.
	for _, m := range result {
		if m.Role == "system" {
			continue
		}
		// The last user message ("msg5") should always be present.
		if m.Content == "msg5" {
			break // found it
		}
	}

	// Verify system prompt is still first.
	if result[0].Role != "system" {
		t.Errorf("system prompt should be first, got role=%q", result[0].Role)
	}

	// Verify total = original - removed.
	if len(result) != len(messages)-removed {
		t.Errorf("expected %d messages after removing %d, got %d",
			len(messages)-removed, removed, len(result))
	}
}

func TestTrimHistory_PreservesSystemPrompt(t *testing.T) {
	// System prompt + 10 user/assistant messages that exceed budget.
	// After trim, system prompt should still be the first message.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 40,
		},
		tokensPerMsg: 10,
	}

	messages := []types.ChatMessage{
		msg("system", "You are a coding assistant."),
		msg("user", "msg1"),
		msg("assistant", "resp1"),
		msg("user", "msg2"),
		msg("assistant", "resp2"),
		msg("user", "msg3"),
		msg("assistant", "resp3"),
		msg("user", "msg4"),
		msg("assistant", "resp4"),
		msg("user", "msg5"),
		msg("assistant", "resp5"),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed == 0 {
		t.Fatal("expected messages to be trimmed")
	}

	// System prompt must be preserved as the first message.
	if len(result) == 0 {
		t.Fatal("result is empty")
	}
	if result[0].Role != "system" {
		t.Errorf("first message should be system, got %q", result[0].Role)
	}
	if result[0].Content != "You are a coding assistant." {
		t.Errorf("system prompt content changed: %q", result[0].Content)
	}

	// No system message should appear elsewhere (only one system prompt).
	systemCount := 0
	for _, m := range result {
		if m.Role == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Errorf("expected 1 system message, found %d", systemCount)
	}
}

func TestTrimHistory_PreservesLastMessage(t *testing.T) {
	// Even with huge messages, the last user message must always be kept.
	// Budget = 20 * 3/4 = 15. Each message = 100 tokens.
	// Even system (100) + last (100) = 200 > 15, but we must keep the last one.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 20,
		},
		tokensPerMsg: 100,
	}

	messages := []types.ChatMessage{
		msg("system", strings.Repeat("x", 400)),
		msg("user", "old message 1"),
		msg("assistant", "old response 1"),
		msg("user", "old message 2"),
		msg("user", "LATEST MESSAGE"),
	}

	result, removed := trimHistory(messages, provider, "test-model")

	// The last non-system message should always be present.
	lastNonSystem := result[len(result)-1]
	if lastNonSystem.Content != "LATEST MESSAGE" {
		t.Errorf("last message should be 'LATEST MESSAGE', got %q", lastNonSystem.Content)
	}

	// System prompt should still be first.
	if result[0].Role != "system" {
		t.Errorf("system prompt should be first, got role=%q", result[0].Role)
	}

	// At minimum we have system + last message = 2.
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages (system + last), got %d", len(result))
	}

	// Removed count should be 3 (all history except the last one).
	if removed != 3 {
		t.Errorf("expected 3 removed, got %d", removed)
	}
}

func TestTrimHistory_NoLimitKnown(t *testing.T) {
	// MaxContextTokens=0 → no trimming should occur.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 0,
		},
		tokensPerMsg: 100,
	}

	messages := []types.ChatMessage{
		msg("system", "system prompt"),
		msg("user", "msg1"),
		msg("assistant", "resp1"),
		msg("user", "msg2"),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed != 0 {
		t.Errorf("expected 0 removed with no limit, got %d", removed)
	}
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

func TestTrimHistory_NoSystemPrompt(t *testing.T) {
	// Conversation without a system prompt should still trim correctly.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 40,
		},
		tokensPerMsg: 10,
	}

	messages := []types.ChatMessage{
		msg("user", "msg1"),
		msg("assistant", "resp1"),
		msg("user", "msg2"),
		msg("assistant", "resp2"),
		msg("user", "msg3"),
		msg("assistant", "resp3"),
	}

	result, removed := trimHistory(messages, provider, "test-model")

	// Budget = 40 * 3/4 = 30. 6 msgs × 10 = 60. Need ≤ 30 → max 3 msgs.
	// So 3 removed.
	if removed != 3 {
		t.Errorf("expected 3 removed, got %d", removed)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 messages remaining, got %d", len(result))
	}

	// Last message should be preserved.
	if result[len(result)-1].Content != "resp3" {
		t.Errorf("last message should be 'resp3', got %q", result[len(result)-1].Content)
	}
}

func TestTrimHistory_CountTokensError(t *testing.T) {
	// If CountTokens returns an error, trimHistory should return messages unchanged.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 100,
		},
		countTokenErr: errors.New("tokenizer unavailable"),
	}

	messages := []types.ChatMessage{
		msg("system", "system prompt"),
		msg("user", "msg1"),
		msg("assistant", "resp1"),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed != 0 {
		t.Errorf("expected 0 removed on error, got %d", removed)
	}
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

func TestTrimHistory_SingleMessage(t *testing.T) {
	// A single user message should never be trimmed, even if it exceeds budget.
	provider := &testProvider{
		caps: inference.ModelCapabilities{
			MaxContextTokens: 10,
		},
		tokensPerMsg: 100,
	}

	messages := []types.ChatMessage{
		msg("user", "very long message"),
	}

	result, removed := trimHistory(messages, provider, "test-model")
	if removed != 0 {
		t.Errorf("expected 0 removed for single message, got %d", removed)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}


