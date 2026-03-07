package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/pkg/types"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewOllamaProvider_DefaultURL(t *testing.T) {
	p := NewOllamaProvider("")
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("expected default URL http://localhost:11434, got %s", p.baseURL)
	}
}

func TestNewOllamaProvider_CustomURL(t *testing.T) {
	p := NewOllamaProvider("http://myhost:9999/")
	if p.baseURL != "http://myhost:9999" {
		t.Errorf("expected trailing slash trimmed, got %s", p.baseURL)
	}
}

func TestOllamaProvider_Name(t *testing.T) {
	p := NewOllamaProvider("")
	if p.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got %s", p.Name())
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestOllamaProvider_Capabilities(t *testing.T) {
	p := NewOllamaProvider("")
	caps := p.Capabilities("llama3.2:1b")
	if caps.MaxContextTokens != 4096 {
		t.Errorf("expected MaxContextTokens 4096, got %d", caps.MaxContextTokens)
	}
	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming true")
	}
	if caps.ProviderID != "ollama" {
		t.Errorf("expected ProviderID 'ollama', got %s", caps.ProviderID)
	}
}

// ---------------------------------------------------------------------------
// CountTokens
// ---------------------------------------------------------------------------

func TestOllamaProvider_CountTokens(t *testing.T) {
	p := NewOllamaProvider("")
	msgs := []types.ChatMessage{
		{Role: "user", Content: "Hello, world!"},   // 13 chars → 13/4 = 3, +4 overhead = 7
		{Role: "assistant", Content: "Hi there!!!"}, // 11 chars → 11/4 = 2, +4 overhead = 6
	}
	count, err := p.CountTokens(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 (conversation overhead) + 7 + 6 = 15
	if count != 15 {
		t.Errorf("expected 15, got %d", count)
	}
}

func TestOllamaProvider_CountTokens_NilContent(t *testing.T) {
	p := NewOllamaProvider("")
	msgs := []types.ChatMessage{
		{Role: "user", Content: nil},
	}
	count, err := p.CountTokens(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 (conversation) + 0/4 + 4 = 6
	if count != 6 {
		t.Errorf("expected 6, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

func TestOllamaProvider_ListModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := ollamaTagsResponse{
			Models: []ollamaModelEntry{
				{Name: "llama3:latest"},
				{Name: "mistral:7b"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "llama3:latest" {
		t.Errorf("expected first model 'llama3:latest', got %s", models[0].ID)
	}
	if models[0].Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got %s", models[0].Provider)
	}
	if models[1].ID != "mistral:7b" {
		t.Errorf("expected second model 'mistral:7b', got %s", models[1].ID)
	}
}

func TestOllamaProvider_ListModels_Unreachable(t *testing.T) {
	// Point at a port that's not listening.
	p := NewOllamaProvider("http://127.0.0.1:1")
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected no error for unreachable Ollama, got: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected empty list, got %d models", len(models))
	}
}

// ---------------------------------------------------------------------------
// Complete (non-streaming)
// ---------------------------------------------------------------------------

func TestOllamaProvider_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Decode and verify request format.
		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Stream {
			t.Error("expected stream=false for Complete")
		}
		if req.Model != "llama3.2:1b" {
			t.Errorf("expected model 'llama3.2:1b', got %s", req.Model)
		}

		resp := ollamaChatResponse{
			Model: "llama3.2:1b",
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "Hello! How can I help?",
			},
			Done:            true,
			DoneReason:      "stop",
			PromptEvalCount: 10,
			EvalCount:       8,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	temp := 0.7
	resp, err := p.Complete(context.Background(), &types.ChatCompletionRequest{
		Model: "llama3.2:1b",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "llama3.2:1b" {
		t.Errorf("expected model 'llama3.2:1b', got %s", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello! How can I help?" {
		t.Errorf("unexpected content: %v", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be present")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt_tokens=10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 8 {
		t.Errorf("expected completion_tokens=8, got %d", resp.Usage.CompletionTokens)
	}
}

func TestOllamaProvider_Complete_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "model not found")
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	_, err := p.Complete(context.Background(), &types.ChatCompletionRequest{
		Model:    "nonexistent",
		Messages: []types.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to contain '400', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// StreamChat (NDJSON)
// ---------------------------------------------------------------------------

func TestOllamaProvider_StreamChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Decode and verify streaming flag.
		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if !req.Stream {
			t.Error("expected stream=true for StreamChat")
		}

		// Send NDJSON lines.
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)

		chunks := []ollamaChatResponse{
			{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: "Hello"}, Done: false},
			{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: " world"}, Done: false},
			{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: "!"}, Done: false},
			{Model: "llama3", Done: true, DoneReason: "stop", PromptEvalCount: 5, EvalCount: 3},
		}

		for _, chunk := range chunks {
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "%s\n", b)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)

	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "llama3",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect all events.
	var events []types.StreamEvent
	for ev := range out {
		events = append(events, ev)
	}

	// Expect 3 deltas + 1 done = 4 events.
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}

	// Verify deltas.
	expectedDeltas := []string{"Hello", " world", "!"}
	for i, expected := range expectedDeltas {
		if events[i].Type != types.EventContentDelta {
			t.Errorf("event %d: expected type %s, got %s", i, types.EventContentDelta, events[i].Type)
		}
		if events[i].Delta != expected {
			t.Errorf("event %d: expected delta %q, got %q", i, expected, events[i].Delta)
		}
	}

	// Verify done event.
	doneEv := events[3]
	if doneEv.Type != types.EventContentDone {
		t.Errorf("expected done event type %s, got %s", types.EventContentDone, doneEv.Type)
	}
	if doneEv.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %s", doneEv.FinishReason)
	}
	if doneEv.Usage == nil {
		t.Fatal("expected usage in done event")
	}
	if doneEv.Usage.CompletionTokens != 3 {
		t.Errorf("expected eval_count=3, got %d", doneEv.Usage.CompletionTokens)
	}
}

func TestOllamaProvider_StreamChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)

	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "llama3",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}, out)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}

	// Drain channel — should contain an error event then be closed.
	var gotError bool
	for ev := range out {
		if ev.Type == types.EventError {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected an error event on the channel")
	}
}

