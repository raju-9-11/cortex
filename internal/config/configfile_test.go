package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfigFile_EnvPath(t *testing.T) {
	// env path set + exists → returns env path
	dir := t.TempDir()
	envPath := filepath.Join(dir, "custom-config.json")
	if err := os.WriteFile(envPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	got := DiscoverConfigFile(envPath)
	if got != envPath {
		t.Errorf("DiscoverConfigFile(%q) = %q, want %q", envPath, got, envPath)
	}
}

func TestDiscoverConfigFile_EnvPathNotExists(t *testing.T) {
	// env path set but doesn't exist → falls through
	got := DiscoverConfigFile("/nonexistent/path/config.json")
	// Should not return the non-existent path; should return "" or another found file
	if got == "/nonexistent/path/config.json" {
		t.Errorf("DiscoverConfigFile returned non-existent env path")
	}
}

func TestDiscoverConfigFile_HomeDir(t *testing.T) {
	// Create a temp dir to simulate ~/.forge/config.json
	// We can't easily mock os.UserHomeDir, so instead we test by placing
	// a file at the actual home dir path temporarily.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	forgeDir := filepath.Join(home, ".forge")
	configPath := filepath.Join(forgeDir, "config.json")

	// Check if directory/file already exist to avoid clobbering
	dirExisted := true
	if _, err := os.Stat(forgeDir); os.IsNotExist(err) {
		dirExisted = false
		if err := os.MkdirAll(forgeDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	fileExisted := true
	var origContent []byte
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fileExisted = false
	} else {
		origContent, _ = os.ReadFile(configPath)
	}

	// Write test file
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Cleanup
	defer func() {
		if fileExisted {
			os.WriteFile(configPath, origContent, 0644)
		} else {
			os.Remove(configPath)
		}
		if !dirExisted {
			os.Remove(forgeDir)
		}
	}()

	got := DiscoverConfigFile("")
	if got != configPath {
		t.Errorf("DiscoverConfigFile(\"\") = %q, want %q", got, configPath)
	}
}

func TestDiscoverConfigFile_LocalDir(t *testing.T) {
	// ./forge.config.json exists → returns it
	dir := t.TempDir()
	localConfig := filepath.Join(dir, "forge.config.json")
	if err := os.WriteFile(localConfig, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp dir so that "forge.config.json" resolves
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Make sure ~/.forge/config.json doesn't interfere
	home, _ := os.UserHomeDir()
	homeConfig := filepath.Join(home, ".forge", "config.json")
	if _, err := os.Stat(homeConfig); err == nil {
		// Home config exists; use envPath="" and accept that home config wins
		// In this case, we just verify some config is found
		got := DiscoverConfigFile("")
		if got == "" {
			t.Error("DiscoverConfigFile(\"\") = \"\", want a config path")
		}
		return
	}

	got := DiscoverConfigFile("")
	if got != "forge.config.json" {
		t.Errorf("DiscoverConfigFile(\"\") = %q, want %q", got, "forge.config.json")
	}
}

func TestDiscoverConfigFile_None(t *testing.T) {
	// No files exist → returns ""
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Make sure ~/.forge/config.json doesn't exist
	home, _ := os.UserHomeDir()
	homeConfig := filepath.Join(home, ".forge", "config.json")
	if _, err := os.Stat(homeConfig); err == nil {
		t.Skip("~/.forge/config.json exists, cannot test 'none found' case")
	}

	got := DiscoverConfigFile("")
	if got != "" {
		t.Errorf("DiscoverConfigFile(\"\") = %q, want \"\"", got)
	}
}

func TestDiscoverConfigFile_Priority(t *testing.T) {
	// env path > ~/.forge/config.json > ./forge.config.json
	dir := t.TempDir()

	envPath := filepath.Join(dir, "env-config.json")
	localConfig := filepath.Join(dir, "forge.config.json")

	if err := os.WriteFile(envPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localConfig, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// env path should win
	got := DiscoverConfigFile(envPath)
	if got != envPath {
		t.Errorf("DiscoverConfigFile(%q) = %q, want %q (env path should win)", envPath, got, envPath)
	}
}

func TestLoadConfigFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"default_provider": "ollama",
		"default_model": "llama3.2:latest",
		"providers": [
			{
				"name": "ollama",
				"base_url": "http://localhost:11434"
			},
			{
				"name": "openai",
				"base_url": "https://api.openai.com/v1",
				"api_key": "sk-test"
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile(%q) error = %v", path, err)
	}
	if fc == nil {
		t.Fatal("LoadConfigFile returned nil")
	}
	if fc.DefaultProvider != "ollama" {
		t.Errorf("DefaultProvider = %q, want %q", fc.DefaultProvider, "ollama")
	}
	if fc.DefaultModel != "llama3.2:latest" {
		t.Errorf("DefaultModel = %q, want %q", fc.DefaultModel, "llama3.2:latest")
	}
	if len(fc.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(fc.Providers))
	}

	// Check first provider
	if fc.Providers[0].Name != "ollama" {
		t.Errorf("Providers[0].Name = %q, want %q", fc.Providers[0].Name, "ollama")
	}
	if fc.Providers[0].BaseURL != "http://localhost:11434" {
		t.Errorf("Providers[0].BaseURL = %q, want %q", fc.Providers[0].BaseURL, "http://localhost:11434")
	}
	if fc.Providers[0].APIKey != "" {
		t.Errorf("Providers[0].APIKey = %q, want empty", fc.Providers[0].APIKey)
	}

	// Check second provider
	if fc.Providers[1].Name != "openai" {
		t.Errorf("Providers[1].Name = %q, want %q", fc.Providers[1].Name, "openai")
	}
	if fc.Providers[1].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("Providers[1].BaseURL = %q, want %q", fc.Providers[1].BaseURL, "https://api.openai.com/v1")
	}
	if fc.Providers[1].APIKey != "sk-test" {
		t.Errorf("Providers[1].APIKey = %q, want %q", fc.Providers[1].APIKey, "sk-test")
	}
}

func TestLoadConfigFile_Empty(t *testing.T) {
	// empty path → nil, nil
	fc, err := LoadConfigFile("")
	if err != nil {
		t.Errorf("LoadConfigFile(\"\") error = %v, want nil", err)
	}
	if fc != nil {
		t.Errorf("LoadConfigFile(\"\") = %+v, want nil", fc)
	}
}

func TestLoadConfigFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := LoadConfigFile(path)
	if err == nil {
		t.Error("LoadConfigFile with invalid JSON: expected error, got nil")
	}
	if fc != nil {
		t.Errorf("LoadConfigFile with invalid JSON: expected nil FileConfig, got %+v", fc)
	}
}

func TestLoadConfigFile_MissingFile(t *testing.T) {
	fc, err := LoadConfigFile("/nonexistent/path/config.json")
	if err == nil {
		t.Error("LoadConfigFile with missing file: expected error, got nil")
	}
	if fc != nil {
		t.Errorf("LoadConfigFile with missing file: expected nil FileConfig, got %+v", fc)
	}
}

func TestLoadConfigFile_MinimalConfig(t *testing.T) {
	// just providers, no defaults → parsed correctly
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.json")
	content := `{
		"providers": [
			{
				"name": "my-custom",
				"base_url": "https://custom.example.com/v1",
				"api_key": "key-123"
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile error = %v", err)
	}
	if fc.DefaultProvider != "" {
		t.Errorf("DefaultProvider = %q, want empty", fc.DefaultProvider)
	}
	if fc.DefaultModel != "" {
		t.Errorf("DefaultModel = %q, want empty", fc.DefaultModel)
	}
	if len(fc.Providers) != 1 {
		t.Fatalf("len(Providers) = %d, want 1", len(fc.Providers))
	}
	if fc.Providers[0].Name != "my-custom" {
		t.Errorf("Providers[0].Name = %q, want %q", fc.Providers[0].Name, "my-custom")
	}
	if fc.Providers[0].APIKey != "key-123" {
		t.Errorf("Providers[0].APIKey = %q, want %q", fc.Providers[0].APIKey, "key-123")
	}
}

func TestLoadConfigFile_EmptyProviders(t *testing.T) {
	// no providers array → empty slice (nil in Go, but no error)
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	content := `{
		"default_provider": "ollama",
		"default_model": "test-model"
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile error = %v", err)
	}
	if fc.DefaultProvider != "ollama" {
		t.Errorf("DefaultProvider = %q, want %q", fc.DefaultProvider, "ollama")
	}
	if len(fc.Providers) != 0 {
		t.Errorf("len(Providers) = %d, want 0", len(fc.Providers))
	}
}
