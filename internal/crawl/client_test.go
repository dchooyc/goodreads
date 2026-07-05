package crawl

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c := NewClient("test-agent", 0, 2*time.Second, 3)
	c.Jitter = 0
	c.Logf = t.Logf
	c.sleep = func(time.Duration) {} // never actually wait in tests
	return c
}

func TestFetchOK(t *testing.T) {
	var gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	body, status, err := testClient(t).Fetch(srv.URL)
	if err != nil || status != 200 || string(body) != "hello" {
		t.Fatalf("got body=%q status=%d err=%v", body, status, err)
	}
	if gotUA != "test-agent" {
		t.Errorf("User-Agent not sent, got %q", gotUA)
	}
	if gotAccept != "text/html,application/xhtml+xml" {
		t.Errorf("Accept header not sent, got %q", gotAccept)
	}
}

func TestFetch404NoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, status, err := testClient(t).Fetch(srv.URL)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if status != 404 {
		t.Fatalf("want status 404, got %d", status)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("404 must not be retried, got %d calls", calls)
	}
}

func TestFetch429RetryAfterThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := testClient(t)
	var slept []time.Duration
	c.sleep = func(d time.Duration) { slept = append(slept, d) }

	body, _, err := c.Fetch(srv.URL)
	if err != nil || string(body) != "ok" {
		t.Fatalf("want recovery after 429, got body=%q err=%v", body, err)
	}

	sawRetryAfter := false
	for _, d := range slept {
		if d == time.Second {
			sawRetryAfter = true
		}
	}
	if !sawRetryAfter {
		t.Errorf("Retry-After of 1s was not honoured, slept: %v", slept)
	}
}

func TestFetch500RetryThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	body, _, err := testClient(t).Fetch(srv.URL)
	if err != nil || string(body) != "ok" {
		t.Fatalf("want recovery after 500s, got body=%q err=%v", body, err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("want 3 calls, got %d", calls)
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	c := testClient(t)
	c.HTTPClient.Timeout = 50 * time.Millisecond
	c.MaxRetries = 1

	_, _, err := c.Fetch(srv.URL)
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
}

func TestFetchHardStopOnRepeatedBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := testClient(t)
	c.MaxRetries = 10
	c.ConsecutiveHardStop = 3

	_, status, err := c.Fetch(srv.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	if status != 403 {
		t.Fatalf("want status 403, got %d", status)
	}
}

func TestBlockedCountSpansRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := testClient(t)
	c.MaxRetries = 0
	c.ConsecutiveHardStop = 2

	if _, _, err := c.Fetch(srv.URL); errors.Is(err, ErrBlocked) {
		t.Fatal("first 429 should not hard-stop yet")
	}
	if _, _, err := c.Fetch(srv.URL); !errors.Is(err, ErrBlocked) {
		t.Fatalf("second consecutive 429 should hard-stop, got %v", err)
	}
}
