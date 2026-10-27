# 14 Retries and rate limits

**Part VI · Operate** · Difficulty: medium · 45 to 60 minutes · 7 graded cases

## What you are building

A program that survives a busy server.

```sh
$ ./agent -p "say something"
rate limited, waiting 1s
The quick brown fox jumps over the lazy dog.
```

The server answered `429 Too Many Requests` the first time and worked the second. The
person asking the question never sees that.

You edit `agent/provider_openai.go` and `agent/cmd/agent/main.go`. You also create one new file, `agent/retry.go`, holding the policy, the retry loop and the `Retry-After` parse.
## Two kinds of failure

Not every error deserves another attempt.

| Status | Meaning | Try again? |
|---|---|---|
| 429 | busy | yes, when the server says |
| 500, 502, 503, 504 | the server broke | yes |
| 501 | the server will never support this | no |
| 400, 401, 403, 404, 422 | the request is wrong | no |
| connection reset, timeout | no decision was reached | yes |
| the caller cancelled | you were told to stop | no |

A 400 sent twice is the same wrong request, paid for twice. A 429 sent once is a
failure the person has to handle by hand.

## Backoff

Each wait doubles, up to a ceiling:

```
  attempt 1 fails   wait 0.5s
  attempt 2 fails   wait 1s
  attempt 3 fails   wait 2s         stop here if MaxAttempts is 3
```

The ceiling matters. Without it, a long retry sequence ends up waiting an hour between
attempts.

No jitter here. With a handful of clients the herd that jitter protects against does
not appear, and a predictable delay is far easier to test.

## Retry-After

When a server knows how long to wait, it says so:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
```

That number is in seconds and overrides your own delay. Retrying sooner earns another
429, and in a shared service it is rude as well as useless.

## Cancellation wins

The wait must be interruptible:

```go
select {
case <-ctx.Done():
	return ctx.Err()
case <-time.After(wait):
}
```

Without that, Ctrl-C during a backoff leaves the program trying for another minute.

## What you need to build

```go
type RetryPolicy struct {
    MaxAttempts int           // counts the first try
    BaseDelay   time.Duration
    MaxDelay    time.Duration
}

func Retryable(err error) bool
func RetryAfter(err error) (time.Duration, bool)
```

`HTTPError` gains a field:

```go
type HTTPError struct {
    Status     int
    Body       string
    RetryAfter time.Duration
}
```

Two flags:

```sh
agent -p "..." -retries 3 -retry-delay 500ms
```

## How you are checked

The verifier builds your program, scripts a server that fails a chosen number of times,
runs the binary, and counts the requests.

- `retries-a-429`: one 429 then a reply. Two requests, exit 0, the second answer on
  stdout.
- `retries-a-500`: one 503 then a reply. Two requests, exit 0.
- `does-not-retry-a-400`: a single 400, one request, exit 1, the status named on stderr.
- `honours-retry-after`: a 429 carrying `Retry-After: 1` with `-retry-delay 10ms` still
  waits about a second.
- `stops-after-the-limit`: with `-retries 3` and a server that always fails, three
  requests and exit 1.
- `no-retry-on-success`: a plain reply costs one request.
- `does-not-retry-a-cancellation`: with `-timeout 100ms` and a slow server, the run ends
  in under 1.5 seconds.

## Notes

Look at the status before reading the body, and close it before retrying. A response
body left unread leaks a connection.

`client.Do` reuses the request when the body is a `*bytes.Reader`, so the same request
can be sent again. A body that has already been read cannot.

Treat a cancelled context as final, in both the wait and the retryable check.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either it retries something it should not, gives up on something it should retry, or it
waits the wrong amount of time.
</details>

<details><summary>Second hint: what you will need</summary>

`errors.As` for `*HTTPError`, `math.Pow` or a doubling loop for the delay, `strconv.Atoi`
for the header, and `select` on `ctx.Done()`.
</details>

<details><summary>Third hint: the finished code</summary>

`Retryable` returns false for nil and for a cancellation, true for 429 and for any 5xx
except 501, and false for every other status. `withRetry` loops over attempts, returns
on success or on a non-retryable error, and otherwise waits, preferring `RetryAfter`
over the computed backoff, and selecting on the context during the wait. In `Chat`, wrap
`client.Do` in `withRetry` and return the error from the status check so the loop can
inspect it.

`kata hint 14 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 14` reports 7/7, and a server that fails once still produces an answer.

[example.md](example.md) has the finished code, after you pass.

---

Next: **15 Bounding a tool result**.
