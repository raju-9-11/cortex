package inference

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"cortex/pkg/types"
)

// ---------------------------------------------------------------------------
// errMockProvider is a MockProvider variant whose ListModels returns an error.
// ---------------------------------------------------------------------------

type errMockProvider struct {
	MockProvider
}

func newErrMockProvider(name string) *errMockProvider {
	return &errMockProvider{MockProvider: *NewMockProvider(name, nil)}
}

func (e *errMockProvider) ListModels(_ context.Context) ([]types.ModelInfo, error) {
	return nil, errors.New("simulated list-models failure")
}

// ---------------------------------------------------------------------------
// multiModelMockProvider returns several models from ListModels.
// ---------------------------------------------------------------------------

type multiModelMockProvider struct {
	MockProvider
	models []types.ModelInfo
}

func newMultiModelMockProvider(name string, modelIDs ...string) *multiModelMockProvider {
	models := make([]types.ModelInfo, len(modelIDs))
	for i, id := range modelIDs {
		models[i] = types.ModelInfo{ID: id, Provider: name}
	}
	return &multiModelMockProvider{
		MockProvider: *NewMockProvider(name, nil),
		models:       models,
	}
}

func (m *multiModelMockProvider) ListModels(_ context.Context) ([]types.ModelInfo, error) {
	return m.models, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRegistry_RegisterAndResolveByPrefix(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	qwen := NewMockProvider("qwen", nil)
	reg.Register(qwen)

	p, model, err := reg.Resolve("qwen/model-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "qwen" {
		t.Errorf("expected provider 'qwen', got %q", p.Name())
	}
	if model != "model-name" {
		t.Errorf("expected model 'model-name', got %q", model)
	}
}

func TestRegistry_ResolveByModelMap(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	prov := newMultiModelMockProvider("openai", "gpt-4o", "gpt-3.5-turbo")
	reg.Register(prov)

	if err := reg.RefreshModelMap(context.Background()); err != nil {
		t.Fatalf("RefreshModelMap: %v", err)
	}

	p, model, err := reg.Resolve("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected provider 'openai', got %q", p.Name())
	}
	if model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", model)
	}
}

func TestRegistry_ResolveWithDefault(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	// First registered becomes default.
	defaultProv := NewMockProvider("default-prov", nil)
	reg.Register(defaultProv)

	p, model, err := reg.Resolve("unknown-model-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "default-prov" {
		t.Errorf("expected default provider, got %q", p.Name())
	}
	if model != "unknown-model-xyz" {
		t.Errorf("expected model name pass-through, got %q", model)
	}
}

func TestRegistry_ResolveUnknownModel_NoProviders(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	_, _, err := reg.Resolve("anything")
	if err == nil {
		t.Fatal("expected error for empty registry")
	}
}

func TestRegistry_RegisterDuplicateOverwrites(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	prov1 := NewMockProvider("samename", []string{"first"})
	prov2 := NewMockProvider("samename", []string{"second"})

	reg.Register(prov1)
	reg.Register(prov2)

	providers := reg.Providers()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	// Resolve by prefix and verify it's the second registration.
	p, _, err := reg.Resolve("samename/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both have same name, but they are different pointers.
	if p != prov2 {
		t.Error("expected the second registered provider to replace the first")
	}
}

func TestRegistry_ConcurrentRegisterAndResolve(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	// Seed with a default so Resolve never errors.
	reg.Register(NewMockProvider("default", nil))

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines register, half resolve.
	for i := 0; i < goroutines; i++ {
		name := fmt.Sprintf("prov-%d", i)
		go func(n string) {
			defer wg.Done()
			reg.Register(NewMockProvider(n, nil))
		}(name)

		go func(n string) {
			defer wg.Done()
			// May resolve via default or via prefix — either is fine.
			_, _, _ = reg.Resolve(n + "/some-model")
		}(name)
	}

	wg.Wait()

	// Sanity: at least the default provider should be present.
	providers := reg.Providers()
	if _, ok := providers["default"]; !ok {
		t.Error("expected 'default' provider to still be registered")
	}
}

func TestRegistry_ProvidersReturnsRegisteredNames(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	reg.Register(NewMockProvider("alpha", nil))
	reg.Register(NewMockProvider("beta", nil))
	reg.Register(NewMockProvider("gamma", nil))

	providers := reg.Providers()
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, ok := providers[name]; !ok {
			t.Errorf("expected provider %q in map", name)
		}
	}
}

func TestRegistry_SetDefaultNonExistent(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	reg.Register(NewMockProvider("real", nil))

	// SetDefault to a name that was never registered.
	reg.SetDefault("ghost")

	// Resolve a bare model: default provider "ghost" doesn't exist in
	// providers map, so step 3 fails and we get an error.
	_, _, err := reg.Resolve("some-model")
	if err == nil {
		t.Fatal("expected error when default provider does not exist in the map")
	}
}

func TestRegistry_ListAllModelsMultipleProviders(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	reg.Register(newMultiModelMockProvider("provA", "m1", "m2"))
	reg.Register(newMultiModelMockProvider("provB", "m3"))

	models := reg.ListAllModels(context.Background())
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}
	for _, expected := range []string{"m1", "m2", "m3"} {
		if !ids[expected] {
			t.Errorf("expected model %q in aggregated list", expected)
		}
	}
}

func TestRegistry_RefreshModelMapWithProviderError(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	good := newMultiModelMockProvider("good", "model-a")
	bad := newErrMockProvider("bad")
	reg.Register(good)
	reg.Register(bad)

	// RefreshModelMap should succeed even when one provider errors.
	if err := reg.RefreshModelMap(context.Background()); err != nil {
		t.Fatalf("RefreshModelMap should not fail: %v", err)
	}

	// The good provider's model should still be resolvable.
	p, _, err := reg.Resolve("model-a")
	if err != nil {
		t.Fatalf("unexpected error resolving model-a: %v", err)
	}
	if p.Name() != "good" {
		t.Errorf("expected provider 'good', got %q", p.Name())
	}
}
