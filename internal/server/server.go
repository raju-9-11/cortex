package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"forge/internal/api"
	"forge/internal/config"
	"forge/internal/inference"
	"forge/internal/session"
	"forge/internal/store"
	"forge/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg        *config.Config
	registry   *inference.ProviderRegistry
	store      store.Store
	sessionMgr session.Manager
	httpServer *http.Server
}

func New(cfg *config.Config, registry *inference.ProviderRegistry, st store.Store, sessionMgr session.Manager, authMiddleware func(http.Handler) http.Handler) *Server {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg))

	// Health endpoint — public, no auth required (load-balancer checks)
	healthHandler := api.NewHealthHandler(st, registry, cfg.Version)
	r.Group(func(r chi.Router) {
		healthHandler.RegisterRoutes(r)
	})

	// Protected routes — auth middleware applied
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		// Chat completions (OpenAI-compatible)
		chatRouter := api.NewRouter(registry)
		chatRouter.SetupRoutes(r)

		// Session CRUD
		sessionHandler := api.NewSessionHandler(sessionMgr, registry)
		sessionHandler.RegisterRoutes(r)
	})

	// Web UI — public, mounted at /chat, served last so API routes take precedence.
	// http.StripPrefix removes "/chat" so the embed handler sees clean paths
	// (e.g. "/css/styles.css" instead of "/chat/css/styles.css").
	r.Mount("/chat", http.StripPrefix("/chat", web.Handler()))

	return &Server{
		cfg:        cfg,
		registry:   registry,
		store:      st,
		sessionMgr: sessionMgr,
		httpServer: &http.Server{
			Addr:    cfg.Addr,
			Handler: r,
		},
	}
}

// corsMiddleware returns a middleware that sets CORS headers based on config.
func corsMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := "*"
			if len(cfg.CORSOrigins) > 0 {
				origin = strings.Join(cfg.CORSOrigins, ", ")
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) StartAndServe() {
	go func() {
		log.Printf("Starting server on %s", s.cfg.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
