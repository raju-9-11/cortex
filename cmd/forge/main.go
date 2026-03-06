package main

import (
	"log"

	"forge/internal/config"
	"forge/internal/inference"
	"forge/internal/server"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Version = version

	providers := make(map[string]inference.InferenceProvider)

	if cfg.OpenAIKey != "" {
		providers["openai"] = inference.NewOpenAIProvider("openai", cfg.OpenAIBaseURL, cfg.OpenAIKey)
	}
	if cfg.QwenKey != "" {
		providers["qwen"] = inference.NewOpenAIProvider("qwen", cfg.QwenBaseURL, cfg.QwenKey)
	}
	if cfg.LlamaKey != "" {
		providers["llama"] = inference.NewOpenAIProvider("llama", cfg.LlamaBaseURL, cfg.LlamaKey)
	}
	if cfg.MinimaxKey != "" {
		providers["minimax"] = inference.NewOpenAIProvider("minimax", cfg.MinimaxBaseURL, cfg.MinimaxKey)
	}
	if cfg.OSSKey != "" {
		providers["oss"] = inference.NewOpenAIProvider("oss", cfg.OSSBaseURL, cfg.OSSKey)
	}

	// For testing and fallback, inject mock providers
	if len(providers) == 0 {
		providers["qwen"] = inference.NewMockProvider("qwen", []string{"Hi", " I", " am", " Qwen", "!"})
		providers["llama"] = inference.NewMockProvider("llama", []string{"Llama", " ", "here", "!"})
		providers["minimax"] = inference.NewMockProvider("minimax", []string{"Minimax", " ", "says", " ", "hello", "!"})
		providers["oss"] = inference.NewMockProvider("oss", []string{"OSS", " ", "power", "!"})
	}

	srv := server.New(cfg, providers)
	srv.StartAndServe()
}
