package app

import (
	"testing"
)

func TestNewSucceeds(t *testing.T) {
	// Use a temp DB so we don't pollute the working directory.
	tmpFile := t.TempDir() + "/test.db"
	t.Setenv("FORGE_DB_PATH", tmpFile)

	application, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer application.Close()

	if application.Config == nil {
		t.Error("Config is nil")
	}
	if application.Store == nil {
		t.Error("Store is nil")
	}
	if application.Registry == nil {
		t.Error("Registry is nil")
	}
	if application.SessionMgr == nil {
		t.Error("SessionMgr is nil")
	}
	if application.Auth == nil {
		t.Error("Auth is nil")
	}
}

func TestNewWithVersion(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	t.Setenv("FORGE_DB_PATH", tmpFile)

	application, err := New(WithVersion("1.2.3"))
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer application.Close()

	if application.Config.Version != "1.2.3" {
		t.Errorf("expected version %q, got %q", "1.2.3", application.Config.Version)
	}
}

func TestNewDefaultVersionIsDev(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	t.Setenv("FORGE_DB_PATH", tmpFile)

	application, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer application.Close()

	if application.Config.Version != "dev" {
		t.Errorf("expected default version %q, got %q", "dev", application.Config.Version)
	}
}

func TestNewMockFallback(t *testing.T) {
	// Ensure no provider env vars are set so we hit mock fallback.
	tmpFile := t.TempDir() + "/test.db"
	t.Setenv("FORGE_DB_PATH", tmpFile)

	// Clear all provider keys so no real providers are registered.
	for _, key := range []string{
		"OPENAI_API_KEY", "QWEN_API_KEY", "LLAMA_API_KEY",
		"MINIMAX_API_KEY", "OSS_API_KEY",
	} {
		t.Setenv(key, "")
	}

	application, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer application.Close()

	providers := application.Registry.Providers()
	if len(providers) == 0 {
		t.Fatal("expected mock providers to be registered, got none")
	}

	// Verify the mock provider names are present.
	expectedProviders := []string{"qwen", "llama", "minimax", "oss"}
	for _, name := range expectedProviders {
		if _, ok := providers[name]; !ok {
			t.Errorf("expected mock provider %q to be registered", name)
		}
	}
}

func TestCloseIdempotent(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	t.Setenv("FORGE_DB_PATH", tmpFile)

	application, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// First close should succeed.
	if err := application.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}

	// Second close should not panic (SQLite tolerates double-close).
	// We don't check the error here since the behavior is implementation-dependent,
	// but it must not panic.
	application.Close()
}

func TestCloseNilStore(t *testing.T) {
	a := &App{}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() on nil Store returned error: %v", err)
	}
}
