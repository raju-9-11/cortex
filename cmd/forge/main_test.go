package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"help"}, os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("expected stdout to contain 'Usage:', got %q", stdout.String())
	}
}

func TestRun_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("expected stdout to contain version %q, got %q", version, stdout.String())
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"foobar"}, os.Stdin, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("expected stderr to contain 'unknown command', got %q", stderr.String())
	}
}

func TestRun_HelpFlags(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{flag}, os.Stdin, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("expected exit code 0 for %q, got %d", flag, code)
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Errorf("expected stdout to contain 'Usage:' for %q, got %q", flag, stdout.String())
			}
		})
	}
}

func TestRun_VersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("expected stdout to contain version %q, got %q", version, stdout.String())
	}
}

func TestRun_NoArgs(t *testing.T) {
	// Verify run() is callable with no args.
	// We can't actually start the server in a unit test (needs network/config),
	// so we just verify the function signature and dispatch structure exist.
	t.Skip("skipping: no-args starts the HTTP server which requires full app initialization")
}
