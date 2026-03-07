package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"forge/internal/inference"
	"forge/pkg/types"
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if cn, ok := w.(http.CloseNotifier); ok {
		go func() {
			select {
			case <-cn.CloseNotify():
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	events := make(chan types.StreamEvent, 32)
	errCh := make(chan error, 1)

	go func() {
		errCh <- p.provider.StreamChat(ctx, req, events)
	}()

	for event := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if event.Type == types.EventError {
			// Write error and stop
			writeSSEError(w, event.Error)
			break
		}

		if err := writeSSEEvent(w, event, req.Model); err != nil {
			cancel()
			return err
		}
		flusher.Flush()
	}

	// Always write [DONE] at the very end
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	return <-errCh
}

func writeSSEEvent(w http.ResponseWriter, event types.StreamEvent, model string) error {
	// Translate StreamEvent back to OpenAI ChatCompletionChunk
	chunk := types.ChatCompletionChunk{
		Object: "chat.completion.chunk",
		Model:  model,
	}

	if event.Type == types.EventContentDelta {
		chunk.Choices = []types.ChunkChoice{
			{
				Delta: types.Delta{
					Content: &event.Delta,
				},
			},
		}
	} else if event.Type == types.EventContentDone {
		chunk.Choices = []types.ChunkChoice{
			{
				FinishReason: event.FinishReason,
			},
		}
	}

	if len(chunk.Choices) > 0 {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "data: %s\n\n", string(data))
		return err
	}

	return nil
}

func writeSSEError(w http.ResponseWriter, err error) {
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", fmt.Sprintf(`{"error": "%s"}`, err.Error()))
}
