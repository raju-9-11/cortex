package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"forge/internal/inference"
	"forge/internal/session"
	"forge/internal/store"
	"forge/pkg/types"

	"github.com/go-chi/chi/v5"
)

// --- Mock session manager ---

type mockManager struct {
	sessions map[string]*store.Session
	messages map[string][]store.Message
	createFn func(ctx context.Context, params session.CreateParams) (*store.Session, error)
}

func newMockManager() *mockManager {
	return &mockManager{
		sessions: make(map[string]*store.Session),
		messages: make(map[string][]store.Message),
	}
}

func (m *mockManager) Create(ctx context.Context, params session.CreateParams) (*store.Session, error) {
	if m.createFn != nil {
		return m.createFn(ctx, params)
	}
	now := time.Now().UTC()
	title := params.Title
	if title == "" {
		title = "New Chat"
	}
	model := params.Model
	if model == "" {
		model = "default-model"
	}
	s := &store.Session{
		ID:        "sess_test123",
		UserID:    params.UserID,
		Title:     title,
		Model:     model,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		LastAccess: now,
	}
	m.sessions[s.ID] = s
	return s, nil
}

func (m *mockManager) Get(ctx context.Context, id string) (*store.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func (m *mockManager) List(ctx context.Context, userID string) ([]store.Session, error) {
	var result []store.Session
	for _, s := range m.sessions {
		if s.UserID == userID {
			result = append(result, *s)
		}
	}
	return result, nil
}

func (m *mockManager) Update(ctx context.Context, id string, params session.UpdateParams) (*store.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	if params.Title != nil {
		s.Title = *params.Title
	}
	if params.Model != nil {
		s.Model = *params.Model
	}
	if params.SystemPrompt != nil {
		s.SystemPrompt = *params.SystemPrompt
	}
	if params.Status != nil {
		s.Status = *params.Status
	}
	s.UpdatedAt = time.Now().UTC()
	return s, nil
}

func (m *mockManager) Delete(ctx context.Context, id string) error {
	if _, ok := m.sessions[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.sessions, id)
	return nil
}

func (m *mockManager) AddMessage(ctx context.Context, sessionID string, msg *store.Message) error {
	msg.ID = "msg_test_" + msg.Role
	msg.SessionID = sessionID
	msg.CreatedAt = time.Now().UTC()
	msg.IsActive = true
	m.messages[sessionID] = append(m.messages[sessionID], *msg)
	return nil
}

func (m *mockManager) GetMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	return m.messages[sessionID], nil
}

// --- Mock provider that supports Complete ---

type mockCompleteProvider struct {
	inference.InferenceProvider
	name    string
	content string
}

func (p *mockCompleteProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return &types.ChatCompletionResponse{
		ID:      "resp_test",
		Object:  "chat.completion",
		Model:   req.Model,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ChatMessage{
				Role:    "assistant",
				Content: p.content,
			},
			FinishReason: "stop",
		}},
	}, nil
}

func (p *mockCompleteProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	defer close(out)
	out <- types.StreamEvent{Type: types.EventContentDelta, Delta: p.content}
	out <- types.StreamEvent{Type: types.EventContentDone, FinishReason: "stop"}
	return nil
}

func (p *mockCompleteProvider) CountTokens(messages []types.ChatMessage) (int, error) { return 10, nil }
func (p *mockCompleteProvider) Capabilities(model string) inference.ModelCapabilities {
	return inference.DefaultCapabilities
}
func (p *mockCompleteProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	return []types.ModelInfo{{ID: "test-model", Provider: p.name}}, nil
}
func (p *mockCompleteProvider) Name() string { return p.name }

// --- Helper to build router with handler ---

func setupTestRouter(mgr session.Manager, reg *inference.ProviderRegistry) *chi.Mux {
	r := chi.NewRouter()
	h := NewSessionHandler(mgr, reg)
	h.RegisterRoutes(r)
	return r
}

// --- Tests ---

func TestListSessions_Empty(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp types.SessionListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total, got %d", resp.Total)
	}
	if resp.HasMore {
		t.Error("expected has_more=false")
	}
	if resp.Data == nil {
		t.Error("expected non-nil data slice")
	}
}

func TestListSessions_WithSessions(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_1"] = &store.Session{
		ID: "sess_1", UserID: "default", Title: "Chat 1", Model: "m1",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}
	mgr.sessions["sess_2"] = &store.Session{
		ID: "sess_2", UserID: "default", Title: "Chat 2", Model: "m2",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp types.SessionListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Errorf("expected 2 total, got %d", resp.Total)
	}
}

func TestCreateSession_Success(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{"title":"My Chat","model":"llama3"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var sess types.Session
	json.NewDecoder(w.Body).Decode(&sess)
	if sess.Title != "My Chat" {
		t.Errorf("expected title 'My Chat', got %q", sess.Title)
	}
	if sess.Model != "llama3" {
		t.Errorf("expected model 'llama3', got %q", sess.Model)
	}
}

func TestCreateSession_BadJSON(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp types.APIErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Code != types.ErrCodeMalformedJSON {
		t.Errorf("expected code %q, got %q", types.ErrCodeMalformedJSON, errResp.Error.Code)
	}
}

func TestCreateSession_BadContentType(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `<xml/>`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "text/xml")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
}