func TestOllamaProvider_StreamChat_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send one chunk, then hang (context will be cancelled).
		w.Header().Set("Content-Type", "application/x-ndjson")
		chunk := ollamaChatResponse{
			Model:   "llama3",
			Message: ollamaMessage{Role: "assistant", Content: "Hi"},
			Done:    false,
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "%s\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Block to simulate slow server.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewOllamaProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)

	done := make(chan error, 1)
	go func() {
		done <- p.StreamChat(ctx, &types.ChatCompletionRequest{
			Model:    "llama3",
			Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
		}, out)
	}()

	// Read the first event.
	ev := <-out
	if ev.Type != types.EventContentDelta {
		t.Errorf("expected content delta, got %s", ev.Type)
	}

	// Cancel context.
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Probe
// ---------------------------------------------------------------------------

func TestOllamaProvider_Probe_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ollamaTagsResponse{})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	if !p.Probe(context.Background()) {
		t.Error("expected Probe to return true for reachable server")
	}
}

func TestOllamaProvider_Probe_Unreachable(t *testing.T) {
	p := NewOllamaProvider("http://127.0.0.1:1")
	if p.Probe(context.Background()) {
		t.Error("expected Probe to return false for unreachable server")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestTranslateFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "stop"},
		{"length", "length"},
		{"tool_calls", "tool_calls"},
		{"", "stop"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := translateFinishReason(tt.input)
		if got != tt.expected {
			t.Errorf("translateFinishReason(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStringifyContent(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"nil", nil, ""},
		{"map", map[string]string{"a": "b"}, `{"a":"b"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringifyContent(tt.input)
			if got != tt.expected {
				t.Errorf("stringifyContent(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTranslateRequest_Options(t *testing.T) {
	temp := 0.5
	topP := 0.9
	maxTok := 100
	req := &types.ChatCompletionRequest{
		Model: "test",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "hello"},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTok,
	}

	ollamaReq := translateRequest(req, true)
	if ollamaReq.Model != "test" {
		t.Errorf("expected model 'test', got %s", ollamaReq.Model)
	}
	if !ollamaReq.Stream {
		t.Error("expected stream=true")
	}
	if ollamaReq.Options == nil {
		t.Fatal("expected options to be set")
	}
	if *ollamaReq.Options.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", *ollamaReq.Options.Temperature)
	}
	if *ollamaReq.Options.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %v", *ollamaReq.Options.TopP)
	}
	if *ollamaReq.Options.NumPredict != 100 {
		t.Errorf("expected num_predict 100, got %v", *ollamaReq.Options.NumPredict)
	}
}

func TestTranslateRequest_NoOptions(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "test",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "hello"},
		},
	}

	ollamaReq := translateRequest(req, false)
	if ollamaReq.Options != nil {
		t.Error("expected nil options when none provided")
	}
	if ollamaReq.Stream {
		t.Error("expected stream=false")
	}
}
