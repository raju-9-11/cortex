package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"cortex/internal/inference"
	"cortex/internal/streaming"
	"cortex/pkg/types"
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
	// Validate Content-Type
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		types.WriteError(w, http.StatusUnsupportedMediaType, types.ErrCodeUnsupportedMedia,
			types.ErrorTypeInvalidRequest, "Content-Type must be application/json")
		return
	}

	var req types.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.WriteError(w, http.StatusBadRequest, types.ErrCodeMalformedJSON,
			types.ErrorTypeInvalidRequest, "Invalid JSON in request body: "+err.Error())
		return
	}

	provider, resolvedModel, err := router.registry.Resolve(req.Model)
	if err != nil {
		types.WriteError(w, http.StatusNotFound, types.ErrCodeModelNotFound,
			types.ErrorTypeNotFound, "No provider found for model: "+req.Model)
		return
	}
	// Replace model with resolved name (strips "provider/" prefix if used)
	req.Model = resolvedModel

	if req.Stream {
		pipeline := streaming.NewPipeline(provider)
		if err := pipeline.Stream(r.Context(), &req, w); err != nil {
			// Headers already sent — cannot use http.Error or types.WriteError.
			// Error was already written as an SSE error event by the pipeline.
			log.Printf("[ERROR] stream error model=%s provider=%s: %v",
				resolvedModel, provider.Name(), err)
		}
		return
	}

	resp, err := provider.Complete(r.Context(), &req)
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Inference failed: "+err.Error())
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
