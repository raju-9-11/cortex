package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		client:       &http.Client{},
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
		body, _ := io.ReadAll(resp.Body)
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
	// Dummy token count for now.
	// We would normally use tiktoken here.
	return 100, nil
}

func (p *OpenAIProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	req.Stream = false
	body, _ := json.Marshal(req)

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
		respBody, _ := io.ReadAll(resp.Body)
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

	req.Stream = true
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
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
			// Skip bad JSON
			continue
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]

			if choice.Delta.Content != nil {
				out <- types.StreamEvent{
					Type:  types.EventContentDelta,
					Delta: *choice.Delta.Content,
				}
			}

			if choice.FinishReason != "" {
				out <- types.StreamEvent{
					Type:         types.EventContentDone,
					FinishReason: choice.FinishReason,
				}
			}
		}
	}

	return scanner.Err()
}
