package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cortex/internal/inference"
	"cortex/internal/store"
	"cortex/pkg/types"

	"github.com/go-chi/chi/v5"
)

// --- Mock Store ---

type mockStore struct {
	pingErr error
}

func (m *mockStore) Ping(ctx context.Context) error { return m.pingErr }

// Satisfy the rest of store.Store with stubs.
func (m *mockStore) Migrate(ctx context.Context) error                        { return nil }
func (m *mockStore) Close() error                                             { return nil }
func (m *mockStore) CreateSession(ctx context.Context, s *store.Session) error { return nil }
func (m *mockStore) GetSession(ctx context.Context, id string) (*store.Session, error) {
	return nil, nil
}
func (m *mockStore) ListSessions(ctx context.Context, p store.SessionListParams) ([]store.Session, error) {
	return nil, nil
}
func (m *mockStore) UpdateSession(ctx context.Context, s *store.Session) error { return nil }
func (m *mockStore) DeleteSession(ctx context.Context, id string) error        { return nil }
func (m *mockStore) CreateMessage(ctx context.Context, msg *store.Message) error { return nil }
func (m *mockStore) GetMessage(ctx context.Context, id string) (*store.Message, error) {
	return nil, nil
}
func (m *mockStore) ListMessages(ctx context.Context, p store.MessageListParams) ([]store.Message, error) {
	return nil, nil
}
func (m *mockStore) DeactivateMessages(ctx context.Context, sessionID string, ids []string) error {
	return nil
}
func (m *mockStore) UpsertProvider(ctx context.Context, p *store.Provider) error { return nil }
func (m *mockStore) GetProvider(ctx context.Context, id string) (*store.Provider, error) {
	return nil, nil
}
func (m *mockStore) ListProviders(ctx context.Context) ([]store.Provider, error) { return nil, nil }
func (m *mockStore) DeleteProvider(ctx context.Context, id string) error          { return nil }

// --- Mock InferenceProvider ---

type mockProvider struct {
	name      string
	models    []types.ModelInfo
	listErr   error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	return m.models, m.listErr
}
func (m *mockProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	return nil
}
func (m *mockProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *mockProvider) CountTokens(messages []types.ChatMessage) (int, error) { return 0, nil }
func (m *mockProvider) Capabilities(model string) inference.ModelCapabilities {
	return inference.DefaultCapabilities
}

// --- Helper ---

func doHealthRequest(t *testing.T, h *HealthHandler) *types.HealthResponse {
	t.Helper()

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp types.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return &resp
}

// --- Tests ---

func TestHealth_NoStoreNoProviders(t *testing.T) {
	h := NewHealthHandler(nil, nil, "test-v1")

	resp := doHealthRequest(t, h)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Database.Status != "ok" {
		t.Errorf("expected database status 'ok', got %q", resp.Database.Status)
	}
	if len(resp.Providers) != 0 {
		t.Errorf("expected empty providers map, got %d entries", len(resp.Providers))
	}
}

func TestHealth_WorkingStore(t *testing.T) {
	ms := &mockStore{pingErr: nil}
	h := NewHealthHandler(ms, nil, "test-v2")

	resp := doHealthRequest(t, h)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Database.Status != "ok" {
		t.Errorf("expected database status 'ok', got %q", resp.Database.Status)
	}
	// Latency should be non-negative (it's a real ping, could be 0ms).
	if resp.Database.Latency < 0 {
		t.Errorf("expected non-negative latency, got %d", resp.Database.Latency)
	}
}

func TestHealth_FailingStore(t *testing.T) {
	ms := &mockStore{pingErr: errors.New("connection refused")}
	h := NewHealthHandler(ms, nil, "test-v3")

	resp := doHealthRequest(t, h)

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if resp.Database.Status != "error" {
		t.Errorf("expected database status 'error', got %q", resp.Database.Status)
	}
	if resp.Database.Error == "" {
		t.Error("expected database error message, got empty string")
	}
}

func TestHealth_VersionAndUptime(t *testing.T) {
	h := NewHealthHandler(nil, nil, "1.2.3")
	// Ensure some uptime has elapsed.
	h.startTime = time.Now().Add(-10 * time.Second)

	resp := doHealthRequest(t, h)

	if resp.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", resp.Version)
	}
	if resp.Uptime < 10 {
		t.Errorf("expected uptime >= 10s, got %d", resp.Uptime)
	}
}

func TestHealth_WithWorkingProvider(t *testing.T) {
	reg := inference.NewProviderRegistry()
	reg.Register(&mockProvider{
		name:   "ollama",
		models: []types.ModelInfo{{ID: "llama3", Object: "model"}},
	})

	h := NewHealthHandler(nil, reg, "test-v4")

	resp := doHealthRequest(t, h)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	prov, ok := resp.Providers["ollama"]
	if !ok {
		t.Fatal("expected 'ollama' in providers map")
	}
	if prov.Status != "ok" {
		t.Errorf("expected provider status 'ok', got %q", prov.Status)
	}
}

func TestHealth_AllProvidersErrored_Degraded(t *testing.T) {
	ms := &mockStore{pingErr: nil}
	reg := inference.NewProviderRegistry()
	reg.Register(&mockProvider{
		name:    "badprovider",
		listErr: errors.New("connection timeout"),
	})

	h := NewHealthHandler(ms, reg, "test-v5")

	resp := doHealthRequest(t, h)

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got %q", resp.Status)
	}
	prov, ok := resp.Providers["badprovider"]
	if !ok {
		t.Fatal("expected 'badprovider' in providers map")
	}
	if prov.Status != "error" {
		t.Errorf("expected provider status 'error', got %q", prov.Status)
	}
}

func TestHealth_MixedProviders_StillOK(t *testing.T) {
	reg := inference.NewProviderRegistry()
	reg.Register(&mockProvider{
		name:   "good",
		models: []types.ModelInfo{{ID: "m1", Object: "model"}},
	})
	reg.Register(&mockProvider{
		name:    "bad",
		listErr: errors.New("offline"),
	})

	ms := &mockStore{pingErr: nil}
	h := NewHealthHandler(ms, reg, "test-v6")

	resp := doHealthRequest(t, h)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok' (at least one provider healthy), got %q", resp.Status)
	}
}
