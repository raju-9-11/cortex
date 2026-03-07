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

type OpenAIProvider struct {
	client       *http.Client
	baseURL      string
	apiKey       string
	providerName string
}

func NewOpenAIProvider(providerName, baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		client: &http.Client{
			// No overall Timeout — streaming responses can run for minutes.
			// Per-request timeouts are handled via context.
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
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

func (p *OpenAIProvider) Name() string {
	return p.providerName
}

func (p *OpenAIProvider) Capabilities(model string) ModelCapabilities {
	// For testing, just return default.
	// In reality, could depend on the model name and provider
	return DefaultCapabilities
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ml types.ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&ml); err != nil {
		return nil, err
	}

	for i := range ml.Data {
		ml.Data[i].Provider = p.providerName
	}

	return ml.Data, nil
}

func (p *OpenAIProvider) CountTokens(messages []types.ChatMessage) (int, error) {
	// Estimate: ~4 chars per token for English text.
	// Each message has ~4 tokens overhead (role, delimiters).
	// Stopgap until tiktoken-go integration.
	total := 0
	for _, msg := range messages {
		total += 4 // per-message overhead
		switch v := msg.Content.(type) {
		case string:
			total += len(v) / 4
		default:
			data, _ := json.Marshal(v)
			total += len(data) / 4
		}
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name)/4 + len(tc.Function.Arguments)/4 + 4
		}
	}
	total += 2 // conversation overhead
	if total < 1 {
		total = 1
	}
	return total, nil
}

func (p *OpenAIProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Non-streaming calls get a 2-minute hard timeout
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	localReq := *req
	localReq.Stream = false
	body, err := json.Marshal(&localReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	return &chatResp, nil
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	defer close(out)

	localReq := *req
	localReq.Stream = true
	body, err := json.Marshal(&localReq)
	if err != nil {
		out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		out <- types.StreamEvent{Type: types.EventError, Error: err, ErrorMessage: err.Error()}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		apiErr := fmt.Errorf("upstream HTTP %d: %s", resp.StatusCode, string(respBody))
		out <- types.StreamEvent{Type: types.EventError, Error: apiErr, ErrorMessage: apiErr.Error()}
		return apiErr
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var chunk types.ChatCompletionChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			log.Printf("[WARN] OpenAI SSE: skipping malformed chunk: %v (data: %.100s)", err, dataStr)
			continue
		}

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
					out <- types.StreamEvent{
						Type: types.EventToolCallStart,
						ToolCall: &types.ToolCallEvent{
							ID:   tc.ID,
							Name: tc.Function.Name,
						},
					}
				}
				if tc.Function.Arguments != "" {
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
	}

	return scanner.Err()
}
