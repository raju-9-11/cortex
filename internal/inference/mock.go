package inference

import (
	"context"
	"errors"
	"time"

	"forge/pkg/types"
)

type MockProvider struct {
	tokens   []string
	delay    time.Duration
	failAt   int // -1 = never
	providerName string
}

func NewMockProvider(providerName string, tokens []string) *MockProvider {
	if len(tokens) == 0 {
		tokens = []string{"Hello", " ", "from", " ", providerName, "!"}
	}
	return &MockProvider{
		tokens:   tokens,
		delay:    10 * time.Millisecond,
		failAt:   -1,
		providerName: providerName,
	}
}

func (m *MockProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	defer close(out)
	for i, token := range m.tokens {
		if m.failAt == i {
			return errors.New("mock provider failure")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.delay):
			content := token
			out <- types.StreamEvent{
				Type:  types.EventContentDelta,
				Delta: content,
			}
		}
	}
	out <- types.StreamEvent{
		Type:         types.EventContentDone,
		FinishReason: "stop",
	}
	return nil
}

func (m *MockProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Not implemented for tests yet
	return nil, errors.New("not implemented")
}

func (m *MockProvider) CountTokens(messages []types.ChatMessage) (int, error) {
	return 10, nil
}

func (m *MockProvider) Capabilities(model string) ModelCapabilities {
	return DefaultCapabilities
}

func (m *MockProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	return []types.ModelInfo{
		{
			ID:       "mock-model",
			Provider: m.providerName,
		},
	}, nil
}

func (m *MockProvider) Name() string {
	return m.providerName
}

// SetFailAt configures the mock to return an error when trying to send the
// token at the given index. Use -1 (default) to never fail.
func (m *MockProvider) SetFailAt(index int) {
	m.failAt = index
}
