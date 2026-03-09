package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cortex/internal/inference"
	"cortex/pkg/types"
)

func setupTestRegistry() *inference.ProviderRegistry {
	registry := inference.NewProviderRegistry()
	registry.Register(inference.NewMockProvider("ollama", []string{"Hi"}))
	registry.Register(inference.NewMockProvider("openai", []string{"Hi"}))
	return registry
}

func TestRunModels_WithModels(t *testing.T) {
	registry := setupTestRegistry()
	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "", "ollama", "mock-model", false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Should contain header
	if !strings.Contains(output, "PROVIDER") {
		t.Error("expected table header with PROVIDER")
	}
	if !strings.Contains(output, "MODEL") {
		t.Error("expected table header with MODEL")
	}

	// Should contain both providers
	if !strings.Contains(output, "ollama") {
		t.Error("expected ollama in output")
	}
	if !strings.Contains(output, "openai") {
		t.Error("expected openai in output")
	}

	// Should contain mock-model
	if !strings.Contains(output, "mock-model") {
		t.Error("expected mock-model in output")
	}

	// Should contain footer
	if !strings.Contains(output, "2 provider(s), 2 model(s) available") {
		t.Errorf("expected footer with correct counts, got: %s", output)
	}
	if !strings.Contains(output, "Default: ollama / mock-model") {
		t.Errorf("expected default line, got: %s", output)
	}
}

func TestRunModels_Empty(t *testing.T) {
	registry := inference.NewProviderRegistry()
	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "", "", "", false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No models available") {
		t.Errorf("expected 'No models available' message, got: %s", output)
	}
}

func TestRunModels_FilterProvider(t *testing.T) {
	registry := setupTestRegistry()
	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "ollama", "ollama", "mock-model", false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "ollama") {
		t.Error("expected ollama in filtered output")
	}
	if strings.Contains(output, "openai") {
		t.Error("did not expect openai in filtered output")
	}
	if !strings.Contains(output, "1 provider(s), 1 model(s) available") {
		t.Errorf("expected 1 provider and 1 model in footer, got: %s", output)
	}
}

func TestRunModels_FilterProvider_NoMatch(t *testing.T) {
	registry := setupTestRegistry()
	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "nonexistent", "", "", false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No models available") {
		t.Errorf("expected 'No models available' for non-existent provider, got: %s", output)
	}
}

func TestRunModels_JSON(t *testing.T) {
	registry := setupTestRegistry()
	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "", "ollama", "mock-model", true, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var models []types.ModelInfo
	if err := json.Unmarshal(buf.Bytes(), &models); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if len(models) != 2 {
		t.Errorf("expected 2 models in JSON output, got %d", len(models))
	}

	// Verify models have expected fields
	for _, m := range models {
		if m.ID == "" {
			t.Error("expected non-empty model ID in JSON output")
		}
		if m.Provider == "" {
			t.Error("expected non-empty provider in JSON output")
		}
	}
}

func TestRunModels_Sorted(t *testing.T) {
	registry := inference.NewProviderRegistry()
	// Register in reverse alphabetical order
	registry.Register(inference.NewMockProvider("zeta", []string{"Hi"}))
	registry.Register(inference.NewMockProvider("alpha", []string{"Hi"}))
	registry.Register(inference.NewMockProvider("beta", []string{"Hi"}))

	var buf bytes.Buffer

	err := RunModels(context.Background(), registry, "", "alpha", "mock-model", false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")

	// Find the data lines (skip header, skip empty/footer lines)
	var dataLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "PROVIDER") ||
			strings.Contains(trimmed, "provider(s)") || strings.HasPrefix(trimmed, "Default:") {
			continue
		}
		dataLines = append(dataLines, trimmed)
	}

	if len(dataLines) != 3 {
		t.Fatalf("expected 3 data lines, got %d: %v", len(dataLines), dataLines)
	}

	// Verify alphabetical order: alpha, beta, zeta
	expectedOrder := []string{"alpha", "beta", "zeta"}
	for i, expected := range expectedOrder {
		if !strings.HasPrefix(dataLines[i], expected) {
			t.Errorf("line %d: expected provider %q, got %q", i, expected, dataLines[i])
		}
	}
}
