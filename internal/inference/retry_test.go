package inference

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// isRetryableStatus
// ---------------------------------------------------------------------------

func TestIsRetryableStatus(t *testing.T) {
	t.Parallel()

	retryable := []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, code := range retryable {
		if !isRetryableStatus(code) {
			t.Errorf("expected %d to be retryable", code)
		}
	}

	nonRetryable := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
	}
	for _, code := range nonRetryable {
		if isRetryableStatus(code) {
			t.Errorf("expected %d to NOT be retryable", code)
		}
	}
}

// ---------------------------------------------------------------------------
// backoffDelay
// ---------------------------------------------------------------------------

func TestBackoffDelay(t *testing.T) {
	t.Parallel()

	cfg := RetryConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  1 * time.Second,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},   // 100ms * 2^0 = 100ms
		{1, 200 * time.Millisecond},   // 100ms * 2^1 = 200ms
		{2, 400 * time.Millisecond},   // 100ms * 2^2 = 400ms
		{3, 800 * time.Millisecond},   // 100ms * 2^3 = 800ms
		{4, 1 * time.Second},          // 100ms * 2^4 = 1600ms → capped at 1s
		{10, 1 * time.Second},         // way past cap
	}

	for _, tt := range tests {
		got := backoffDelay(tt.attempt, cfg)
		if got != tt.expected {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// sleep respects context cancellation
// ---------------------------------------------------------------------------

func TestSleep_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	start := time.Now()
	sleep(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("sleep should have returned immediately on cancelled ctx, took %v", elapsed)
	}
}

func TestSleep_FullDuration(t *testing.T) {
	t.Parallel()

	start := time.Now()
	sleep(context.Background(), 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Errorf("sleep should have waited ~50ms, took only %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// retryDo — success on first attempt
// ---------------------------------------------------------------------------

func TestRetryDo_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	resp, err := retryDo(context.Background(), srv.Client(), req, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&calls))
	}
}

// ---------------------------------------------------------------------------
// retryDo — retries on 503 then succeeds
// ---------------------------------------------------------------------------

func TestRetryDo_RetryThenSuccess(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	body := bytes.NewReader([]byte("request-body"))
	req, _ := http.NewRequestWithContext(context.Background(), "POST", srv.URL, body)
	resp, err := retryDo(context.Background(), srv.Client(), req, RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&calls))
	}
}

// ---------------------------------------------------------------------------
// retryDo — all attempts exhausted
// ---------------------------------------------------------------------------

func TestRetryDo_AllAttemptsExhausted(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "error")
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	_, err := retryDo(context.Background(), srv.Client(), req, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error when all attempts exhausted")
	}
	if !strings.Contains(err.Error(), "all 3 attempts exhausted") {
		t.Errorf("unexpected error message: %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&calls))
	}
}

// ---------------------------------------------------------------------------
// retryDo — non-retryable status returned immediately
// ---------------------------------------------------------------------------

func TestRetryDo_NonRetryableStatusReturned(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	resp, err := retryDo(context.Background(), srv.Client(), req, RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Non-retryable status: returned immediately, no retries.
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "bad request" {
		t.Errorf("unexpected body: %q", string(body))
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call (no retries), got %d", atomic.LoadInt32(&calls))
	}
}

// ---------------------------------------------------------------------------
// retryDo — context cancelled aborts retries
// ---------------------------------------------------------------------------

func TestRetryDo_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, err := retryDo(ctx, srv.Client(), req, RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Second,
		MaxDelay:    10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(err.Error(), "retry aborted") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// retryDo — MaxAttempts <= 0 uses default
// ---------------------------------------------------------------------------

func TestRetryDo_ZeroMaxAttemptsUsesDefault(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	resp, err := retryDo(context.Background(), srv.Client(), req, RetryConfig{
		MaxAttempts: 0, // should default to DefaultRetryConfig.MaxAttempts
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&calls))
	}
}

// ---------------------------------------------------------------------------
// DefaultRetryConfig sanity
// ---------------------------------------------------------------------------

func TestDefaultRetryConfig(t *testing.T) {
	t.Parallel()

	if DefaultRetryConfig.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts=3, got %d", DefaultRetryConfig.MaxAttempts)
	}
	if DefaultRetryConfig.BaseDelay != 500*time.Millisecond {
		t.Errorf("expected BaseDelay=500ms, got %v", DefaultRetryConfig.BaseDelay)
	}
	if DefaultRetryConfig.MaxDelay != 10*time.Second {
		t.Errorf("expected MaxDelay=10s, got %v", DefaultRetryConfig.MaxDelay)
	}
}
