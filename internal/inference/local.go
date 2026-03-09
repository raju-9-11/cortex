package inference

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cortex/pkg/types"
)

// LocalProvider implements InferenceProvider by running llama-server as a subprocess.
type LocalProvider struct {
	mu        sync.Mutex
	modelsDir string
	binPath   string
	process   *exec.Cmd
	port      int
	ctxSize   int
	current   string // current model path
}

func NewLocalProvider(modelsDir string, ctxSize int) *LocalProvider {
	if modelsDir == "" {
		modelsDir = "./models"
	}
	if ctxSize <= 0 {
		ctxSize = 4096
	}
	return &LocalProvider{
		modelsDir: modelsDir,
		binPath:   "./bin/llama-server", // Default location
		port:      8081,
		ctxSize:   ctxSize,
	}
}

func (p *LocalProvider) Name() string {
	return "local"
}

// ensureBinary checks if llama-server exists in ./bin or PATH.
func (p *LocalProvider) ensureBinary() error {
	// 1. Check ./bin/llama-server
	if _, err := os.Stat(p.binPath); err == nil {
		return nil
	}

	// 2. Check PATH
	path, err := exec.LookPath("llama-server")
	if err == nil {
		p.binPath = path
		return nil
	}

	return fmt.Errorf("llama-server not found in ./bin or PATH. Please download it from https://github.com/ggerganov/llama.cpp/releases")
}

func (p *LocalProvider) startServer(ctx context.Context, modelPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.process != nil {
		if p.current == modelPath {
			return nil // Already running
		}
		// Different model, stop previous one
		p.stopServerLocked()
	}

	if err := p.ensureBinary(); err != nil {
		return err
	}

	fullPath := filepath.Join(p.modelsDir, modelPath)
	args := []string{
		"-m", fullPath,
		"--port", fmt.Sprintf("%d", p.port),
		"--n-gpu-layers", "0", // Default to CPU for maximum compatibility
		"--ctx-size", fmt.Sprintf("%d", p.ctxSize),
		"--parallel", "1",
	}

	log.Printf("Starting local inference: %s %s", p.binPath, strings.Join(args, " "))
	cmd := exec.CommandContext(context.Background(), p.binPath, args...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	p.process = cmd
	p.current = modelPath

	// Wait for server to be ready
	ready := false
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p.port), 1*time.Second)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
	}

	if !ready {
		p.stopServerLocked()
		return fmt.Errorf("llama-server failed to start within 30s")
	}

	return nil
}

func (p *LocalProvider) stopServerLocked() {
	if p.process != nil {
		if p.process.Process != nil {
			p.process.Process.Kill()
		}
		p.process.Wait()
		p.process = nil
		p.current = ""
	}
}

func (p *LocalProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopServerLocked()
	return nil
}

// StreamChat starts the server if needed and proxies to its OpenAI-compatible API.
func (p *LocalProvider) StreamChat(ctx context.Context, req *types.ChatCompletionRequest, out chan<- types.StreamEvent) error {
	if err := p.startServer(ctx, req.Model); err != nil {
		defer close(out)
		sendError(out, err)
		return err
	}

	// Use the OpenAI provider to proxy to the local llama-server
	proxy := NewOpenAIProvider("local", fmt.Sprintf("http://127.0.0.1:%d/v1", p.port), "not-needed")
	return proxy.StreamChat(ctx, req, out)
}

// Complete starts the server if needed and proxies to its OpenAI-compatible API.
func (p *LocalProvider) Complete(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if err := p.startServer(ctx, req.Model); err != nil {
		return nil, err
	}

	proxy := NewOpenAIProvider("local", fmt.Sprintf("http://127.0.0.1:%d/v1", p.port), "not-needed")
	return proxy.Complete(ctx, req)
}

func (p *LocalProvider) CountTokens(messages []types.ChatMessage) (int, error) {
	return EstimateTokens(messages), nil
}

func (p *LocalProvider) Capabilities(model string) ModelCapabilities {
	return DefaultCapabilities
}

// ListModels scans the models directory for .gguf files.
func (p *LocalProvider) ListModels(ctx context.Context) ([]types.ModelInfo, error) {
	var models []types.ModelInfo

	err := filepath.Walk(p.modelsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
			rel, err := filepath.Rel(p.modelsDir, path)
			if err != nil {
				rel = info.Name()
			}
			models = append(models, types.ModelInfo{
				ID:       rel,
				Object:   "model",
				OwnedBy:  "local",
				Provider: "local",
			})
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("local: walk models dir: %w", err)
	}

	return models, nil
}
