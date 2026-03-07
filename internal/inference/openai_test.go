package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forge/pkg/types"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// sseLines builds a slice of raw SSE "data: ..." lines (including trailing
// blank lines) that the mock server writes verbatim.
func sseLines(chunks ...string) []string {
	var lines []string
	for _, c := range chunks {
		lines = append(lines, "data: "+c+"\n\n")
	}
	lines = append(lines, "data: [DONE]\n\n")
	return lines
}

// mustJSON marshals v or panics — only for tests.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// newTestOpenAIProvider creates an OpenAIProvider pointing at the given
// httptest server, stripping the default heavy Transport so the test server
// is reachable.
func newTestOpenAIProvider(url string) *OpenAIProvider {
	return &OpenAIProvider{
		client:       http.DefaultClient,
		baseURL:      strings.TrimSuffix(url, "/"),
		apiKey:       "test-key",
		providerName: "test-openai",
	}
}

// ---------------------------------------------------------------------------
// StreamChat — happy path
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_HappyPath(t *testing.T) {
	t.Parallel()

	hello := "Hello"
	world := " world"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []types.ChatCompletionChunk{
			{ID: "1", Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Content: &hello}}}},
			{ID: "2", Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Content: &world}}}},
			{ID: "3", Choices: []types.ChunkChoice{{Index: 0, FinishReason: "stop"}}},
		}

		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(c))
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)
	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []types.StreamEvent
	for ev := range out {
		events = append(events, ev)
	}

	// Expect: content delta "Hello", content delta " world", content done.
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != types.EventContentDelta || events[0].Delta != "Hello" {
		t.Errorf("event 0: want content.delta 'Hello', got %s %q", events[0].Type, events[0].Delta)
	}
	if events[1].Type != types.EventContentDelta || events[1].Delta != " world" {
		t.Errorf("event 1: want content.delta ' world', got %s %q", events[1].Type, events[1].Delta)
	}
	if events[2].Type != types.EventContentDone || events[2].FinishReason != "stop" {
		t.Errorf("event 2: want content.done/stop, got %s/%s", events[2].Type, events[2].FinishReason)
	}
}

// ---------------------------------------------------------------------------
// StreamChat — tool calls
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_ToolCalls(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Chunk 1: tool call start (name)
		c1 := types.ChatCompletionChunk{
			ID: "tc1",
			Choices: []types.ChunkChoice{{
				Index: 0,
				Delta: types.Delta{
					ToolCalls: []types.ToolCall{{
						ID:   "call_abc",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: "",
						},
					}},
				},
			}},
		}
		// Chunk 2: tool call argument delta
		c2 := types.ChatCompletionChunk{
			ID: "tc2",
			Choices: []types.ChunkChoice{{
				Index: 0,
				Delta: types.Delta{
					ToolCalls: []types.ToolCall{{
						ID:   "call_abc",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "",
							Arguments: `{"city":"NYC"}`,
						},
					}},
				},
			}},
		}
		// Chunk 3: finish
		c3 := types.ChatCompletionChunk{
			ID: "tc3",
			Choices: []types.ChunkChoice{{
				Index:        0,
				FinishReason: "tool_calls",
			}},
		}

		for _, c := range []types.ChatCompletionChunk{c1, c2, c3} {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(c))
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)
	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "weather?"}},
	}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []types.StreamEvent
	for ev := range out {
		events = append(events, ev)
	}

	// Expect: tool.start, tool.progress (args), tool.complete
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(events), events)
	}

	if events[0].Type != types.EventToolCallStart {
		t.Errorf("event 0: want tool.start, got %s", events[0].Type)
	}
	if events[0].ToolCall == nil || events[0].ToolCall.Name != "get_weather" {
		t.Errorf("event 0: expected tool name 'get_weather', got %+v", events[0].ToolCall)
	}

	if events[1].Type != types.EventToolCallDelta {
		t.Errorf("event 1: want tool.progress, got %s", events[1].Type)
	}
	if events[1].ToolCall == nil || events[1].ToolCall.Arguments != `{"city":"NYC"}` {
		t.Errorf("event 1: expected args, got %+v", events[1].ToolCall)
	}

	if events[2].Type != types.EventToolCallComplete {
		t.Errorf("event 2: want tool.complete, got %s", events[2].Type)
	}
	if events[2].FinishReason != "tool_calls" {
		t.Errorf("event 2: expected finish_reason 'tool_calls', got %q", events[2].FinishReason)
	}
}

