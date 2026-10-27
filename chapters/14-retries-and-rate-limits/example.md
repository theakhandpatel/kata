# 14 Example

*The finished implementation, taken apart. Open this after `kata submit 14` passes.*

The files it describes are `reference/ch14/agent/retry.go` and the send path in
`reference/ch14/agent/provider_openai.go`.

---

## Deciding what to retry

```go
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var he *HTTPError
	if !errors.As(err, &he) {
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
```

The `errors.As` test comes first because the answer depends on what kind of error it is.
Not an `HTTPError` means the request never reached a decision: a reset, a refused
connection, a timeout. Those are worth another attempt, unless the caller cancelled.

The `501` case is explicit for a reason. It is a 5xx, so the range check would catch it,
but it means "this server will never implement that method". Retrying it burns attempts
on something that cannot change.

## The wait, bounded

```go
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
```

Doubling from the base, capped. The `delay <= 0` guard catches the overflow that a
large attempt count produces: `time.Duration` is an integer, and multiplying it past
its limit wraps to something negative.

## The loop

```go
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
```

Four exit points, and each matters:

- **success**, so a healthy request is one attempt.
- **not retryable**, so a 400 costs one request rather than three.
- **out of attempts**, returning the last error rather than a summary sentence.
- **cancelled**, which returns `ctx.Err()` so `errors.Is(err, context.Canceled)` keeps
  working upstream.

`if attempts <= 0 { attempts = 1 }` is the guard against a policy nobody set. A zero
value that retries forever turns a busy server into a hang.

## Where it goes in the provider

```go
var resp *http.Response
err = withRetry(ctx, p.retryPolicy(), func() error {
	var doErr error
	resp, doErr = client.Do(httpReq)
	if doErr != nil {
		return doErr
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	return &HTTPError{
		Status:     resp.StatusCode,
		Body:       string(snippet),
		RetryAfter: parseRetryAfter(resp.Header),
	}
})
```

The retry wraps the whole exchange, so the stream parser above it never learns a 429
happened. That is the property the first two cases check.

Two details inside the closure. The response body is closed before returning the error,
because an unread body holds a connection open, and three retries would leak three. And
the status check lives inside the closure, so the retry loop can see the status without
the caller having to parse it.

## Reading Retry-After

```go
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
```

The header is allowed to be an HTTP date instead of a number. Only the number is
handled, because a date requires a trusted clock and a clock skew makes the wait
unpredictable. A date parses as `0`, which falls back to the computed backoff.

## The fixes that matter

**1. Giving up on a 429** (`retries-a-429`)

```diff
- return Message{}, Usage{}, &HTTPError{Status: 429, ...}
+ err = withRetry(ctx, policy, func() error { ... })
```

**2. Retrying everything** (`does-not-retry-a-400`)

```diff
- if err != nil { retry() }
+ if !Retryable(err) { return err }
```

**3. Ignoring the header** (`honours-retry-after`)

```diff
- wait := policy.backoff(attempt)
+ wait := policy.backoff(attempt)
+ if asked, ok := RetryAfter(err); ok { wait = asked }
```

**4. An uninterruptible wait** (`does-not-retry-a-cancellation`)

```diff
- time.Sleep(wait)
+ select {
+ case <-ctx.Done(): return ctx.Err()
+ case <-time.After(wait):
+ }
```

## What to compare against your own

- [ ] 429 and 5xx retry, 4xx does not, 501 does not
- [ ] a cancellation is never retried
- [ ] the delay doubles and is capped
- [ ] `Retry-After` overrides the computed delay
- [ ] the body is closed before the next attempt
- [ ] the attempt count is bounded, and a zero policy means one attempt
- [ ] the parser never sees a retried status

Everything outside that list is style.
