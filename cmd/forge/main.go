package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"forge/internal/auth"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/inference"
	"forge/internal/server"
	"forge/internal/session"
	"forge/internal/store"
)

var version = "dev"

func main() {
	// Dispatch subcommands.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "chat":
			chatCmd()
			return
		}
	}

	// Default: start the server.
	serverCmd()
}

// chatCmd implements the "forge chat" interactive REPL.
func chatCmd() {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	model := fs.String("model", "", "Model to use")
	sessionID := fs.String("session", "", "Resume session by ID")
	system := fs.String("system", "", "System prompt for new sessions")
	fs.Parse(os.Args[2:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := setupProviders(cfg)

	sessionMgr := session.NewManager(db, cfg.DefaultModel)

	useModel := cfg.DefaultModel
	if *model != "" {
		useModel = *model
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cli.RunREPL(ctx, registry, sessionMgr, useModel, *sessionID, *system, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// serverCmd starts the Forge HTTP server (default behavior).
func serverCmd() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Version = version

	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	registry := setupProviders(cfg)

	sessionMgr := session.NewManager(db, cfg.DefaultModel)

	authMiddleware := auth.NewMiddleware(cfg.APIKey)

	// Startup banner
	models := registry.ListAllModels(context.Background())
	fmt.Printf("\n🔥 Forge %s\n", cfg.Version)
	fmt.Printf("  API:    http://localhost%s/v1/chat/completions\n", cfg.Addr)
	fmt.Printf("  Health: http://localhost%s/api/health\n", cfg.Addr)
	fmt.Printf("  Chat UI: http://localhost%s/chat\n", cfg.Addr)
	fmt.Println()
	fmt.Printf("  Providers: %d registered\n", len(registry.Providers()))
	fmt.Printf("  Models:    %d available\n", len(models))
	fmt.Println()

	srv := server.New(cfg, registry, db, sessionMgr, authMiddleware)
	srv.StartAndServe()
}

// setupProviders creates a ProviderRegistry, registers all configured
// providers, and falls back to mocks if none are available.
func setupProviders(cfg *config.Config) *inference.ProviderRegistry {
	registry := inference.NewProviderRegistry()

	// Ollama — auto-detect at configured URL
	ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
	if ollamaProvider.Probe(context.Background()) {
		registry.Register(ollamaProvider)
		log.Printf("✅ Detected Ollama at %s", cfg.OllamaURL)
	} else {
		log.Printf("⚠️  Ollama not detected at %s", cfg.OllamaURL)
	}

	// OpenAI-compatible providers
	if cfg.OpenAIKey != "" {
		registry.Register(inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey))
	}
	if cfg.QwenKey != "" {
		registry.Register(inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey))
	}
	if cfg.LlamaKey != "" {
		registry.Register(inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey))
	}
	if cfg.MinimaxKey != "" {
		registry.Register(inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey))
	}
	if cfg.OSSKey != "" {
		registry.Register(inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey))
	}

	// Set default provider from config
	if cfg.DefaultProvider != "" {
		registry.SetDefault(cfg.DefaultProvider)
	}

	// Refresh model map
	if err := registry.RefreshModelMap(context.Background()); err != nil {
		log.Printf("[WARN] Initial model map refresh failed: %v", err)
	}

	// Fall back to mocks if no real providers are configured
	if len(registry.Providers()) == 0 {
		log.Println("No providers configured — using mock providers")
		registry.Register(inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"}))
		registry.Register(inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"}))
		registry.Register(inference.NewMockProvider("minimax", []string{"Minimax", " ", "says", " ", "hello", "!"}))
		registry.Register(inference.NewMockProvider("oss", []string{"OSS", " ", "power", "!"}))
		registry.SetDefault("qwen")
	}

	return registry
}
