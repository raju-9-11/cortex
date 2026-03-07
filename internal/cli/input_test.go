package cli

import (
	"os"
	"strings"
	"testing"
)

func TestReadPrompt_FlagTakesPriority(t *testing.T) {
	// Even when stdin is a pipe with data, the flag should win.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("pipe content\n")
	w.Close()

	got, err := ReadPrompt(r, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestReadPrompt_StdinPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("hello world\n")
	w.Close()

	got, err := ReadPrompt(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestReadPrompt_EmptyPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	_, err = ReadPrompt(r, "")
	if err != ErrNoPrompt {
		t.Errorf("got error %v, want %v", err, ErrNoPrompt)
	}
}

func TestReadPrompt_WhitespaceOnly(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("  \n\t  ")
	w.Close()

	_, err = ReadPrompt(r, "")
	if err != ErrNoPrompt {
		t.Errorf("got error %v, want %v", err, ErrNoPrompt)
	}
}

func TestReadPrompt_NoFlagNoInput(t *testing.T) {
	// Use the read end of a pipe that simulates a "terminal-like" scenario.
	// Since os.Pipe() is NOT a terminal, we need a different approach.
	// We'll use /dev/tty if available; otherwise skip.
	f, err := os.Open("/dev/tty")
	if err != nil {
		t.Skip("cannot open /dev/tty: " + err.Error())
	}
	defer f.Close()

	_, err = ReadPrompt(f, "")
	if err != ErrNoPrompt {
		t.Errorf("got error %v, want %v", err, ErrNoPrompt)
	}
}

func TestReadPrompt_TrimWhitespace(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("  hello\n  ")
	w.Close()

	got, err := ReadPrompt(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestReadPrompt_LargeInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write more than 100KB.
	data := strings.Repeat("a", maxPromptSize+1)
	go func() {
		w.WriteString(data)
		w.Close()
	}()

	_, err = ReadPrompt(r, "")
	if err != ErrPromptTooLarge {
		t.Errorf("got error %v, want %v", err, ErrPromptTooLarge)
	}
}

func TestReadPrompt_ExactLimit(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write exactly 100KB.
	data := strings.Repeat("b", maxPromptSize)
	go func() {
		w.WriteString(data)
		w.Close()
	}()

	got, err := ReadPrompt(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != maxPromptSize {
		t.Errorf("got length %d, want %d", len(got), maxPromptSize)
	}
}

func TestIsTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if IsTerminal(r) {
		t.Error("expected pipe read end to not be a terminal")
	}
	if IsTerminal(w) {
		t.Error("expected pipe write end to not be a terminal")
	}
}
