package cli

import (
	"errors"
	"io"
	"os"
	"strings"
)

var (
	ErrNoPrompt      = errors.New("no prompt provided: use --prompt flag or pipe input via stdin")
	ErrPromptTooLarge = errors.New("prompt too large: maximum 100KB via stdin pipe")
)

const maxPromptSize = 100 * 1024 // 100KB

// IsTerminal reports whether f is connected to a terminal (not a pipe/redirect).
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// ReadPrompt determines the prompt text from available sources.
// Priority: flagPrompt > stdin pipe content > ErrNoPrompt.
// If stdin is a pipe (not a terminal), reads all content up to maxPromptSize.
// Whitespace is trimmed from the result.
func ReadPrompt(stdin *os.File, flagPrompt string) (string, error) {
	// 1. If flagPrompt is non-empty after trimming, use it.
	if trimmed := strings.TrimSpace(flagPrompt); trimmed != "" {
		return trimmed, nil
	}

	// 2. If stdin is a pipe (not a terminal), read from it.
	if !IsTerminal(stdin) {
		limited := &io.LimitedReader{R: stdin, N: maxPromptSize + 1}
		data, err := io.ReadAll(limited)
		if err != nil {
			return "", err
		}

		// If we read more than maxPromptSize, the input is too large.
		if len(data) > maxPromptSize {
			return "", ErrPromptTooLarge
		}

		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			return "", ErrNoPrompt
		}
		return trimmed, nil
	}

	// 3. No flag and stdin is a terminal — no prompt available.
	return "", ErrNoPrompt
}
