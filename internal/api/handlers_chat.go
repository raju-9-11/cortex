package api

import (
	"encoding/json"
	"net/http"

	"forge/internal/inference"
	"forge/internal/streaming"
	"forge/pkg/types"
	"github.com/go-chi/chi/v5"
)

type Router struct {
	providers map[string]inference.InferenceProvider
}

func NewRouter(providers map[string]inference.InferenceProvider) *Router {
	return &Router{providers: providers}
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

	provider := router.getProviderForModel(req.Model)
	if provider == nil {
		http.Error(w, "Provider not found for model: "+req.Model, http.StatusNotFound)
		return
	}

	if req.Stream {
		pipeline := streaming.NewPipeline(provider)
		err := pipeline.Stream(r.Context(), &req, w)
		if err != nil {
			// If already streaming, error is in SSE. Else:
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
	var models []types.ModelInfo
	for _, p := range router.providers {
		m, err := p.ListModels(r.Context())
		if err == nil {
			models = append(models, m...)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ModelListResponse{
		Object: "list",
		Data:   models,
	})
}

// Very basic routing based on model prefixes or direct matching
func (router *Router) getProviderForModel(model string) inference.InferenceProvider {
	// Let's assume the provider name is passed as prefix like "openai/gpt-4"
	// For MVP: Check provider names directly, or default to first if only one exists.
	// We'll hardcode some simple mappings:
	if len(router.providers) == 0 {
		return nil
	}

	for name, p := range router.providers {
		// e.g. "openai" handles "gpt-..."
		// For our testing we use provider name matching
		if name == model || (model == "qwen" && name == "qwen") || (model == "llama" && name == "llama") || (model == "minimax" && name == "minimax") || (model == "oss" && name == "oss") {
			return p
		}
	}

	// Default to first provider
	for _, p := range router.providers {
		return p
	}

	return nil
}
