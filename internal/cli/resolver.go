package cli

import (
	"context"

	"cortex/internal/inference"
	"cortex/pkg/types"
)

// ModelResolver abstracts provider registry operations needed by CLI commands.
// Concrete implementation: *inference.ProviderRegistry.
type ModelResolver interface {
	Resolve(model string) (inference.InferenceProvider, string, error)
	ListAllModels(ctx context.Context) []types.ModelInfo
}
