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

	"cortex/pkg/types"
)

// ---------------------------------------------------------------------------
// Ollama-specific request / response structs (unexported)
// ---------------------------------------------------------------------------

type ollamaMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type ollamaToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"` // Ollama may return a map rather than a string
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ollamaOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"`
	Stop        any      `json:"stop,omitempty"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
}

// ollamaChatResponse is returned for both streaming (one per line) and
// non-streaming (single JSON object) /api/chat calls.
type ollamaChatResponse struct {
	Model           string         `json:"model"`
	CreatedAt       string         `json:"created_at"`
	Message         ollamaMessage  `json:"message"`
	Done            bool           `json:"done"`
	DoneReason      string         `json:"done_reason,omitempty"`
	TotalDuration   int64          `json:"total_duration,omitempty"`
	LoadDuration    int64          `json:"load_duration,omitempty"`
	PromptEvalCount int            `json:"prompt_eval_count,omitempty"`
	EvalCount       int            `json:"eval_count,omitempty"`
}

// ollamaTagsResponse is the response from GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	ModifiedAt string    `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    ollamaModelDetails `json:"details,omitempty"`
}

type ollamaModelDetails struct {
	Format            string   `json:"format,omitempty"`
	Family            string   `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameter_size,omitempty"`
	QuantizationLevel string   `json:"quantization_level,omitempty"`
}

// ---------------------------------------------------------------------------
// OllamaProvider
// ---------------------------------------------------------------------------

// OllamaProvider implements InferenceProvider for a local Ollama instance.
type OllamaProvider struct {
	client       *http.Client // for non-streaming requests (30s timeout)
	streamClient *http.Client // for streaming requests (no timeout)
	baseURL      string
}

// NewOllamaProvider creates a new Ollama provider pointing at the given base
// URL (e.g. "http://localhost:11434").  The HTTP client is configured with a
// transport tuned for streaming: no overall timeout (streaming reads are
// indefinite), but sensible dial and idle timeouts.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 10,
	}

	return &OllamaProvider{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		streamClient: &http.Client{
			Transport: transport.Clone(),
			// No overall Timeout – streaming responses need indefinite reads.
		},
		baseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

// Name returns the provider identifier.
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// ---------------------------------------------------------------------------
// Probe
// ---------------------------------------------------------------------------

// Probe checks whether an Ollama server is reachable at the configured URL.
// It hits GET /api/tags with a 2-second timeout and returns true on 200 OK.
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
	defer resp.Body.Close()
	// Drain body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

// ListModels fetches the available models from the Ollama server via
// GET /api/tags and converts them to the common ModelInfo type.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		// Ollama is unreachable – return empty list for graceful degradation.
		return []types.ModelInfo{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("ollama: list models HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("ollama: decode tags response: %w", err)
	}

	models := make([]types.ModelInfo, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		models = append(models, types.ModelInfo{
			ID:       m.Name,
			Object:   "model",
			OwnedBy:  "ollama",
			Provider: "ollama",
		})
	}

	return models, nil
}

// ---------------------------------------------------------------------------
// Complete (non-streaming)
// ---------------------------------------------------------------------------

// Complete sends a non-streaming chat completion request to Ollama and returns
// the response in OpenAI-compatible format.
func (p *OllamaProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	ollamaReq := translateRequest(req, false)

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: complete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("ollama: complete HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	// Build tool calls if present.
	var toolCalls []types.ToolCall
	for _, tc := range ollamaResp.Message.ToolCalls {
		argsStr, _ := marshalArguments(tc.Function.Arguments)
		toolCalls = append(toolCalls, types.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: types.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: argsStr,
			},
		})
	}

	chatResp := &types.ChatCompletionResponse{
		Model: ollamaResp.Model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:      ollamaResp.Message.Role,
					Content:   ollamaResp.Message.Content,
					ToolCalls: toolCalls,
				},
				FinishReason: translateFinishReason(ollamaResp.DoneReason),
			},
		},
		Usage: &types.Usage{
			PromptTokens:     ollamaResp.PromptEvalCount,
			CompletionTokens: ollamaResp.EvalCount,
			TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		},
	}

	return chatResp, nil
}

// ---------------------------------------------------------------------------
// StreamChat
// ---------------------------------------------------------------------------

