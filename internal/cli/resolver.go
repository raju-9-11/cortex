package cli

import (
	"context"

	"forge/internal/inference"
	"forge/pkg/types"
)

// ModelResolver abstracts provider registry operations needed by CLI commands.
// Concrete implementation: *inference.ProviderRegistry.
type ModelResolver interface {
	Resolve(model string) (inference.InferenceProvider, string, error)
	ListAllModels(ctx context.Context) []types.ModelInfo
}
