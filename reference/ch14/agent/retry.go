package agent

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy decides how many times to try again and how long to wait between
// attempts.
//
// The zero value is unusable on purpose. A policy that retries forever when nobody set
// one turns a busy server into a program that hangs.
type RetryPolicy struct {
	// MaxAttempts counts the first try. 3 means one try and two retries.
	MaxAttempts int
	// BaseDelay is the first wait. Each later wait doubles it.
	BaseDelay time.Duration
	// MaxDelay caps the doubling.
	MaxDelay time.Duration
}

// DefaultRetryPolicy is what the program uses unless told otherwise.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts: 3,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    8 * time.Second,
}

// Retryable reports whether an error is worth trying again.
//
// Two cases qualify. A transport failure, where the request never reached a decision.
// And the status codes servers use for "come back later": 429 and the 5xx family,
// except 501, which means the server will never support this.
//
// Everything else is a decision. A 400 means the request is wrong, and sending it again
// sends the same wrong request.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		// Not an HTTP error: a connection problem, a reset, a timeout.
		return !errors.Is(err, context.Canceled)
	}
	switch {
	case he.Status == http.StatusTooManyRequests:
		return true
	case he.Status == http.StatusNotImplemented:
		return false
	case he.Status >= 500:
		return true
	default:
		return false
	}
}

// RetryAfter returns the wait the server asked for, if it asked.
func RetryAfter(err error) (time.Duration, bool) {
	var he *HTTPError
	if !errors.As(err, &he) {
		return 0, false
	}
	if he.RetryAfter <= 0 {
		return 0, false
	}
	return he.RetryAfter, true
}

// backoff returns the wait before attempt n, counting from 1.
//
// The doubling is bounded by MaxDelay, so a long retry sequence does not end up waiting
// an hour. There is no jitter, and the reason is written down rather than skipped: with
// a handful of clients, the thundering herd that jitter protects against does not
// appear, and a predictable delay is far easier to test.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	base := p.BaseDelay
	if base <= 0 {
		base = DefaultRetryPolicy.BaseDelay
	}
	max := p.MaxDelay
	if max <= 0 {
		max = DefaultRetryPolicy.MaxDelay
	}
	delay := time.Duration(float64(base) * math.Pow(2, float64(attempt-1)))
	if delay > max || delay <= 0 {
		return max
	}
	return delay
}

// withRetry runs fn until it succeeds, fails permanently, or runs out of attempts.
//
// It returns the last error, so the caller sees the reason the final attempt failed
// rather than a summary sentence.
func withRetry(ctx context.Context, policy RetryPolicy, fn func() error) error {
	attempts := policy.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !Retryable(err) || attempt == attempts {
			return err
		}

		wait := policy.backoff(attempt)
		if asked, ok := RetryAfter(err); ok {
			wait = asked
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}

// parseRetryAfter reads the Retry-After header, which is either a number of seconds or
// an HTTP date. Only the number is handled; a date requires a trusted clock.
func parseRetryAfter(h http.Header) time.Duration {
	raw := h.Get("Retry-After")
	if raw == "" {
		return 0
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
