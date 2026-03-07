package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	PrintUsage(&buf, "1.0.0")
	output := buf.String()

	commands := []string{"chat", "run", "sessions", "models", "version", "help"}
	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("usage output missing command %q", cmd)
		}
	}
}

func TestPrintUsage_ContainsEnvVars(t *testing.T) {
	var buf bytes.Buffer
	PrintUsage(&buf, "1.0.0")
	output := buf.String()

	envVars := []string{
		"FORGE_PROVIDER",
		"FORGE_MODEL",
		"FORGE_ADDR",
		"FORGE_API_KEY",
		"FORGE_DB_PATH",
		"FORGE_CONFIG",
		"OLLAMA_URL",
	}
	for _, env := range envVars {
		if !strings.Contains(output, env) {
			t.Errorf("usage output missing environment variable %q", env)
		}
	}
}

func TestPrintUsage_ContainsVersion(t *testing.T) {
	var buf bytes.Buffer
	PrintUsage(&buf, "42.99.1")
	output := buf.String()

	if !strings.Contains(output, "42.99.1") {
		t.Errorf("usage output does not contain version string; got:\n%s", output)
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	PrintVersion(&buf, "1.2.3")
	got := buf.String()
	want := "forge 1.2.3\n"
	if got != want {
		t.Errorf("PrintVersion output = %q, want %q", got, want)
	}
}

func TestPrintUnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	PrintUnknownCommand(&buf, "foobar")
	output := buf.String()

	if !strings.Contains(output, `"foobar"`) {
		t.Errorf("unknown command output should contain the command name; got:\n%s", output)
	}
	if !strings.Contains(output, "Error:") {
		t.Errorf("unknown command output should start with Error:; got:\n%s", output)
	}
	if !strings.Contains(output, "forge help") {
		t.Errorf("unknown command output should suggest 'forge help'; got:\n%s", output)
	}
}
