# Forge LLM Engine — Implementation Plan

## Audit Summary

After reviewing all files in `internal/inference/`, `internal/streaming/`, `pkg/types/`, `internal/api/`, `internal/config/`, `internal/server/`, and `cmd/forge/main.go`, this plan covers 6 work areas with exact file paths, line numbers, and code.

---

## 1. Immediate Fixes to Existing Code

### 1.1 — CRITICAL: `json.Marshal` error silently swallowed (openai.go:82, 117)

**Location:** `internal/inference/openai.go`, lines 82 and 117

Both `Complete()` and `StreamChat()` ignore the marshal error:

```go
// BEFORE (line 82, and identically line 117)
body, _ := json.Marshal(req)
```

If `req` contains an unserializable `ToolChoice` (it's `any`), this silently sends `null` as the POST body, producing a confusing 400 from the upstream API.

```go
// AFTER
body, err := json.Marshal(req)
if err != nil {
    return nil, fmt.Errorf("marshal request: %w", err)
}
```

For `StreamChat` (line 117), the same fix but send an error event before returning:

```go
body, err := json.Marshal(req)
if err != nil {
    return fmt.Errorf("marshal request: %w", err)
}
```

### 1.2 — HIGH: StreamChat returns error AFTER closing channel — caller can't distinguish (openai.go:113-184)

**Location:** `internal/inference/openai.go`, `StreamChat()` method

The current contract says "provider MUST close the channel when done" (provider.go:12). The `defer close(out)` on line 114 is correct. But errors that happen **before any events are sent** (marshal failure, HTTP error on line 136-137) return an error after closing the channel. The SSE pipeline in `sse.go:74` does `return <-errCh`, but by that point the `range events` loop already exited with zero events written.

This means: on a 401/403 from the upstream API, the client gets `data: [DONE]` with **no error event** — just an empty completion.

**Fix:** Send an error event into the channel before returning the error:

```go
func (p *OpenAIProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
    defer close(out)

    req.Stream = true
    body, err := json.Marshal(req)
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return fmt.Errorf("marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return err
    }
    // ... headers ...

    resp, err := p.client.Do(httpReq)
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        respBody, _ := io.ReadAll(resp.Body)
        apiErr := fmt.Errorf("upstream HTTP %d: %s", resp.StatusCode, string(respBody))
        out <- types.StreamEvent{Type: types.EventError, Error: apiErr, ErrorMessage: apiErr.Error()}
        return apiErr
    }
    // ... SSE scan loop unchanged ...
}
```

### 1.3 — HIGH: Tool call deltas are completely ignored (openai.go:164-180)

**Location:** `internal/inference/openai.go`, lines 164-180

The stream loop only handles `choice.Delta.Content` and `choice.FinishReason`. When the model makes a tool call, `Delta.ToolCalls` is populated but **entirely dropped**. This means tool-use models silently produce empty responses.

**Fix:** Add tool call delta handling inside the `for scanner.Scan()` loop:

```go
if len(chunk.Choices) > 0 {
    choice := chunk.Choices[0]

    // Content deltas
    if choice.Delta.Content != nil {
        out <- types.StreamEvent{
            Type:  types.EventContentDelta,
            Delta: *choice.Delta.Content,
        }
    }

    // Tool call deltas
    for _, tc := range choice.Delta.ToolCalls {
        if tc.Function.Name != "" {
            // First chunk for this tool call — emit start event
            out <- types.StreamEvent{
                Type: types.EventToolCallStart,
                ToolCall: &types.ToolCallEvent{
                    ID:   tc.ID,
                    Name: tc.Function.Name,
                },
            }
        }
        if tc.Function.Arguments != "" {
            // Argument fragment — emit progress event
            out <- types.StreamEvent{
                Type: types.EventToolCallDelta,
                ToolCall: &types.ToolCallEvent{
                    ID:        tc.ID,
                    Arguments: tc.Function.Arguments,
                },
            }
        }
    }

    // Finish reason
    if choice.FinishReason != "" {
        eventType := types.EventContentDone
        if choice.FinishReason == "tool_calls" {
            eventType = types.EventToolCallComplete
        }
        out <- types.StreamEvent{
            Type:         eventType,
            FinishReason: choice.FinishReason,
        }
    }
}
```

### 1.4 — MEDIUM: Scanner silently drops malformed SSE chunks (openai.go:159-161)

**Location:** `internal/inference/openai.go`, lines 159-161

```go
if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
    // Skip bad JSON
    continue
}
```

This should at minimum log the error. In production, a corrupt chunk could indicate a proxy issue, and silently dropping it causes the response to appear truncated.

**Fix:**

```go
if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
    log.Printf("[WARN] OpenAI SSE: skipping malformed chunk: %v (data: %.100s)", err, dataStr)
    continue
}
```

### 1.5 — MEDIUM: `io.ReadAll` on error responses is unbounded (openai.go:58, 101, 136)

**Location:** `internal/inference/openai.go`, lines 58, 101, 136

```go
body, _ := io.ReadAll(resp.Body)
```

A misbehaving upstream could send gigabytes. Cap it:

```go
body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
```

Apply this at all three call sites.

---

## 2. Streaming Pipeline Fixes (`internal/streaming/sse.go`)

### 2.1 — CRITICAL: Replace deprecated `CloseNotifier` (sse.go:35-43)

**Location:** `internal/streaming/sse.go`, lines 35-43

`http.CloseNotifier` was deprecated in Go 1.11. Modern Go HTTP servers propagate client disconnects through `r.Context()` — which is already passed as `ctx` to `Pipeline.Stream()`.

**Delete the entire block:**

```go
// DELETE lines 35-43 entirely:
//  if cn, ok := w.(http.CloseNotifier); ok {
//      go func() {
//          select {
//          case <-cn.CloseNotify():
//              cancel()
//          case <-ctx.Done():
//          }
//      }()
//  }
```

The `ctx, cancel := context.WithCancel(ctx)` on line 33 already inherits from `r.Context()`, which is cancelled on client disconnect by Go's HTTP server since Go 1.8.

### 2.2 — HIGH: Error sent after headers are written (handlers_chat.go:43-44)

**Location:** `internal/api/handlers_chat.go`, lines 39-45

```go
if req.Stream {
    pipeline := streaming.NewPipeline(provider)
    err := pipeline.Stream(r.Context(), &req, w)
    if err != nil {
        // If already streaming, error is in SSE. Else:
        http.Error(w, err.Error(), http.StatusInternalServerError) // BUG
    }
    return
}
```

Once `Pipeline.Stream()` calls `w.Header().Set(...)` and writes the first `data:` line, calling `http.Error()` is a protocol violation — headers are already sent. The HTTP server will log a superfluous WriteHeader warning, and the client sees garbage appended to the SSE stream.

**Fix:** Track whether headers have been sent:

```go
if req.Stream {
    pipeline := streaming.NewPipeline(provider)
    if err := pipeline.Stream(r.Context(), &req, w); err != nil {
        // Error is already in the SSE stream via error events.
        // Only log it server-side; do NOT call http.Error after
        // headers have been sent.
        log.Printf("[ERROR] stream error for model=%s: %v", req.Model, err)
    }
    return
}
```

### 2.3 — HIGH: Add streaming timeout and backpressure

**Location:** `internal/streaming/sse.go`, `Stream()` method

Current code has no timeout for the overall stream or per-event timeout. A stalled upstream API will hold the HTTP connection open forever.

**Full replacement for `Stream()` method:**

```go
func (p *Pipeline) Stream(ctx context.Context, req *types.ChatCompletionRequest, w http.ResponseWriter) error {
    flusher, ok := w.(http.Flusher)
    if !ok {
        return fmt.Errorf("streaming not supported")
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")

    // Stream-level timeout: 5 minutes max for the entire completion.
    // Per-event stall timeout: 60 seconds — if no event arrives for 60s,
    // assume upstream is dead.
    const (
        streamTimeout = 5 * time.Minute
        stallTimeout  = 60 * time.Second
    )

    ctx, cancel := context.WithTimeout(ctx, streamTimeout)
    defer cancel()

    events := make(chan types.StreamEvent, 32)
    errCh := make(chan error, 1)

    go func() {
        errCh <- p.provider.StreamChat(ctx, req, events)
    }()

    stallTimer := time.NewTimer(stallTimeout)
    defer stallTimer.Stop()

    for {
        select {
        case event, ok := <-events:
            if !ok {
                // Channel closed — provider is done
                goto done
            }

            // Reset stall timer on every event
            if !stallTimer.Stop() {
                select {
                case <-stallTimer.C:
                default:
                }
            }
            stallTimer.Reset(stallTimeout)

            if event.Type == types.EventError {
                writeSSEError(w, event.Error)
                flusher.Flush()
                goto done
            }

            if err := writeSSEEvent(w, event, req.Model); err != nil {
                cancel()
                return fmt.Errorf("write SSE event: %w", err)
            }
            flusher.Flush()

        case <-stallTimer.C:
            cancel()
            writeSSEError(w, fmt.Errorf("upstream stalled: no event for %v", stallTimeout))
            flusher.Flush()
            goto done

        case <-ctx.Done():
            writeSSEError(w, ctx.Err())
            flusher.Flush()
            goto done
        }
    }

done:
    fmt.Fprintf(w, "data: [DONE]\n\n")
    flusher.Flush()

    // Drain the error channel — don't leak the goroutine
    select {
    case err := <-errCh:
        return err
    case <-time.After(5 * time.Second):
        return fmt.Errorf("provider goroutine did not exit within 5s")
    }
}
```

### 2.4 — MEDIUM: SSE writeSSEEvent should handle all event types

**Location:** `internal/streaming/sse.go`, `writeSSEEvent()` function (lines 77-110)

Currently only handles `EventContentDelta` and `EventContentDone`. Extend:

```go
func writeSSEEvent(w http.ResponseWriter, event types.StreamEvent, model string) error {
    chunk := types.ChatCompletionChunk{
        Object: "chat.completion.chunk",
        Model:  model,
    }

    switch event.Type {
    case types.EventContentDelta:
        chunk.Choices = []types.ChunkChoice{{
            Delta: types.Delta{Content: &event.Delta},
        }}

    case types.EventToolCallStart:
        if event.ToolCall != nil {
            chunk.Choices = []types.ChunkChoice{{
                Delta: types.Delta{
                    ToolCalls: []types.ToolCall{{
                        ID:   event.ToolCall.ID,
                        Type: "function",
                        Function: types.FunctionCall{
                            Name: event.ToolCall.Name,
                        },
                    }},
                },
            }}
        }

    case types.EventToolCallDelta:
        if event.ToolCall != nil {
            chunk.Choices = []types.ChunkChoice{{
                Delta: types.Delta{
                    ToolCalls: []types.ToolCall{{
                        ID: event.ToolCall.ID,
                        Function: types.FunctionCall{
                            Arguments: event.ToolCall.Arguments,
                        },
                    }},
                },
            }}
        }

    case types.EventToolCallComplete, types.EventContentDone:
        chunk.Choices = []types.ChunkChoice{{
            FinishReason: event.FinishReason,
        }}

    case types.EventStatus:
        // Status events: include usage if available
        if event.Usage != nil {
            chunk.Usage = event.Usage
        }
        // Don't write a chunk if there's no data
        if chunk.Usage == nil {
            return nil
        }

    default:
        // Unknown event type — skip silently
        return nil
    }

    if len(chunk.Choices) == 0 && chunk.Usage == nil {
        return nil
    }

    data, err := json.Marshal(chunk)
    if err != nil {
        return fmt.Errorf("marshal SSE chunk: %w", err)
    }
    _, err = fmt.Fprintf(w, "data: %s\n\n", data)
    return err
}
```

---

## 3. Ollama Provider Implementation

### 3.1 — New file: `internal/inference/ollama.go`

Ollama uses NDJSON streaming (not SSE), has different API paths, and runs locally.

```go
package inference

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "strings"
    "time"

    "forge/pkg/types"
)

// OllamaProvider implements InferenceProvider for Ollama's native API.
// Ollama uses NDJSON streaming (one JSON object per line), not SSE.
// API reference: https://github.com/ollama/ollama/blob/main/docs/api.md
type OllamaProvider struct {
    client  *http.Client
    baseURL string
}

// ollamaChatRequest maps to Ollama's /api/chat request format.
type ollamaChatRequest struct {
    Model    string              `json:"model"`
    Messages []ollamaChatMessage `json:"messages"`
    Stream   bool                `json:"stream"`
    Options  *ollamaOptions      `json:"options,omitempty"`
    Tools    []types.Tool        `json:"tools,omitempty"`
}

type ollamaChatMessage struct {
    Role      string     `json:"role"`
    Content   string     `json:"content"`
    ToolCalls []types.ToolCall `json:"tool_calls,omitempty"`
}

type ollamaOptions struct {
    Temperature float64 `json:"temperature,omitempty"`
    TopP        float64 `json:"top_p,omitempty"`
    NumPredict  int     `json:"num_predict,omitempty"`
    Stop        any     `json:"stop,omitempty"`
}

// ollamaChatResponse is a single NDJSON line from Ollama's /api/chat.
type ollamaChatResponse struct {
    Model           string            `json:"model"`
    CreatedAt       string            `json:"created_at"`
    Message         ollamaChatMessage `json:"message"`
    Done            bool              `json:"done"`
    DoneReason      string            `json:"done_reason,omitempty"`
    TotalDuration   int64             `json:"total_duration,omitempty"`
    PromptEvalCount int               `json:"prompt_eval_count,omitempty"`
    EvalCount       int               `json:"eval_count,omitempty"`
}

// ollamaTagsResponse is the response from /api/tags.
type ollamaTagsResponse struct {
    Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
    Name       string `json:"name"`
    Model      string `json:"model"`
    ModifiedAt string `json:"modified_at"`
    Size       int64  `json:"size"`
}

func NewOllamaProvider(baseURL string) *OllamaProvider {
    return &OllamaProvider{
        client: &http.Client{
            Timeout: 0, // Streaming — no overall timeout; per-request timeouts via context
            Transport: &http.Transport{
                MaxIdleConns:        10,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
                DialContext: (&net.Dialer{
                    Timeout:   5 * time.Second,
                    KeepAlive: 30 * time.Second,
                }).DialContext,
            },
        },
        baseURL: strings.TrimSuffix(baseURL, "/"),
    }
}

func (p *OllamaProvider) Name() string {
    return "ollama"
}

// Probe checks if Ollama is reachable at the configured URL.
// Returns true if the server responds within 2 seconds.
func (p *OllamaProvider) Probe(ctx context.Context) bool {
    probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(probeCtx, "GET", p.baseURL+"/api/tags", nil)
    if err != nil {
        return false
    }
    resp, err := p.client.Do(req)
    if err != nil {
        return false
    }
    resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}

func (p *OllamaProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
    if err != nil {
        return nil, fmt.Errorf("ollama list models: %w", err)
    }

    resp, err := p.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("ollama list models: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
        return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
    }

    var tags ollamaTagsResponse
    if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
        return nil, fmt.Errorf("ollama decode tags: %w", err)
    }

    models := make([]types.ModelInfo, 0, len(tags.Models))
    for _, m := range tags.Models {
        models = append(models, types.ModelInfo{
            ID:       m.Name,
            Object:   "model",
            OwnedBy:  "ollama",
            Provider: "ollama",
        })
    }
    return models, nil
}

func (p *OllamaProvider) Capabilities(model string) ModelCapabilities {
    // Ollama models: conservative defaults.
    // Could be refined per-model with /api/show endpoint.
    return ModelCapabilities{
        MaxContextTokens:    4096,
        MaxOutputTokens:     2048,
        DefaultOutputTokens: 1024,
        SupportsTools:       true,  // Ollama ≥0.4 supports tools
        SupportsVision:      false, // Model-dependent
        SupportsJSON:        true,
        SupportsStreaming:   true,
    }
}

func (p *OllamaProvider) CountTokens(messages []types.ChatMessage) (int, error) {
    // Estimate: ~4 chars per token for English text.
    // Ollama doesn't expose a tokenize endpoint for chat messages.
    total := 0
    for _, msg := range messages {
        total += 4 // Role overhead
        switch v := msg.Content.(type) {
        case string:
            total += len(v) / 4
        default:
            data, _ := json.Marshal(v)
            total += len(data) / 4
        }
    }
    if total < 1 {
        total = 1
    }
    return total, nil
}

func (p *OllamaProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
    ollamaReq := p.translateRequest(req)
    ollamaReq.Stream = false

    body, err := json.Marshal(ollamaReq)
    if err != nil {
        return nil, fmt.Errorf("ollama marshal: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("ollama request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("ollama chat: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
        return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(respBody))
    }

    var ollamaResp ollamaChatResponse
    if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
        return nil, fmt.Errorf("ollama decode: %w", err)
    }

    content := ollamaResp.Message.Content
    return &types.ChatCompletionResponse{
        Model:  ollamaResp.Model,
        Object: "chat.completion",
        Choices: []types.Choice{{
            Index:        0,
            Message:      types.ChatMessage{Role: "assistant", Content: content},
            FinishReason: p.translateFinishReason(ollamaResp.DoneReason),
        }},
        Usage: &types.Usage{
            PromptTokens:     ollamaResp.PromptEvalCount,
            CompletionTokens: ollamaResp.EvalCount,
            TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
        },
    }, nil
}

func (p *OllamaProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
    defer close(out)

    ollamaReq := p.translateRequest(req)
    ollamaReq.Stream = true

    body, err := json.Marshal(ollamaReq)
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return fmt.Errorf("ollama marshal: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return err
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(httpReq)
    if err != nil {
        out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
        return fmt.Errorf("ollama connect: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
        apiErr := fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(respBody))
        out <- types.StreamEvent{Type: types.EventError, Error: apiErr, ErrorMessage: apiErr.Error()}
        return apiErr
    }

    // Ollama streams NDJSON: one JSON object per line, NOT SSE.
    // Each line is a complete ollamaChatResponse.
    // The final line has "done": true.
    scanner := bufio.NewScanner(resp.Body)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    for scanner.Scan() {
        if ctx.Err() != nil {
            return ctx.Err()
        }

        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }

        var chunk ollamaChatResponse
        if err := json.Unmarshal(line, &chunk); err != nil {
            log.Printf("[WARN] Ollama NDJSON: skipping malformed line: %v", err)
            continue
        }

        if chunk.Done {
            // Final message — emit done event with usage
            out <- types.StreamEvent{
                Type:         types.EventContentDone,
                FinishReason: p.translateFinishReason(chunk.DoneReason),
                Usage: &types.Usage{
                    PromptTokens:     chunk.PromptEvalCount,
                    CompletionTokens: chunk.EvalCount,
                    TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
                },
            }
            break
        }

        // Streaming content delta
        if chunk.Message.Content != "" {
            out <- types.StreamEvent{
                Type:  types.EventContentDelta,
                Delta: chunk.Message.Content,
            }
        }

        // Tool calls in streaming mode (Ollama ≥0.4)
        for _, tc := range chunk.Message.ToolCalls {
            out <- types.StreamEvent{
                Type: types.EventToolCallStart,
                ToolCall: &types.ToolCallEvent{
                    ID:        tc.ID,
                    Name:      tc.Function.Name,
                    Arguments: tc.Function.Arguments,
                },
            }
        }
    }

    if err := scanner.Err(); err != nil {
        return fmt.Errorf("ollama stream read: %w", err)
    }
    return nil
}

// translateRequest converts the OpenAI-format request to Ollama's format.
func (p *OllamaProvider) translateRequest(req *types.ChatCompletionRequest) ollamaChatRequest {
    msgs := make([]ollamaChatMessage, 0, len(req.Messages))
    for _, m := range req.Messages {
        content := ""
        switch v := m.Content.(type) {
        case string:
            content = v
        default:
            data, _ := json.Marshal(v)
            content = string(data)
        }
        msgs = append(msgs, ollamaChatMessage{
            Role:      m.Role,
            Content:   content,
            ToolCalls: m.ToolCalls,
        })
    }

    ollamaReq := ollamaChatRequest{
        Model:    req.Model,
        Messages: msgs,
        Tools:    req.Tools,
    }

    // Map OpenAI sampling params to Ollama options
    opts := &ollamaOptions{}
    hasOpts := false
    if req.Temperature != nil {
        opts.Temperature = *req.Temperature
        hasOpts = true
    }
    if req.TopP != nil {
        opts.TopP = *req.TopP
        hasOpts = true
    }
    if req.MaxTokens != nil {
        opts.NumPredict = *req.MaxTokens
        hasOpts = true
    }
    if req.Stop != nil {
        opts.Stop = req.Stop
        hasOpts = true
    }
    if hasOpts {
        ollamaReq.Options = opts
    }

    return ollamaReq
}

func (p *OllamaProvider) translateFinishReason(ollamaReason string) string {
    switch ollamaReason {
    case "stop", "":
        return "stop"
    case "length":
        return "length"
    case "tool_calls":
        return "tool_calls"
    default:
        return ollamaReason
    }
}
```

### 3.2 — Wire Ollama into main.go

**Location:** `cmd/forge/main.go`

Add Ollama as the **default** provider with auto-detection:

```go
func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    cfg.Version = version

    providers := make(map[string]inference.InferenceProvider)

    // Ollama: always attempt auto-detection at configured URL
    ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
    if ollamaProvider.Probe(context.Background()) {
        providers["ollama"] = ollamaProvider
        log.Printf("Ollama detected at %s", cfg.OllamaURL)
    } else {
        log.Printf("Ollama not found at %s (skipping)", cfg.OllamaURL)
    }

    // OpenAI-compatible providers
    if cfg.OpenAIKey != "" {
        providers["openai"] = inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey)
    }
    if cfg.QwenKey != "" {
        providers["qwen"] = inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey)
    }
    // ... rest unchanged ...

    if len(providers) == 0 {
        // Fall back to mocks
        providers["qwen"] = inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"})
        // ...
    }

    srv := server.New(cfg, providers)
    srv.StartAndServe()
}
```

---

## 4. Token Counting Strategy

### 4.1 — Replace stub with character-based estimation

**Location:** `internal/inference/openai.go`, `CountTokens()` (line 74-78)

The current stub returns hardcoded `100`. Replace with a reasonable estimator:

```go
func (p *OpenAIProvider) CountTokens(messages []types.ChatMessage) (int, error) {
    // OpenAI tokenizer: ~4 chars per token for English.
    // Each message has ~4 tokens of overhead (role, delimiters).
    // This is a stopgap — replace with tiktoken-go when adding context management.
    total := 0
    for _, msg := range messages {
        total += 4 // Per-message overhead (role tags, separators)
        switch v := msg.Content.(type) {
        case string:
            total += len(v) / 4
        default:
            // Multimodal content (array of parts) — serialize and estimate
            data, _ := json.Marshal(v)
            total += len(data) / 4
        }
        // Tool calls contribute to token count
        for _, tc := range msg.ToolCalls {
            total += len(tc.Function.Name)/4 + len(tc.Function.Arguments)/4 + 4
        }
    }
    total += 2 // Conversation-level overhead (BOS/EOS)
    if total < 1 {
        total = 1
    }
    return total, nil
}
```

### 4.2 — Future: tiktoken-go integration

When you need accurate token counting (for context compaction):

```
go get github.com/pkoukk/tiktoken-go
```

Then create `internal/inference/tokenizer.go`:

```go
package inference

import (
    "sync"
    "github.com/pkoukk/tiktoken-go"
)

// TokenizerCache maintains per-model tokenizer instances.
// Tokenizer creation is expensive (~50ms), so cache them.
type TokenizerCache struct {
    mu    sync.RWMutex
    cache map[string]*tiktoken.Tiktoken
}

var globalTokenizerCache = &TokenizerCache{
    cache: make(map[string]*tiktoken.Tiktoken),
}

func (tc *TokenizerCache) Get(model string) (*tiktoken.Tiktoken, error) {
    tc.mu.RLock()
    if enc, ok := tc.cache[model]; ok {
        tc.mu.RUnlock()
        return enc, nil
    }
    tc.mu.RUnlock()

    tc.mu.Lock()
    defer tc.mu.Unlock()

    // Double-check after acquiring write lock
    if enc, ok := tc.cache[model]; ok {
        return enc, nil
    }

    enc, err := tiktoken.EncodingForModel(model)
    if err != nil {
        // Fall back to cl100k_base (GPT-4 / GPT-3.5 tokenizer)
        enc, err = tiktoken.GetEncoding("cl100k_base")
        if err != nil {
            return nil, err
        }
    }

    tc.cache[model] = enc
    return enc, nil
}
```

This is Phase 2 — not needed for MVP.

---

## 5. HTTP Client Hardening

### 5.1 — Add timeouts and transport to OpenAI provider

**Location:** `internal/inference/openai.go`, `NewOpenAIProvider()` (lines 23-30)

The current client is `&http.Client{}` — no timeouts, no connection pool config, no TLS settings.

```go
func NewOpenAIProvider(providerName, baseURL, apiKey string) *OpenAIProvider {
    return &OpenAIProvider{
        client: &http.Client{
            // No overall Timeout here — streaming responses can be long.
            // Individual timeouts are handled via context.
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
                TLSHandshakeTimeout: 10 * time.Second,
                ExpectContinueTimeout: 1 * time.Second,
                ResponseHeaderTimeout: 30 * time.Second, // Time to first byte
                DialContext: (&net.Dialer{
                    Timeout:   10 * time.Second,
                    KeepAlive: 30 * time.Second,
                }).DialContext,
            },
        },
        baseURL:      strings.TrimSuffix(baseURL, "/"),
        apiKey:       apiKey,
        providerName: providerName,
    }
}
```

Add imports: `"net"`, `"time"`.

### 5.2 — Add connect timeout to `Complete()` (non-streaming)

For non-streaming calls, wrap the context with a deadline:

```go
func (p *OpenAIProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
    // Non-streaming calls get a 2-minute hard timeout
    ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    defer cancel()

    req.Stream = false
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }
    // ... rest unchanged ...
}
```

### 5.3 — Add retry with exponential backoff for transient errors

Create `internal/inference/retry.go`:

```go
package inference

import (
    "context"
    "fmt"
    "math"
    "net/http"
    "time"
)

// RetryConfig controls retry behavior for upstream API calls.
type RetryConfig struct {
    MaxAttempts int           // Total attempts including initial (default: 3)
    BaseDelay   time.Duration // Initial delay (default: 500ms)
    MaxDelay    time.Duration // Cap on backoff (default: 10s)
}

var DefaultRetryConfig = RetryConfig{
    MaxAttempts: 3,
    BaseDelay:   500 * time.Millisecond,
    MaxDelay:    10 * time.Second,
}

// isRetryableStatus returns true for status codes that indicate a transient error.
func isRetryableStatus(code int) bool {
    switch code {
    case http.StatusTooManyRequests,       // 429
        http.StatusInternalServerError,     // 500
        http.StatusBadGateway,              // 502
        http.StatusServiceUnavailable,      // 503
        http.StatusGatewayTimeout:          // 504
        return true
    }
    return false
}

// retryDo executes an HTTP request with retry logic.
// Only use for non-streaming (Complete, ListModels) — streaming calls are NOT retryable.
func retryDo(ctx context.Context, client *http.Client, req *http.Request, cfg RetryConfig) (*http.Response, error) {
    var lastErr error
    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }

        // Clone the request for each attempt (body was already consumed)
        // The caller must use bytes.NewReader so Seek works.
        if req.GetBody != nil {
            body, err := req.GetBody()
            if err != nil {
                return nil, fmt.Errorf("retry get body: %w", err)
            }
            req.Body = body
        }

        resp, err := client.Do(req)
        if err != nil {
            lastErr = err
            // Network errors are retryable
            delay := backoffDelay(attempt, cfg)
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
                continue
            }
        }

        if !isRetryableStatus(resp.StatusCode) {
            return resp, nil
        }

        // Retryable HTTP status — close body and retry
        resp.Body.Close()
        lastErr = fmt.Errorf("HTTP %d (attempt %d/%d)", resp.StatusCode, attempt+1, cfg.MaxAttempts)

        delay := backoffDelay(attempt, cfg)
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(delay):
        }
    }

    return nil, fmt.Errorf("all %d attempts failed: %w", cfg.MaxAttempts, lastErr)
}

func backoffDelay(attempt int, cfg RetryConfig) time.Duration {
    delay := time.Duration(float64(cfg.BaseDelay) * math.Pow(2, float64(attempt)))
    if delay > cfg.MaxDelay {
        delay = cfg.MaxDelay
    }
    return delay
}
```

Then in `openai.go`, use `retryDo` for `Complete()` and `ListModels()`:

```go
// In Complete():
resp, err := retryDo(ctx, p.client, httpReq, DefaultRetryConfig)

// In ListModels():
resp, err := retryDo(ctx, p.client, req, DefaultRetryConfig)
```

**Do NOT retry `StreamChat`** — streaming calls are not idempotent once bytes have been emitted.

---

## 6. Provider Registration & Model Routing

### 6.1 — Replace hardcoded string matching with a ProviderRegistry

**Location:** Replace `internal/api/handlers_chat.go` `getProviderForModel()` (lines 76-98)

The current routing is a series of hardcoded conditionals. Replace with a proper registry.

Create `internal/inference/registry.go`:

```go
package inference

import (
    "context"
    "fmt"
    "log"
    "strings"
    "sync"
)

// ProviderRegistry manages provider instances and routes models to providers.
type ProviderRegistry struct {
    mu        sync.RWMutex
    providers map[string]InferenceProvider // keyed by provider name
    modelMap  map[string]string           // model ID -> provider name
    defaultProvider string               // fallback provider name
}

func NewProviderRegistry() *ProviderRegistry {
    return &ProviderRegistry{
        providers: make(map[string]InferenceProvider),
        modelMap:  make(map[string]string),
    }
}

// Register adds a provider. If it's the first provider, it becomes the default.
func (r *ProviderRegistry) Register(provider InferenceProvider) {
    r.mu.Lock()
    defer r.mu.Unlock()

    name := provider.Name()
    r.providers[name] = provider
    if r.defaultProvider == "" {
        r.defaultProvider = name
    }
}

// SetDefault sets the default provider for unrecognized models.
func (r *ProviderRegistry) SetDefault(name string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.defaultProvider = name
}

// RefreshModelMap queries all providers for their models and builds the routing table.
// Call this at startup and periodically (e.g., every 5 minutes).
func (r *ProviderRegistry) RefreshModelMap(ctx context.Context) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    newMap := make(map[string]string)
    for name, provider := range r.providers {
        models, err := provider.ListModels(ctx)
        if err != nil {
            log.Printf("[WARN] Failed to list models for %s: %v", name, err)
            continue
        }
        for _, m := range models {
            newMap[m.ID] = name
        }
    }

    r.modelMap = newMap
    log.Printf("Model registry refreshed: %d models across %d providers", len(newMap), len(r.providers))
    return nil
}

// Resolve finds the provider for a given model string.
// Resolution order:
//  1. Exact match in model map (populated by RefreshModelMap)
//  2. "provider/model" prefix syntax (e.g., "ollama/llama3.2:1b")
//  3. Default provider
func (r *ProviderRegistry) Resolve(model string) (InferenceProvider, string, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // 1. Check "provider/model" prefix syntax
    if idx := strings.IndexByte(model, '/'); idx > 0 {
        providerName := model[:idx]
        modelName := model[idx+1:]
        if p, ok := r.providers[providerName]; ok {
            return p, modelName, nil
        }
    }

    // 2. Exact match in model map
    if providerName, ok := r.modelMap[model]; ok {
        if p, ok2 := r.providers[providerName]; ok2 {
            return p, model, nil
        }
    }

    // 3. Default provider (pass model name through)
    if r.defaultProvider != "" {
        if p, ok := r.providers[r.defaultProvider]; ok {
            return p, model, nil
        }
    }

    return nil, "", fmt.Errorf("no provider found for model %q", model)
}

// ListAllModels aggregates models from all providers.
func (r *ProviderRegistry) ListAllModels(ctx context.Context) []types.ModelInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()

    var all []types.ModelInfo
    for _, provider := range r.providers {
        models, err := provider.ListModels(ctx)
        if err != nil {
            log.Printf("[WARN] ListModels failed for %s: %v", provider.Name(), err)
            continue
        }
        all = append(all, models...)
    }
    return all
}

// Providers returns a snapshot of all registered providers.
func (r *ProviderRegistry) Providers() map[string]InferenceProvider {
    r.mu.RLock()
    defer r.mu.RUnlock()

    snapshot := make(map[string]InferenceProvider, len(r.providers))
    for k, v := range r.providers {
        snapshot[k] = v
    }
    return snapshot
}
```

Note: need to add the import `"forge/pkg/types"` at the top.

### 6.2 — Update Router to use ProviderRegistry

**Location:** `internal/api/handlers_chat.go` — replace Router struct and methods

```go
package api

import (
    "encoding/json"
    "log"
    "net/http"

    "forge/internal/inference"
    "forge/internal/streaming"
    "forge/pkg/types"
    "github.com/go-chi/chi/v5"
)

type Router struct {
    registry *inference.ProviderRegistry
}

func NewRouter(registry *inference.ProviderRegistry) *Router {
    return &Router{registry: registry}
}

func (router *Router) SetupRoutes(r chi.Router) {
    r.Post("/v1/chat/completions", router.HandleChatCompletions)
    r.Get("/v1/models", router.HandleModels)
}

func (router *Router) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
    var req types.ChatCompletionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    provider, resolvedModel, err := router.registry.Resolve(req.Model)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    // Replace the model in the request with the resolved name
    // (strips "provider/" prefix if used)
    req.Model = resolvedModel

    if req.Stream {
        pipeline := streaming.NewPipeline(provider)
        if err := pipeline.Stream(r.Context(), &req, w); err != nil {
            // Headers already sent — cannot use http.Error.
            // Error was already written as an SSE error event.
            log.Printf("[ERROR] stream error model=%s provider=%s: %v",
                resolvedModel, provider.Name(), err)
        }
        return
    }

    resp, err := provider.Complete(r.Context(), &req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (router *Router) HandleModels(w http.ResponseWriter, r *http.Request) {
    models := router.registry.ListAllModels(r.Context())
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(types.ModelListResponse{
        Object: "list",
        Data:   models,
    })
}
```

### 6.3 — Update main.go to use ProviderRegistry

```go
func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    cfg.Version = version

    registry := inference.NewProviderRegistry()

    // Ollama — auto-detect
    ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
    if ollamaProvider.Probe(context.Background()) {
        registry.Register(ollamaProvider)
        log.Printf("Ollama detected at %s", cfg.OllamaURL)
    }

    // OpenAI-compatible providers
    if cfg.OpenAIKey != "" {
        registry.Register(inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey))
    }
    if cfg.QwenKey != "" {
        registry.Register(inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey))
    }
    if cfg.LlamaKey != "" {
        registry.Register(inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey))
    }
    if cfg.MinimaxKey != "" {
        registry.Register(inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey))
    }
    if cfg.OSSKey != "" {
        registry.Register(inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey))
    }

    // Set default provider from config
    if cfg.DefaultProvider != "" {
        registry.SetDefault(cfg.DefaultProvider)
    }

    // Populate model map
    if err := registry.RefreshModelMap(context.Background()); err != nil {
        log.Printf("[WARN] Initial model map refresh failed: %v", err)
    }

    // Fall back to mocks if nothing is configured
    if len(registry.Providers()) == 0 {
        log.Println("No providers configured — using mock providers")
        registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
        registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
        registry.SetDefault("qwen")
    }

    srv := server.New(cfg, registry)
    srv.StartAndServe()
}
```

### 6.4 — Update server.go to accept ProviderRegistry

```go
func New(cfg *config.Config, registry *inference.ProviderRegistry) *Server {
    apiRouter := api.NewRouter(registry)
    // ... rest unchanged ...
}
```

---

## 7. File Summary — What Gets Created/Modified

| File | Action | Scope |
|------|--------|-------|
| `internal/inference/openai.go` | **MODIFY** | Fix marshal error handling, add tool call streaming, add error events, bounded ReadAll, HTTP client hardening |
| `internal/inference/ollama.go` | **CREATE** | Full Ollama provider (NDJSON streaming, /api/chat, /api/tags, Probe) |
| `internal/inference/registry.go` | **CREATE** | ProviderRegistry with model routing, prefix syntax, auto-refresh |
| `internal/inference/retry.go` | **CREATE** | retryDo with exponential backoff for non-streaming calls |
| `internal/streaming/sse.go` | **MODIFY** | Remove CloseNotifier, add stall timeout, add backpressure, handle all event types |
| `internal/api/handlers_chat.go` | **MODIFY** | Use ProviderRegistry, fix post-header http.Error bug |
| `internal/server/server.go` | **MODIFY** | Accept ProviderRegistry instead of map |
| `cmd/forge/main.go` | **MODIFY** | Wire registry, add Ollama auto-detection |
| `pkg/types/events.go` | NO CHANGE | Event types already defined correctly |
| `pkg/types/openai.go` | NO CHANGE | Types already sufficient |

---

## 8. Implementation Order

**Phase 1 — Critical fixes (do first, in this order):**
1. Fix `json.Marshal` error swallowing in `openai.go`
2. Add error events to `StreamChat` pre-stream failures in `openai.go`
3. Remove `CloseNotifier` from `sse.go`
4. Fix `http.Error` after headers bug in `handlers_chat.go`
5. Add `io.LimitReader` to all `io.ReadAll` calls

**Phase 2 — Infrastructure (can be done in parallel):**
6. Create `retry.go` — retry with backoff
7. Create `registry.go` — ProviderRegistry
8. Harden HTTP client in `NewOpenAIProvider`
9. Replace token counting stub
10. Update `handlers_chat.go` to use registry
11. Update `server.go` and `main.go`

**Phase 3 — New capability:**
12. Create `ollama.go` — full Ollama provider
13. Wire Ollama into `main.go` with auto-detection

**Phase 4 — Streaming hardening:**
14. Rewrite `Pipeline.Stream()` with stall timer
15. Extend `writeSSEEvent()` for all event types
16. Add tool call delta handling in `openai.go` SSE parser

---

## 9. Testing Strategy

Each phase needs verification:

```bash
# Phase 1 — Verify build still passes
go build ./...
go vet ./...

# Phase 2 — Unit tests for retry and registry
go test ./internal/inference/ -run TestRetry
go test ./internal/inference/ -run TestRegistry

# Phase 3 — Integration test with local Ollama
# (requires Ollama running)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:1b","messages":[{"role":"user","content":"hi"}],"stream":true}'

# Phase 4 — Streaming tests
# Test stall timeout: use mock provider with 120s delay
# Test client disconnect: curl + Ctrl-C mid-stream
# Test tool calls: send request with tools defined
```
