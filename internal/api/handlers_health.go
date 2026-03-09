package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cortex/internal/inference"
	"cortex/internal/store"
	"cortex/pkg/types"

	"github.com/go-chi/chi/v5"
)

// HealthHandler handles GET /api/health.
// It reports system status including database connectivity and provider availability.
type HealthHandler struct {
	store     store.Store
	registry  *inference.ProviderRegistry
	version   string
	startTime time.Time
}

// NewHealthHandler creates a new HealthHandler. Both store and registry may be nil
// (e.g. during early startup before subsystems are wired).
func NewHealthHandler(s store.Store, reg *inference.ProviderRegistry, version string) *HealthHandler {
	return &HealthHandler{
		store:     s,
		registry:  reg,
		version:   version,
		startTime: time.Now(),
	}
}

// RegisterRoutes mounts the health endpoint on the given router.
// The health endpoint does NOT require authentication — it is intended
// for load-balancer health checks.
func (h *HealthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/health", h.HandleHealth)
}

// HandleHealth responds with a types.HealthResponse describing the current
// state of the database and every registered inference provider.
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := int64(time.Since(h.startTime).Seconds())

	dbComponent := h.checkDatabase(r.Context())
	providerComponents := h.checkProviders(r.Context())

	status := h.overallStatus(dbComponent, providerComponents)

	resp := types.HealthResponse{
		Status:    status,
		Version:   h.version,
		Uptime:    uptime,
		Database:  dbComponent,
		Providers: providerComponents,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// checkDatabase pings the store and measures latency.
// If the store is nil (not yet wired), it returns an "ok" component with zero latency.
func (h *HealthHandler) checkDatabase(ctx context.Context) types.HealthComponent {
	if h.store == nil {
		return types.HealthComponent{
			Status:  "ok",
			Latency: 0,
		}
	}

	start := time.Now()
	err := h.store.Ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return types.HealthComponent{
			Status:  "error",
			Latency: latency,
			Error:   err.Error(),
		}
	}

	return types.HealthComponent{
		Status:  "ok",
		Latency: latency,
	}
}

// checkProviders queries each registered provider's ListModels with a 5-second
// timeout and measures latency. If the registry is nil or empty, an empty map
// is returned.
func (h *HealthHandler) checkProviders(ctx context.Context) map[string]types.HealthComponent {
	components := make(map[string]types.HealthComponent)

	if h.registry == nil {
		return components
	}

	providers := h.registry.Providers()
	if len(providers) == 0 {
		return components
	}

	for name, provider := range providers {
		components[name] = h.checkSingleProvider(ctx, provider)
	}

	return components
}

// checkSingleProvider calls ListModels with a 5-second timeout and returns
// the resulting HealthComponent.
func (h *HealthHandler) checkSingleProvider(ctx context.Context, provider inference.InferenceProvider) types.HealthComponent {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err := provider.ListModels(timeoutCtx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return types.HealthComponent{
			Status:  "error",
			Latency: latency,
			Error:   err.Error(),
		}
	}

	return types.HealthComponent{
		Status:  "ok",
		Latency: latency,
	}
}

// overallStatus determines the aggregate status string.
//
//   - "ok"       — DB healthy AND at least one provider connected
//   - "degraded" — DB healthy BUT all providers offline/errored
//   - "error"    — DB unreachable
func (h *HealthHandler) overallStatus(db types.HealthComponent, providers map[string]types.HealthComponent) string {
	if db.Status == "error" {
		return "error"
	}

	// If there are no providers at all, consider it "ok" (nothing to be degraded about).
	if len(providers) == 0 {
		return "ok"
	}

	for _, p := range providers {
		if p.Status == "ok" {
			return "ok"
		}
	}

	// DB is healthy but every provider errored.
	return "degraded"
}
