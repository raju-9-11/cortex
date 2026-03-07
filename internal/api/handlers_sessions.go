package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"forge/internal/inference"
	"forge/internal/session"
	"forge/internal/store"
	"forge/internal/streaming"
	"forge/pkg/types"

	"github.com/go-chi/chi/v5"
)

// SessionHandler handles CRUD operations for sessions and message sending.
type SessionHandler struct {
	manager  session.Manager
	registry *inference.ProviderRegistry
}

// NewSessionHandler creates a new SessionHandler with the given session manager
// and provider registry.
func NewSessionHandler(mgr session.Manager, reg *inference.ProviderRegistry) *SessionHandler {
	return &SessionHandler{
		manager:  mgr,
		registry: reg,
	}
}

// RegisterRoutes mounts session endpoints on the given chi router.
func (h *SessionHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/sessions", func(r chi.Router) {
		r.Get("/", h.HandleListSessions)
		r.Post("/", h.HandleCreateSession)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.HandleGetSession)
			r.Patch("/", h.HandleUpdateSession)
			r.Delete("/", h.HandleDeleteSession)
			r.Post("/messages", h.HandleSendMessage)
		})
	})
}

// HandleListSessions handles GET /api/sessions.
// Returns all sessions for the hardcoded "default" user as a lightweight list.
func (h *SessionHandler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.manager.List(r.Context(), "default")
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to list sessions: "+err.Error())
		return
	}

	items := make([]types.SessionListItem, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, storeSessionToListItem(s))
	}

	resp := types.SessionListResponse{
		Data:    items,
		Total:   len(items),
		HasMore: false,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleCreateSession handles POST /api/sessions.
// Creates a new session with optional model, title, and system prompt.
func (h *SessionHandler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		types.WriteError(w, http.StatusUnsupportedMediaType, types.ErrCodeUnsupportedMedia,
			types.ErrorTypeInvalidRequest, "Content-Type must be application/json")
		return
	}

	var req types.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.WriteError(w, http.StatusBadRequest, types.ErrCodeMalformedJSON,
			types.ErrorTypeInvalidRequest, "Invalid JSON in request body: "+err.Error())
		return
	}

	sess, err := h.manager.Create(r.Context(), session.CreateParams{
		Model:        req.Model,
		Title:        req.Title,
		SystemPrompt: req.SystemPrompt,
		UserID:       "default",
	})
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to create session: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(storeSessionToAPI(sess))
}

// HandleGetSession handles GET /api/sessions/{id}.
// Returns the session with its full message history.
func (h *SessionHandler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := h.manager.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			types.WriteError(w, http.StatusNotFound, types.ErrCodeSessionNotFound,
				types.ErrorTypeNotFound, fmt.Sprintf("Session %q not found", id))
			return
		}
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to get session: "+err.Error())
		return
	}

	messages, err := h.manager.GetMessages(r.Context(), id)
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to get messages: "+err.Error())
		return
	}

	apiMessages := make([]types.Message, 0, len(messages))
	for _, m := range messages {
		apiMessages = append(apiMessages, storeMessageToAPI(m))
	}

	resp := types.SessionDetailResponse{
		Session:  *storeSessionToAPI(sess),
		Messages: apiMessages,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleUpdateSession handles PATCH /api/sessions/{id}.
// Applies merge-patch updates to the session.
func (h *SessionHandler) HandleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		types.WriteError(w, http.StatusUnsupportedMediaType, types.ErrCodeUnsupportedMedia,
			types.ErrorTypeInvalidRequest, "Content-Type must be application/json")
		return
	}

	var req types.UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.WriteError(w, http.StatusBadRequest, types.ErrCodeMalformedJSON,
			types.ErrorTypeInvalidRequest, "Invalid JSON in request body: "+err.Error())
		return
	}

	sess, err := h.manager.Update(r.Context(), id, session.UpdateParams{
		Title:        req.Title,
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		Status:       req.Status,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			types.WriteError(w, http.StatusNotFound, types.ErrCodeSessionNotFound,
				types.ErrorTypeNotFound, fmt.Sprintf("Session %q not found", id))
			return
		}
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to update session: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(storeSessionToAPI(sess))
}