// StreamChat sends a streaming chat completion request to Ollama. Ollama uses
// NDJSON (one JSON object per line), NOT SSE. The final line has "done": true
// with usage statistics. The channel is always closed before this method
// returns.
func (p *OllamaProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	defer close(out)

	ollamaReq := translateRequest(req, true)

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		sendError(out, fmt.Errorf("ollama: marshal request: %w", err))
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		sendError(out, fmt.Errorf("ollama: create request: %w", err))
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.streamClient.Do(httpReq)
	if err != nil {
		sendError(out, fmt.Errorf("ollama: stream request: %w", err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		streamErr := fmt.Errorf("ollama: stream HTTP %d: %s", resp.StatusCode, string(errBody))
		sendError(out, streamErr)
		return streamErr
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Track active tool calls across chunks.
	activeToolCalls := make(map[int]*types.ToolCallEvent) // keyed by index within the chunk

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var chunk ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("[ollama] warning: malformed NDJSON line: %s (error: %v)", line, err)
			continue
		}

		// Handle tool calls (Ollama ≥0.4).
		if len(chunk.Message.ToolCalls) > 0 {
			for i, tc := range chunk.Message.ToolCalls {
				argsStr, _ := marshalArguments(tc.Function.Arguments)

				if _, exists := activeToolCalls[i]; !exists {
					// New tool call – emit start event.
					tce := &types.ToolCallEvent{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: argsStr,
					}
					activeToolCalls[i] = tce
					select {
					case out <- types.StreamEvent{
						Type:     types.EventToolCallStart,
						ToolCall: tce,
					}:
					case <-ctx.Done():
						return ctx.Err()
					}
				} else {
					// Continuation – emit delta.
					activeToolCalls[i].Arguments += argsStr
					select {
					case out <- types.StreamEvent{
						Type: types.EventToolCallDelta,
						ToolCall: &types.ToolCallEvent{
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: argsStr,
						},
					}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
		}

		// Handle content delta.
		if chunk.Message.Content != "" {
			select {
			case out <- types.StreamEvent{
				Type:  types.EventContentDelta,
				Delta: chunk.Message.Content,
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Handle completion.
		if chunk.Done {
			// Emit tool-call-complete events for any accumulated tool calls.
			for _, tce := range activeToolCalls {
				select {
				case out <- types.StreamEvent{
					Type:     types.EventToolCallComplete,
					ToolCall: tce,
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			finishReason := translateFinishReason(chunk.DoneReason)
			select {
			case out <- types.StreamEvent{
				Type:         types.EventContentDone,
				FinishReason: finishReason,
				Usage: &types.Usage{
					PromptTokens:     chunk.PromptEvalCount,
					CompletionTokens: chunk.EvalCount,
					TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
				},
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		sendError(out, fmt.Errorf("ollama: stream read: %w", err))
		return err
	}

	return nil
}

// ---------------------------------------------------------------------------
// CountTokens
// ---------------------------------------------------------------------------

// CountTokens provides a character-based token estimate. Each message gets
// len(content)/4 tokens plus 4 tokens of per-message overhead. An additional
// 2 tokens cover conversation-level overhead.
func (p *OllamaProvider) CountTokens(messages []types.ChatMessage) (int, error) {
	return EstimateTokens(messages), nil
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

// Capabilities returns conservative defaults for Ollama models. The actual
// capabilities vary by model, but we cannot query them from the Ollama API,
// so we use safe lower bounds.
func (p *OllamaProvider) Capabilities(model string) ModelCapabilities {
	return ModelCapabilities{
		MaxContextTokens:    4096,
		MaxOutputTokens:     2048,
		DefaultOutputTokens: 1024,
		SupportsTools:       true,
		SupportsVision:      false,
		SupportsJSON:        false,
		SupportsStreaming:    true,
		ProviderID:          "ollama",
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// translateRequest converts an OpenAI-style ChatCompletionRequest to the
// Ollama /api/chat request format.
func translateRequest(req *types.ChatCompletionRequest, stream bool) ollamaChatRequest {
	// Convert messages.
	msgs := make([]ollamaMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		om := ollamaMessage{
			Role:       m.Role,
			Content:    stringifyContent(m.Content),
			ToolCallID: m.ToolCallID,
		}
		// Convert tool calls.
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: ollamaFunctionCall{
					Name:      tc.Function.Name,
					Arguments: json.RawMessage(tc.Function.Arguments),
				},
			})
		}
		msgs = append(msgs, om)
	}

	// Build options from top-level OpenAI fields.
	var opts *ollamaOptions
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil || req.Stop != nil {
		opts = &ollamaOptions{
			Temperature: req.Temperature,
			TopP:        req.TopP,
			NumPredict:  req.MaxTokens,
			Stop:        req.Stop,
		}
	}

	// Convert tools.
	var tools []ollamaTool
	for _, t := range req.Tools {
		tools = append(tools, ollamaTool{
			Type: t.Type,
			Function: ollamaToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	return ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   stream,
		Options:  opts,
		Tools:    tools,
	}
}

// translateFinishReason maps Ollama's done_reason to OpenAI's finish_reason.
func translateFinishReason(ollamaReason string) string {
	switch ollamaReason {
	case "stop":
		return "stop"
	case "length":
		return "length"
	case "tool_calls":
		return "tool_calls"
	case "":
		// Ollama often omits done_reason when done is true with a "stop" case.
		return "stop"
	default:
		return ollamaReason
	}
}

// stringifyContent converts the ChatMessage.Content (which is any) to a plain
// string. If the content is already a string it is returned as-is. If it is a
// structured value (e.g. []ContentPart) it is JSON-serialized.
func stringifyContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// marshalArguments converts Ollama's function arguments (which may be a map or
// already a string) into a JSON string suitable for the OpenAI ToolCall format.
func marshalArguments(args any) (string, error) {
	if args == nil {
		return "{}", nil
	}
	switch v := args.(type) {
	case string:
		return v, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "{}", err
		}
		return string(b), nil
	}
}

// sendError emits an error event on the stream channel.
func sendError(out chan<- types.StreamEvent, err error) {
	out <- types.StreamEvent{
		Type:         types.EventError,
		Error:        err,
		ErrorMessage: err.Error(),
	}
}
