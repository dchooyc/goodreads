package crawl

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ErrBlocked is returned when the server has answered 403/429 enough times in
// a row that continuing would be impolite. Callers must stop the run.
var ErrBlocked = errors.New("blocked: too many consecutive 403/429 responses")

// ErrNotFound is returned for 404 responses. It is never retried.
var ErrNotFound = errors.New("not found (404)")

// HTTPStatusError reports a non-200 final status.
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status %d for %s", e.StatusCode, e.URL)
}

// Fetcher is the minimal interface recovery code depends on, so tests can
// substitute fakes.
type Fetcher interface {
	Fetch(url string) (body []byte, statusCode int, err error)
}

// Client is a polite HTTP client: single shared instance, per-request
// timeout, identifying User-Agent, minimum delay with jitter between
// requests, bounded retries with exponential backoff, Retry-After support,
// and a hard stop after repeated 403/429.
type Client struct {
	HTTPClient          *http.Client
	UserAgent           string
	MinDelay            time.Duration
	Jitter              time.Duration
	MaxRetries          int
	BackoffBase         time.Duration
	BackoffMax          time.Duration
	ConsecutiveHardStop int
	Logf                func(format string, args ...interface{})

	// sleep is swappable in tests.
	sleep func(time.Duration)

	mu                 sync.Mutex
	lastRequestAt      time.Time
	consecutiveBlocked int
}

// DefaultUserAgent identifies the crawler and gives a contact address.
const DefaultUserAgent = "topreads-recovery-bot/1.0 (contact: dchooyc@gmail.com)"

// NewClient returns a Client with the politeness defaults recommended for
// targeted recovery.
func NewClient(userAgent string, minDelay, timeout time.Duration, maxRetries int) *Client {
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	return &Client{
		HTTPClient:          &http.Client{Timeout: timeout},
		UserAgent:           userAgent,
		MinDelay:            minDelay,
		Jitter:              minDelay / 4,
		MaxRetries:          maxRetries,
		BackoffBase:         10 * time.Second,
		BackoffMax:          5 * time.Minute,
		ConsecutiveHardStop: 3,
		Logf:                func(format string, args ...interface{}) { fmt.Printf(format+"\n", args...) },
		sleep:               time.Sleep,
	}
}

func (c *Client) logf(format string, args ...interface{}) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

func (c *Client) doSleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if c.sleep != nil {
		c.sleep(d)
	} else {
		time.Sleep(d)
	}
}

// waitPoliteDelay enforces MinDelay (+ random jitter) between request starts.
func (c *Client) waitPoliteDelay() {
	c.mu.Lock()
	wait := time.Duration(0)
	if !c.lastRequestAt.IsZero() {
		elapsed := time.Since(c.lastRequestAt)
		if elapsed < c.MinDelay {
			wait = c.MinDelay - elapsed
		}
	}
	if c.Jitter > 0 {
		wait += time.Duration(rand.Int63n(int64(c.Jitter) + 1))
	}
	c.lastRequestAt = time.Now().Add(wait)
	c.mu.Unlock()

	c.doSleep(wait)
}

func (c *Client) recordBlocked() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutiveBlocked++
	if c.ConsecutiveHardStop > 0 && c.consecutiveBlocked >= c.ConsecutiveHardStop {
		return ErrBlocked
	}
	return nil
}

func (c *Client) recordSuccess() {
	c.mu.Lock()
	c.consecutiveBlocked = 0
	c.mu.Unlock()
}

func retryAfterDelay(resp *http.Response, fallback time.Duration) time.Duration {
	if resp == nil {
		return fallback
	}
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return fallback
	}
	if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return fallback
}

// backoff grows linearly: base, 2*base, 3*base (10s, 20s, 30s by default).
func (c *Client) backoff(attempt int) time.Duration {
	d := c.BackoffBase * time.Duration(attempt+1)
	if c.BackoffMax > 0 && d > c.BackoffMax {
		d = c.BackoffMax
	}
	return d
}

// Fetch GETs a URL politely and returns the body and final status code.
// Retries transient failures (429, 5xx, network errors) with backoff.
// Does not retry 404 or other 4xx. Returns ErrBlocked once repeated 403/429
// responses hit the hard-stop threshold.
func (c *Client) Fetch(url string) ([]byte, int, error) {
	var lastStatus int
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt - 1)
			c.logf("retry %d/%d for %s in %s", attempt, c.MaxRetries, url, delay)
			c.doSleep(delay)
		}

		c.waitPoliteDelay()

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("build request for %s: %w", url, err)
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http get %s: %w", url, err)
			c.logf("attempt %d: %v", attempt+1, lastErr)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode
		c.logf("attempt %d: GET %s -> %d", attempt+1, url, resp.StatusCode)

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				lastErr = fmt.Errorf("read body of %s: %w", url, readErr)
				continue
			}
			c.recordSuccess()
			return body, resp.StatusCode, nil

		case resp.StatusCode == http.StatusNotFound:
			c.recordSuccess()
			return nil, resp.StatusCode, fmt.Errorf("%s: %w", url, ErrNotFound)

		case resp.StatusCode == http.StatusForbidden:
			lastErr = &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
			if err := c.recordBlocked(); err != nil {
				return nil, resp.StatusCode, err
			}
			c.doSleep(c.backoff(attempt))

		case resp.StatusCode == http.StatusTooManyRequests:
			lastErr = &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
			if err := c.recordBlocked(); err != nil {
				return nil, resp.StatusCode, err
			}
			c.doSleep(retryAfterDelay(resp, c.backoff(attempt)))

		case resp.StatusCode == http.StatusServiceUnavailable:
			lastErr = &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
			c.doSleep(retryAfterDelay(resp, 0))

		case resp.StatusCode >= 500:
			lastErr = &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}

		default:
			// Other 4xx: not transient, do not retry.
			c.recordSuccess()
			return nil, resp.StatusCode, &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all attempts failed for %s", url)
	}
	return nil, lastStatus, fmt.Errorf("giving up after %d attempts: %w", c.MaxRetries+1, lastErr)
}
