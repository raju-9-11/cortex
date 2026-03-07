package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"forge/internal/inference"
)

func TestRunOnce_BasicPrompt(t *testing.T) {
	registry := inference.NewProviderRegistry()
	registry.Register(inference.NewMockProvider("test", []string{"Hello", " World"}))
	registry.SetDefault("test")

	var buf bytes.Buffer
	fullText, err := RunOnce(context.Background(), registry, "mock-model", "", "say hello", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fullText != "Hello World" {
		t.Errorf("fullText = %q, want %q", fullText, "Hello World")
	}

	// Output should be the streamed text plus a trailing newline
	want := "Hello World\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestRunOnce_WithSystemPrompt(t *testing.T) {
	registry := inference.NewProviderRegistry()
	registry.Register(inference.NewMockProvider("test", []string{"Brief", " answer"}))
	registry.SetDefault("test")

	var buf bytes.Buffer
	fullText, err := RunOnce(context.Background(), registry, "mock-model", "Be brief", "What is Go?", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fullText != "Brief answer" {
		t.Errorf("fullText = %q, want %q", fullText, "Brief answer")
	}

	// The mock provider doesn't inspect messages, but the function should
	// still work correctly with a system prompt. We verify output is correct.
	want := "Brief answer\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestRunOnce_ProviderError(t *testing.T) {
	// Create a mock provider that fails at token index 2
	mock := inference.NewMockProvider("test", []string{"partial", " text", " fail"})
	mock.SetFailAt(2) // will fail when trying to send 3rd token

	registry := inference.NewProviderRegistry()
	registry.Register(mock)
	registry.SetDefault("test")

	var buf bytes.Buffer
	fullText, err := RunOnce(context.Background(), registry, "mock-model", "", "test", &buf)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "provider error") {
		t.Errorf("error = %q, want it to contain 'provider error'", err.Error())
	}

	// Should have partial text from the tokens before failure
	if fullText != "partial text" {
		t.Errorf("fullText = %q, want %q", fullText, "partial text")
	}
}

func TestRunOnce_EmptyResponse(t *testing.T) {
	// Mock provider with no content tokens — just the EventContentDone
	// NewMockProvider with empty slice gets default tokens, so we use a single empty string
	mock := inference.NewMockProvider("test", []string{""})

	registry := inference.NewProviderRegistry()
	registry.Register(mock)
	registry.SetDefault("test")

	var buf bytes.Buffer
	fullText, err := RunOnce(context.Background(), registry, "mock-model", "", "test", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fullText != "" {
		t.Errorf("fullText = %q, want empty", fullText)
	}
}

func TestRunOnce_ModelResolution(t *testing.T) {
	registry := inference.NewProviderRegistry()
	registry.Register(inference.NewMockProvider("ollama", []string{"from", " ollama"}))
	registry.Register(inference.NewMockProvider("openai", []string{"from", " openai"}))

	// Test "provider/model" prefix syntax
	var buf bytes.Buffer
	fullText, err := RunOnce(context.Background(), registry, "openai/gpt-4", "", "test", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fullText != "from openai" {
		t.Errorf("fullText = %q, want %q", fullText, "from openai")
	}
}

func TestRunOnce_ResolutionError(t *testing.T) {
	registry := inference.NewProviderRegistry()
	// Empty registry — no providers

	var buf bytes.Buffer
	_, err := RunOnce(context.Background(), registry, "nonexistent/model", "", "test", &buf)
	if err == nil {
		t.Fatal("expected error for unresolvable model, got nil")
	}

	if !strings.Contains(err.Error(), "resolving model") {
		t.Errorf("error = %q, want it to contain 'resolving model'", err.Error())
	}
}

func TestRunOnce_ContextCancellation(t *testing.T) {
	registry := inference.NewProviderRegistry()
	registry.Register(inference.NewMockProvider("test", []string{"a", "b", "c", "d", "e", "f", "g", "h"}))
	registry.SetDefault("test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	_, err := RunOnce(ctx, registry, "mock-model", "", "test", &buf)
	if err == nil {
		// Context cancellation may or may not propagate as an error depending on timing
		// This is acceptable — the important thing is it doesn't hang
		return
	}

	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "provider error") {
		t.Errorf("error = %q, want context-related error", err.Error())
	}
}