func TestGetSession_Success(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_abc"] = &store.Session{
		ID: "sess_abc", UserID: "default", Title: "Test", Model: "m1",
		Status: "active", SystemPrompt: "Be helpful", TokenCount: 100,
		MessageCount: 5, CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}
	mgr.messages["sess_abc"] = []store.Message{
		{ID: "msg_1", SessionID: "sess_abc", Role: "user", Content: "Hi", IsActive: true, CreatedAt: now},
		{ID: "msg_2", SessionID: "sess_abc", Role: "assistant", Content: "Hello!", IsActive: true, CreatedAt: now},
	}

	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess_abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp types.SessionDetailResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Session.ID != "sess_abc" {
		t.Errorf("expected session ID 'sess_abc', got %q", resp.Session.ID)
	}
	if len(resp.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(resp.Messages))
	}
}

func TestGetSession_NotFound(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var errResp types.APIErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Code != types.ErrCodeSessionNotFound {
		t.Errorf("expected code %q, got %q", types.ErrCodeSessionNotFound, errResp.Error.Code)
	}
}

func TestUpdateSession_Success(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_upd"] = &store.Session{
		ID: "sess_upd", UserID: "default", Title: "Old Title", Model: "m1",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{"title":"New Title"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/sess_upd", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sess types.Session
	json.NewDecoder(w.Body).Decode(&sess)
	if sess.Title != "New Title" {
		t.Errorf("expected title 'New Title', got %q", sess.Title)
	}
}

func TestUpdateSession_NotFound(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{"title":"X"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/missing", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteSession_Success(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_del"] = &store.Session{
		ID: "sess_del", UserID: "default", Title: "Delete Me", Model: "m1",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/sess_del", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	if _, exists := mgr.sessions["sess_del"]; exists {
		t.Error("expected session to be deleted")
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/nope", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSendMessage_NonStreaming(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_msg"] = &store.Session{
		ID: "sess_msg", UserID: "default", Title: "Chat", Model: "test-model",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	provider := &mockCompleteProvider{name: "test", content: "Hello back!"}
	reg := inference.NewProviderRegistry()
	reg.Register(provider)

	router := setupTestRouter(mgr, reg)

	body := `{"content":"Hello","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/sess_msg/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp types.SendMessageResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.UserMessage.Role != "user" {
		t.Errorf("expected user role, got %q", resp.UserMessage.Role)
	}
	if resp.UserMessage.Content != "Hello" {
		t.Errorf("expected user content 'Hello', got %q", resp.UserMessage.Content)
	}
	if resp.AssistantMessage.Role != "assistant" {
		t.Errorf("expected assistant role, got %q", resp.AssistantMessage.Role)
	}
	if resp.AssistantMessage.Content != "Hello back!" {
		t.Errorf("expected assistant content 'Hello back!', got %q", resp.AssistantMessage.Content)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{"content":"","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/sess_msg/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp types.APIErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Code != types.ErrCodeValidation {
		t.Errorf("expected code %q, got %q", types.ErrCodeValidation, errResp.Error.Code)
	}
}

func TestSendMessage_SessionNotFound(t *testing.T) {
	mgr := newMockManager()
	reg := inference.NewProviderRegistry()
	router := setupTestRouter(mgr, reg)

	body := `{"content":"Hi","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/nonexistent/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSendMessage_ProviderNotFound(t *testing.T) {
	mgr := newMockManager()
	now := time.Now().UTC()
	mgr.sessions["sess_nop"] = &store.Session{
		ID: "sess_nop", UserID: "default", Title: "Chat", Model: "unknown-model",
		Status: "active", CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	reg := inference.NewProviderRegistry() // empty registry — no providers
	router := setupTestRouter(mgr, reg)

	body := `{"content":"Hi","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/sess_nop/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var errResp types.APIErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Code != types.ErrCodeProviderNotFound {
		t.Errorf("expected code %q, got %q", types.ErrCodeProviderNotFound, errResp.Error.Code)
	}
}

func TestModelConversion_SessionToAPI(t *testing.T) {
	now := time.Now().UTC()
	s := &store.Session{
		ID: "sess_1", UserID: "default", Title: "Chat", Model: "m1",
		SystemPrompt: "Be helpful", Status: "active", TokenCount: 100,
		MessageCount: 5, CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	api := storeSessionToAPI(s)
	if api.ID != s.ID || api.Title != s.Title || api.Model != s.Model {
		t.Error("session conversion mismatch")
	}
	if api.SystemPrompt != s.SystemPrompt || api.TokenCount != s.TokenCount {
		t.Error("session conversion mismatch on detail fields")
	}
}

func TestModelConversion_SessionToListItem(t *testing.T) {
	now := time.Now().UTC()
	s := store.Session{
		ID: "sess_1", UserID: "default", Title: "Chat", Model: "m1",
		SystemPrompt: "Be helpful", Status: "active", TokenCount: 100,
		MessageCount: 5, CreatedAt: now, UpdatedAt: now, LastAccess: now,
	}

	item := storeSessionToListItem(s)
	if item.ID != s.ID || item.Title != s.Title || item.Model != s.Model {
		t.Error("list item conversion mismatch")
	}
	if item.MessageCount != s.MessageCount {
		t.Error("list item message count mismatch")
	}
}

func TestModelConversion_MessageToAPI(t *testing.T) {
	now := time.Now().UTC()
	parentID := "msg_0"
	m := store.Message{
		ID: "msg_1", SessionID: "sess_1", ParentID: &parentID,
		Role: "user", Content: "Hi", TokenCount: 5,
		IsActive: true, Pinned: false, Model: "m1", CreatedAt: now,
	}

	api := storeMessageToAPI(m)
	if api.ID != m.ID || api.Content != m.Content || api.Role != m.Role {
		t.Error("message conversion mismatch")
	}
	if api.ParentID == nil || *api.ParentID != parentID {
		t.Error("message parent_id conversion mismatch")
	}
}

// Verify unused import check - errors package is used by mockManager.
var _ = errors.New