// HandleDeleteSession handles DELETE /api/sessions/{id}.
// Removes the session and all associated messages.
func (h *SessionHandler) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.manager.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			types.WriteError(w, http.StatusNotFound, types.ErrCodeSessionNotFound,
				types.ErrorTypeNotFound, fmt.Sprintf("Session %q not found", id))
			return
		}
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to delete session: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleSendMessage handles POST /api/sessions/{id}/messages.
// Creates a user message, runs inference, and returns (or streams) the response.
func (h *SessionHandler) HandleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		types.WriteError(w, http.StatusUnsupportedMediaType, types.ErrCodeUnsupportedMedia,
			types.ErrorTypeInvalidRequest, "Content-Type must be application/json")
		return
	}

	var req types.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		types.WriteError(w, http.StatusBadRequest, types.ErrCodeMalformedJSON,
			types.ErrorTypeInvalidRequest, "Invalid JSON in request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		types.WriteError(w, http.StatusBadRequest, types.ErrCodeValidation,
			types.ErrorTypeInvalidRequest, "Message content must not be empty")
		return
	}

	// Verify session exists.
	sess, err := h.manager.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			types.WriteError(w, http.StatusNotFound, types.ErrCodeSessionNotFound,
				types.ErrorTypeNotFound, fmt.Sprintf("Session %q not found", id))
			return
		}
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to get session: "+err.Error())
		return
	}

	// Save user message.
	role := req.Role
	if role == "" {
		role = types.RoleUser
	}
	userMsg := &store.Message{
		Role:     role,
		Content:  req.Content,
		ParentID: req.ParentID,
	}
	if err := h.manager.AddMessage(r.Context(), id, userMsg); err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to save user message: "+err.Error())
		return
	}

	// Resolve provider for the session's model.
	provider, resolvedModel, err := h.registry.Resolve(sess.Model)
	if err != nil {
		types.WriteError(w, http.StatusNotFound, types.ErrCodeProviderNotFound,
			types.ErrorTypeNotFound, "No provider found for model: "+sess.Model)
		return
	}

	// Build chat completion request from session history.
	chatReq, err := h.buildChatRequest(r, id, sess, resolvedModel)
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to build chat request: "+err.Error())
		return
	}

	if req.Stream {
		h.handleStreamingMessage(w, r, id, sess, provider, chatReq)
		return
	}

	h.handleNonStreamingMessage(w, r, id, sess, provider, chatReq, userMsg)
}

// handleStreamingMessage runs a streaming inference and saves the assistant
// message after the stream completes.
func (h *SessionHandler) handleStreamingMessage(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	sess *store.Session,
	provider inference.InferenceProvider,
	chatReq *types.ChatCompletionRequest,
) {
	// Use the SSE pipeline for streaming. We need to capture the content
	// afterwards, so we tee the events by wrapping the provider in an
	// accumulating interceptor.
	accumulator := &contentAccumulator{}
	wrappedProvider := &accumulatingProvider{
		InferenceProvider: provider,
		accumulator:       accumulator,
	}

	chatReq.Stream = true
	pipeline := streaming.NewPipeline(wrappedProvider)
	if err := pipeline.Stream(r.Context(), chatReq, w); err != nil {
		// Headers already sent — error was written as SSE error event by pipeline.
		log.Printf("[ERROR] stream error session=%s model=%s: %v",
			sessionID, sess.Model, err)
	}

	// Save the accumulated assistant message.
	// IMPORTANT: Use a background context, not r.Context(). The HTTP request
	// context is cancelled when the client disconnects, which may happen before
	// or immediately after the stream completes. We must still persist the
	// assistant response to avoid data loss.
	content := accumulator.Content()
	if content != "" {
		assistantMsg := &store.Message{
			Role:    types.RoleAssistant,
			Content: content,
			Model:   sess.Model,
		}
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer saveCancel()
		if err := h.manager.AddMessage(saveCtx, sessionID, assistantMsg); err != nil {
			log.Printf("[ERROR] failed to save streamed assistant message session=%s: %v",
				sessionID, err)
		}
	}
}

// handleNonStreamingMessage runs a non-streaming inference and returns
// both the user and assistant messages.
func (h *SessionHandler) handleNonStreamingMessage(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	sess *store.Session,
	provider inference.InferenceProvider,
	chatReq *types.ChatCompletionRequest,
	userMsg *store.Message,
) {
	chatReq.Stream = false
	resp, err := provider.Complete(r.Context(), chatReq)
	if err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Inference failed: "+err.Error())
		return
	}

	// Extract assistant content from the response.
	var assistantContent string
	if len(resp.Choices) > 0 {
		if content, ok := resp.Choices[0].Message.Content.(string); ok {
			assistantContent = content
		}
	}

	// Save the assistant message.
	assistantMsg := &store.Message{
		Role:    types.RoleAssistant,
		Content: assistantContent,
		Model:   sess.Model,
	}
	if err := h.manager.AddMessage(r.Context(), sessionID, assistantMsg); err != nil {
		types.WriteError(w, http.StatusInternalServerError, types.ErrCodeInternalError,
			types.ErrorTypeServer, "Failed to save assistant message: "+err.Error())
		return
	}

	sendResp := types.SendMessageResponse{
		UserMessage:      storeMessageToAPI(*userMsg),
		AssistantMessage: storeMessageToAPI(*assistantMsg),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sendResp)
}