// ---------------------------------------------------------------------------
// StreamChat — server error
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)
	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}, out)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected '500' in error, got: %v", err)
	}

	// Channel should contain an error event and then be closed.
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

// ---------------------------------------------------------------------------
// StreamChat — malformed SSE chunks
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_MalformedSSE(t *testing.T) {
	t.Parallel()

	hello := "Hello"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Valid chunk, then garbage, then another valid chunk.
		good := types.ChatCompletionChunk{
			ID:      "1",
			Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Content: &hello}}},
		}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(good))
		fmt.Fprint(w, "data: {{{invalid-json}}}\n\n")
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(types.ChatCompletionChunk{
			ID:      "3",
			Choices: []types.ChunkChoice{{Index: 0, FinishReason: "stop"}},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)
	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []types.StreamEvent
	for ev := range out {
		events = append(events, ev)
	}

	// Malformed chunk should be silently skipped: we get content delta + content done.
	if len(events) != 2 {
		t.Fatalf("expected 2 events (malformed skipped), got %d: %+v", len(events), events)
	}
	if events[0].Type != types.EventContentDelta {
		t.Errorf("event 0: expected content.delta, got %s", events[0].Type)
	}
	if events[1].Type != types.EventContentDone {
		t.Errorf("event 1: expected content.done, got %s", events[1].Type)
	}
}

// ---------------------------------------------------------------------------
// StreamChat — context cancellation
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_ContextCancellation(t *testing.T) {
	t.Parallel()

	hello := "Hi"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunk := types.ChatCompletionChunk{
			ID:      "1",
			Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Content: &hello}}},
		}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(chunk))
		if flusher != nil {
			flusher.Flush()
		}
		// Block until the client goes away.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)

	done := make(chan error, 1)
	go func() {
		done <- p.StreamChat(ctx, &types.ChatCompletionRequest{
			Model:    "gpt-4o",
			Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
		}, out)
	}()

	// Read the first event.
	ev := <-out
	if ev.Type != types.EventContentDelta {
		t.Errorf("expected content delta, got %s", ev.Type)
	}

	// Cancel and wait for StreamChat to exit.
	cancel()

	err := <-done
	if err != nil && err != context.Canceled {
		t.Errorf("expected nil or context.Canceled, got %v", err)
	}

	// Drain remaining events — channel must close.
	for range out {
	}
}

// ---------------------------------------------------------------------------
// Complete — happy path
// ---------------------------------------------------------------------------

