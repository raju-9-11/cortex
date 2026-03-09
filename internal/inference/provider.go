package inference

import (
	"context"

	"cortex/pkg/types"
)

type InferenceProvider interface {
	// StreamChat sends a chat completion request and streams events to the channel.
	// The provider MUST close the channel when done (or on error).
	// The provider MUST respect ctx cancellation.
	StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error

	// Complete is the non-streaming variant
	Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error)

	// CountTokens returns the token count for the given messages
	CountTokens(messages []types.ChatMessage) (int, error)

	// Capabilities returns what this provider/model combination supports.
	Capabilities(model string) ModelCapabilities

	// ListModels returns available models from this provider.
	ListModels(ctx context.Context) ([]types.ModelInfo, error)

	// Name returns the provider identifier
	Name() string

	// Close releases any resources held by the provider (e.g. background processes).
	Close() error
}

type ModelCapabilities struct {
	MaxContextTokens    int
	MaxOutputTokens     int
	DefaultOutputTokens int
	SupportsTools       bool
	SupportsVision      bool
	SupportsJSON        bool
	SupportsStreaming   bool
	TokenizerID         string
	ProviderID          string
}

// DefaultCapabilities returns conservative defaults
var DefaultCapabilities = ModelCapabilities{
	MaxContextTokens:    8192,
	MaxOutputTokens:     2048,
	DefaultOutputTokens: 1024,
	SupportsTools:       false,
	SupportsVision:      false,
	SupportsJSON:        false,
	SupportsStreaming:   true,
}