// buildChatRequest constructs a ChatCompletionRequest from the session's
// message history, including the system prompt if set.
func (h *SessionHandler) buildChatRequest(
	r *http.Request,
	sessionID string,
	sess *store.Session,
	resolvedModel string,
) (*types.ChatCompletionRequest, error) {
	messages, err := h.manager.GetMessages(r.Context(), sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	chatMessages := make([]types.ChatMessage, 0, len(messages)+1)

	// Prepend system prompt if set.
	if sess.SystemPrompt != "" {
		chatMessages = append(chatMessages, types.ChatMessage{
			Role:    types.RoleSystem,
			Content: sess.SystemPrompt,
		})
	}

	for _, m := range messages {
		chatMessages = append(chatMessages, types.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return &types.ChatCompletionRequest{
		Model:    resolvedModel,
		Messages: chatMessages,
	}, nil
}

// --- Model conversion helpers ---

// storeSessionToAPI converts a store.Session to a types.Session (full detail).
func storeSessionToAPI(s *store.Session) *types.Session {
	return &types.Session{
		ID:           s.ID,
		UserID:       s.UserID,
		Title:        s.Title,
		Model:        s.Model,
		SystemPrompt: s.SystemPrompt,
		Status:       s.Status,
		TokenCount:   s.TokenCount,
		MessageCount: s.MessageCount,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		LastAccess:   s.LastAccess,
	}
}

// storeSessionToListItem converts a store.Session to a types.SessionListItem
// (lightweight sidebar projection).
func storeSessionToListItem(s store.Session) types.SessionListItem {
	return types.SessionListItem{
		ID:           s.ID,
		Title:        s.Title,
		Model:        s.Model,
		Status:       s.Status,
		MessageCount: s.MessageCount,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		LastAccess:   s.LastAccess,
	}
}

// storeMessageToAPI converts a store.Message to a types.Message.
func storeMessageToAPI(m store.Message) types.Message {
	return types.Message{
		ID:         m.ID,
		SessionID:  m.SessionID,
		ParentID:   m.ParentID,
		Role:       m.Role,
		Content:    m.Content,
		TokenCount: m.TokenCount,
		IsActive:   m.IsActive,
		Pinned:     m.Pinned,
		Model:      m.Model,
		CreatedAt:  m.CreatedAt,
	}
}

// --- Streaming content accumulator ---

// contentAccumulator collects content deltas from streaming events.
type contentAccumulator struct {
	content strings.Builder
}

// Content returns the accumulated content string.
func (a *contentAccumulator) Content() string {
	return a.content.String()
}

// accumulatingProvider wraps an InferenceProvider and intercepts StreamChat
// to accumulate content deltas before forwarding them.
type accumulatingProvider struct {
	inference.InferenceProvider
	accumulator *contentAccumulator
}

// StreamChat intercepts the events channel to accumulate content deltas,
// then forwards all events to the downstream consumer.
func (p *accumulatingProvider) StreamChat(
	ctx context.Context,
	req *types.ChatCompletionRequest,
	out chan<- types.StreamEvent,
) error {
	// Create an intermediate channel for the real provider.
	intermediate := make(chan types.StreamEvent, 32)

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.InferenceProvider.StreamChat(ctx, req, intermediate)
	}()

	// Forward events, accumulating content deltas.
	for event := range intermediate {
		if event.Type == types.EventContentDelta {
			p.accumulator.content.WriteString(event.Delta)
		}
		out <- event
	}
	close(out)

	return <-errCh
}

// Name delegates to the wrapped provider (needed so the pipeline sees
// the correct provider name for logging).
func (p *accumulatingProvider) Name() string {
	return p.InferenceProvider.Name()
}

// Ensure accumulatingProvider satisfies InferenceProvider at compile time.
var _ inference.InferenceProvider = (*accumulatingProvider)(nil)