func TestOpenAI_Complete_HappyPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req types.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Stream {
			t.Error("expected stream=false for Complete")
		}

		resp := types.ChatCompletionResponse{
			ID:    "cmpl-1",
			Model: req.Model,
			Choices: []types.Choice{{
				Index:        0,
				Message:      types.ChatMessage{Role: "assistant", Content: "I can help!"},
				FinishReason: "stop",
			}},
			Usage: &types.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	resp, err := p.Complete(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "Help"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "I can help!" {
		t.Errorf("unexpected content: %v", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 8 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

// ---------------------------------------------------------------------------
// Complete — server error
// ---------------------------------------------------------------------------

func TestOpenAI_Complete_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid model"}}`)
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	_, err := p.Complete(context.Background(), &types.ChatCompletionRequest{
		Model:    "nonexistent",
		Messages: []types.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected '400' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

func TestOpenAI_ListModels(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := types.ModelListResponse{
			Object: "list",
			Data: []types.ModelInfo{
				{ID: "gpt-4o", Object: "model", OwnedBy: "openai"},
				{ID: "gpt-3.5-turbo", Object: "model", OwnedBy: "openai"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	// Provider field should be populated.
	for _, m := range models {
		if m.Provider != "test-openai" {
			t.Errorf("expected provider 'test-openai', got %q", m.Provider)
		}
	}
	if models[0].ID != "gpt-4o" {
		t.Errorf("expected first model 'gpt-4o', got %s", models[0].ID)
	}
}

// ---------------------------------------------------------------------------
// CountTokens
// ---------------------------------------------------------------------------

func TestOpenAI_CountTokens(t *testing.T) {
	t.Parallel()

	p := newTestOpenAIProvider("http://unused")

	tests := []struct {
		name     string
		messages []types.ChatMessage
		wantMin  int
	}{
		{
			name:     "empty messages",
			messages: nil,
			wantMin:  1, // minimum clamp
		},
		{
			name: "single short message",
			messages: []types.ChatMessage{
				{Role: "user", Content: "Hello, world!"},
			},
			// 2 (conv overhead) + 4 (msg overhead) + 13/4=3 = 9
			wantMin: 9,
		},
		{
			name: "message with tool calls",
			messages: []types.ChatMessage{
				{
					Role:    "assistant",
					Content: "Sure",
					ToolCalls: []types.ToolCall{
						{
							ID:   "tc1",
							Type: "function",
							Function: types.FunctionCall{
								Name:      "read_file",
								Arguments: `{"path":"/tmp/x"}`,
							},
						},
					},
				},
			},
			wantMin: 10, // must be non-trivial
		},
		{
			name: "non-string content (map)",
			messages: []types.ChatMessage{
				{Role: "user", Content: map[string]string{"type": "text", "text": "hi"}},
			},
			wantMin: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := p.CountTokens(tt.messages)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count < tt.wantMin {
				t.Errorf("expected count >= %d, got %d", tt.wantMin, count)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Name & Capabilities
// ---------------------------------------------------------------------------

func TestOpenAI_NameAndCapabilities(t *testing.T) {
	t.Parallel()

	p := NewOpenAIProvider("my-provider", "http://example.com", "sk-xxx")
	if p.Name() != "my-provider" {
		t.Errorf("expected name 'my-provider', got %q", p.Name())
	}

	caps := p.Capabilities("gpt-4o")
	if caps != DefaultCapabilities {
		t.Errorf("expected DefaultCapabilities, got %+v", caps)
	}
}

// ---------------------------------------------------------------------------
// StreamChat — non-SSE lines are skipped
// ---------------------------------------------------------------------------

func TestOpenAI_StreamChat_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	content := "OK"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Write some comment and event-type lines that are NOT "data:" prefixed.
		fmt.Fprint(w, ": this is a comment\n\n")
		fmt.Fprint(w, "event: ping\n\n")
		// Then a valid data line.
		chunk := types.ChatCompletionChunk{
			ID:      "1",
			Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Content: &content}}},
		}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(chunk))
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(types.ChatCompletionChunk{
			ID:      "2",
			Choices: []types.ChunkChoice{{Index: 0, FinishReason: "stop"}},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	out := make(chan types.StreamEvent, 10)
	err := p.StreamChat(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "test"}},
	}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []types.StreamEvent
	for ev := range out {
		events = append(events, ev)
	}
	// Should only see the content delta + content done, comment and event: lines ignored.
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
}

// ---------------------------------------------------------------------------
// Ensure newTestOpenAIProvider doesn't leak timeout (quick construction test)
// ---------------------------------------------------------------------------

func TestOpenAI_NewOpenAIProvider_URLTrimming(t *testing.T) {
	t.Parallel()

	p := NewOpenAIProvider("p", "http://localhost:8080/", "key")
	if p.baseURL != "http://localhost:8080" {
		t.Errorf("expected trailing slash trimmed, got %q", p.baseURL)
	}
}

// ---------------------------------------------------------------------------
// ListModels — server error
// ---------------------------------------------------------------------------

func TestOpenAI_ListModels_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "forbidden")
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv.URL)
	_, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected '403' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Complete — timeout via context
// ---------------------------------------------------------------------------

func TestOpenAI_Complete_ContextTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay long enough for the client context to expire, but not forever
		// so that srv.Close() doesn't hang.
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	p := newTestOpenAIProvider(srv.URL)
	_, err := p.Complete(ctx, &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
