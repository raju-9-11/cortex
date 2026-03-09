package inference

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"cortex/pkg/types"
)

// ProviderRegistry manages inference providers and resolves model names to the
// appropriate provider. All methods are safe for concurrent use.
type ProviderRegistry struct {
	mu              sync.RWMutex
	providers       map[string]InferenceProvider
	modelMap        map[string]string // model ID → provider name
	defaultProvider string
}

// NewProviderRegistry creates an empty, ready-to-use registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]InferenceProvider),
		modelMap:  make(map[string]string),
	}
}

// Register adds a provider to the registry. The first provider registered
// automatically becomes the default.
func (r *ProviderRegistry) Register(provider InferenceProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := provider.Name()
	r.providers[name] = provider

	if r.defaultProvider == "" {
		r.defaultProvider = name
	}
}

// SetDefault overrides the default provider used when no specific provider can
// be resolved for a model.
func (r *ProviderRegistry) SetDefault(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.defaultProvider = name
}

// RefreshModelMap queries every registered provider's ListModels and rebuilds
// the model→provider lookup table. Providers that return errors are logged as
// warnings but do not cause the overall refresh to fail.
func (r *ProviderRegistry) RefreshModelMap(ctx context.Context) error {
	r.mu.RLock()
	// Snapshot the providers so we can release the lock while querying.
	providersCopy := make(map[string]InferenceProvider, len(r.providers))
	for k, v := range r.providers {
		providersCopy[k] = v
	}
	r.mu.RUnlock()

	newMap := make(map[string]string)

	for name, provider := range providersCopy {
		models, err := provider.ListModels(ctx)
		if err != nil {
			log.Printf("WARNING: failed to list models for provider %q: %v", name, err)
			continue
		}
		for _, m := range models {
			newMap[m.ID] = name
		}
	}

	r.mu.Lock()
	r.modelMap = newMap
	r.mu.Unlock()

	log.Printf("Model map refreshed: %d models across providers", len(newMap))
	return nil
}

// Resolve determines which provider should handle a given model string.
//
// Resolution order:
//  1. "provider/model" prefix syntax — e.g. "ollama/qwen2.5:0.5b".
//  2. Exact match in the model map (populated by RefreshModelMap).
//  3. Default provider (model name passed through unchanged).
//  4. Error if nothing matches.
//
// Returns the provider, the resolved model name (prefix stripped if applicable),
// and any error.
func (r *ProviderRegistry) Resolve(model string) (InferenceProvider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Check for "provider/model" prefix syntax.
	if idx := strings.Index(model, "/"); idx > 0 {
		prefix := model[:idx]
		remainder := model[idx+1:]
		if p, ok := r.providers[prefix]; ok {
			return p, remainder, nil
		}
	}

	// 2. Exact match in modelMap.
	if providerName, ok := r.modelMap[model]; ok {
		if p, exists := r.providers[providerName]; exists {
			return p, model, nil
		}
	}

	// 3. Default provider.
	if r.defaultProvider != "" {
		if p, ok := r.providers[r.defaultProvider]; ok {
			return p, model, nil
		}
	}

	// 4. Nothing matched.
	return nil, "", fmt.Errorf("no provider found for model %q", model)
}

// ListAllModels aggregates ModelInfo from every registered provider. Providers
// that fail are logged as warnings and skipped. The returned slice is never nil.
func (r *ProviderRegistry) ListAllModels(ctx context.Context) []types.ModelInfo {
	r.mu.RLock()
	providersCopy := make(map[string]InferenceProvider, len(r.providers))
	for k, v := range r.providers {
		providersCopy[k] = v
	}
	r.mu.RUnlock()

	all := make([]types.ModelInfo, 0)

	for name, provider := range providersCopy {
		models, err := provider.ListModels(ctx)
		if err != nil {
			log.Printf("WARNING: failed to list models for provider %q: %v", name, err)
			continue
		}
		all = append(all, models...)
	}

	return all
}

// Providers returns a snapshot copy of the registered providers map.
func (r *ProviderRegistry) Providers() map[string]InferenceProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := make(map[string]InferenceProvider, len(r.providers))
	for k, v := range r.providers {
		snapshot[k] = v
	}
	return snapshot
}
