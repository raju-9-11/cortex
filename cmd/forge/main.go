package main

import (
	"context"
	"fmt"
	"log"

	"forge/internal/auth"
	"forge/internal/config"
	"forge/internal/inference"
	"forge/internal/server"
	"forge/internal/session"
	"forge/internal/store"
)

var version = "dev"

func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Version = version

	// 2. Initialize SQLite store
	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Run migrations
	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// 4. Load config file
	configPath := config.DiscoverConfigFile(cfg.ConfigFilePath)
	if configPath != "" {
		log.Printf("Loading config from %s", configPath)
	}
	fileConfig, err := config.LoadConfigFile(configPath)
	if err != nil {
		log.Printf("[WARN] Failed to load config file: %v", err)
	}

	// Apply config file defaults (env vars take precedence)
	if fileConfig != nil {
		// Only apply if env var wasn't explicitly set (still at default value)
		if cfg.DefaultProvider == "qwen" && fileConfig.DefaultProvider != "" {
			cfg.DefaultProvider = fileConfig.DefaultProvider
		}
		if cfg.DefaultModel == "qwen2.5:0.5b" && fileConfig.DefaultModel != "" {
			cfg.DefaultModel = fileConfig.DefaultModel
		}
	}

	// 5. Create provider registry
	registry := inference.NewProviderRegistry()

	// 6. Register providers from config file
	if fileConfig != nil && len(fileConfig.Providers) > 0 {
		for _, p := range fileConfig.Providers {
			if p.Name == "ollama" {
				ollamaP := inference.NewOllamaProvider(p.BaseURL)
				if ollamaP.Probe(context.Background()) {
					registry.Register(ollamaP)
					log.Printf("✅ Config file: registered Ollama at %s", p.BaseURL)
				}
			} else {
				registry.Register(inference.NewOpenAIProvider(p.Name, p.BaseURL, p.APIKey))
				log.Printf("✅ Config file: registered %s at %s", p.Name, p.BaseURL)
			}
		}
	}

	// 7. Register providers from env vars (skip if already registered via config file)
	registeredProviders := registry.Providers()

	// Ollama — auto-detect at configured URL
	if _, exists := registeredProviders["ollama"]; !exists {
		ollamaProvider := inference.NewOllamaProvider(cfg.OllamaURL)
		if ollamaProvider.Probe(context.Background()) {
			registry.Register(ollamaProvider)
			log.Printf("✅ Detected Ollama at %s", cfg.OllamaURL)
		} else {
			log.Printf("⚠️  Ollama not detected at %s", cfg.OllamaURL)
		}
	}

	// OpenAI-compatible providers
	if cfg.OpenAIKey != "" {
		if _, exists := registeredProviders["openai"]; !exists {
			registry.Register(inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey))
		}
	}
	if cfg.QwenKey != "" {
		if _, exists := registeredProviders["qwen"]; !exists {
			registry.Register(inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey))
		}
	}
	if cfg.LlamaKey != "" {
		if _, exists := registeredProviders["llama"]; !exists {
			registry.Register(inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey))
		}
	}
	if cfg.MinimaxKey != "" {
		if _, exists := registeredProviders["minimax"]; !exists {
			registry.Register(inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey))
		}
	}
	if cfg.OSSKey != "" {
		if _, exists := registeredProviders["oss"]; !exists {
			registry.Register(inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey))
		}
	}

	// Set default provider from config
	if cfg.DefaultProvider != "" {
		registry.SetDefault(cfg.DefaultProvider)
	}

	// 8. Refresh model map
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

	// 9. Create session manager
	sessionMgr := session.NewManager(db, cfg.DefaultModel)

	// 10. Create auth middleware
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

	// 11. Create and start server with all dependencies
	srv := server.New(cfg, registry, db, sessionMgr, authMiddleware)
	srv.StartAndServe()
}
