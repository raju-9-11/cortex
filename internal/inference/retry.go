package inference

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"
)

// RetryConfig controls the retry behaviour for HTTP calls.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts including the initial one.
	MaxAttempts int
	// BaseDelay is the initial backoff delay before the first retry.
	BaseDelay time.Duration
	// MaxDelay is the upper bound on the exponential backoff delay.
	MaxDelay time.Duration
}

// DefaultRetryConfig provides sensible defaults for production use.
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    10 * time.Second,
}

// isRetryableStatus reports whether the HTTP status code is worth retrying.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// retryDo executes an HTTP request with exponential-backoff retries.
//
// Callers must construct the request with a body created via bytes.NewReader
// (or another mechanism that populates req.GetBody) so that the body can be
// re-read on subsequent attempts.
func retryDo(ctx context.Context, client *http.Client, req *http.Request, cfg RetryConfig) (*http.Response, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultRetryConfig.MaxAttempts
	}

	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Bail out early if the context is already cancelled.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("retry aborted before attempt %d: %w", attempt+1, err)
		}

		// On retries we need to reset the request body.
		if attempt > 0 {
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("failed to reset request body on attempt %d: %w", attempt+1, err)
				}
				req.Body = body
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			// Network-level error — eligible for retry.
			lastErr = fmt.Errorf("attempt %d: %w", attempt+1, err)
			sleep(ctx, backoffDelay(attempt, cfg))
			continue
		}

		// Non-retryable status or success — return immediately.
		if !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Retryable HTTP status — drain and close the body before retrying.
		resp.Body.Close()
		lastErr = fmt.Errorf("attempt %d: server returned retryable status %d", attempt+1, resp.StatusCode)
		sleep(ctx, backoffDelay(attempt, cfg))
	}

	return nil, fmt.Errorf("all %d attempts exhausted: %w", cfg.MaxAttempts, lastErr)
}

// backoffDelay computes the exponential backoff delay for the given attempt,
// capped at cfg.MaxDelay.  Formula: baseDelay * 2^attempt.
func backoffDelay(attempt int, cfg RetryConfig) time.Duration {
	delay := time.Duration(float64(cfg.BaseDelay) * math.Pow(2, float64(attempt)))
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	return delay
}

// sleep pauses for the given duration but returns early if ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
