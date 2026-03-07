package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"forge/internal/inference"
	"forge/pkg/types"

	"github.com/google/uuid"
)

type Pipeline struct {
	provider inference.InferenceProvider
}

func NewPipeline(p inference.InferenceProvider) *Pipeline {
	return &Pipeline{provider: p}
}

func (p *Pipeline) Stream(ctx context.Context, req *types.ChatCompletionRequest, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Generate a stable chunk ID and timestamp for all chunks in this stream.
	// OpenAI-compatible clients expect a non-empty "id" and non-zero "created"
	// on every ChatCompletionChunk.
	chunkID := "chatcmpl-" + uuid.NewString()[:8]
	created := time.Now().Unix()

	// Stream-level timeout: 5 minutes max for the entire completion.
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

			// Reset stall timer on every received event
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

			if err := writeSSEEvent(w, event, req.Model, chunkID, created); err != nil {
				cancel()
				return fmt.Errorf("write SSE event: %w", err)
			}
			flusher.Flush()

		case <-stallTimer.C:
			cancel()
			stallErr := fmt.Errorf("upstream stalled: no event for %v", stallTimeout)
			writeSSEError(w, stallErr)
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

	// Drain the events channel so the provider goroutine can exit
	// even if it's blocked writing to the channel.
	go func() { for range events {} }()

	// Drain the error channel — don't leak the goroutine
	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("provider goroutine did not exit within 5s")
	}
}

func writeSSEEvent(w http.ResponseWriter, event types.StreamEvent, model string, chunkID string, created int64) error {
	chunk := types.ChatCompletionChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Model:   model,
		Created: created,
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
		if event.Usage != nil {
			chunk.Usage = event.Usage
		}
		if chunk.Usage == nil {
			return nil
		}

	default:
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

func writeSSEError(w http.ResponseWriter, err error) {
	errResp := types.APIErrorResponse{
		Error: types.APIError{
			Code:    types.ErrCodeStreamFailed,
			Message: err.Error(),
			Type:    types.ErrorTypeServer,
		},
	}
	data, _ := json.Marshal(errResp)
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
}
