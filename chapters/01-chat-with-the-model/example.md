# 01 Example

*The finished implementation, taken apart. Open this after a real attempt, or after
`kata submit 01` passes, and compare. Reading it first turns the exercise into
copy-typing.*

The file it describes is `reference/ch01/agent/provider_openai.go`. This page follows
the order you would think it through, not the order the lines appear.

---

## Split it into three jobs

One 90-line function will pass. You will regret it in the Tool calling chapter, when the same parser
also has to handle tool calls. Three small functions cost nothing now and save a rewrite:

```
Chat
 ├── buildRequest   our Request  →  *http.Request     make the request
 ├── do             send it, or fail                  send and check
 └── readSSE        the stream   →  Message, Usage    read the pieces
```

`Chat` builds, sends, reads. Each of the other two is short enough to hold in your head.

## Job 1: make the request

Your own `Message` type and the JSON message are similar, but not the same. Keep them
separate. Do not put `json:` tags on the types you use everywhere else, because in
the Tool calling chapter the JSON shape grows an object that your type does not have, and you would
have to change every file that touches it.

```go
type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}
```

Two small decisions that cost nothing now and a lot later:

**Use `*int` and `*float64` for the optional numbers.** With a plain `int`, you cannot
tell "the caller did not set this" apart from "the caller set it to zero". One day you
will send `max_tokens: 0`, which tells a real provider to produce nothing, and the
program will look broken for no visible reason. A pointer is nil when it was not set,
so `omitempty` drops it from the JSON.

**Let the model name fall back.** Use the one on the request if it is set, otherwise the
one on the client. The Sessions chapter reuses a provider across runs and does not always
pass a
model name.

## Job 2: send it

```go
url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
if err != nil {
	return Message{}, Usage{}, fmt.Errorf("build request: %w", err)
}
httpReq.Header.Set("Content-Type", "application/json")
httpReq.Header.Set("Accept", "text/event-stream")
if p.APIKey != "" {
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
}
```

**`TrimRight` on the base URL.** With it, `http://x/v1` and `http://x/v1/` both work.
Without it, one of them silently produces a URL with a double slash and a 404 that
takes an hour to find.

**`NewRequestWithContext`, not `NewRequest`.** This is the only place the caller's
context enters your code. It is what makes the timeout check pass, and what makes
Ctrl-C work in the Steer and abort chapter.

**`"stream": true`.** The checks require it, and the next job depends on it. Without
it the server sends one large reply and your line reader waits forever.

## Job 3: fail in a way the caller can use

```go
resp, err := client.Do(httpReq)
if err != nil {
	return Message{}, Usage{}, err
}
defer resp.Body.Close()

if resp.StatusCode < 200 || resp.StatusCode >= 300 {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return Message{}, Usage{}, &HTTPError{Status: resp.StatusCode, Body: string(snippet)}
}
```

Three things here, each of which is a check.

**Return the error from `client.Do` untouched.** It already wraps `context.Canceled`
when the caller gives up. If you wrap it in a string to add context, then
`errors.Is(err, context.Canceled)` stops working and the timeout check fails. Let it
through.

**Read at most 4 KiB of an error page.** Error responses are sometimes megabytes of
HTML. You want a line you can read, not a memory spike.

**Return your own `*HTTPError`, not a formatted string.** The agent loop needs to tell "the
server is busy, try again" apart from "the model gave a bad answer". A typed error makes
that a type assertion. A string makes it a regex.

## Job 4: read the pieces

This is the chapter. Everything above is plumbing.

```go
func readSSE(r io.Reader) (Message, Usage, error) {
	var content strings.Builder
	var usage Usage

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip blank lines, comments, and any field that is not data.
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk wireChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Message{}, Usage{}, fmt.Errorf("decode stream chunk: %w", err)
		}

		if chunk.Usage != nil {
			usage = Usage{ /* copy the three numbers */ }
		}
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return Message{}, Usage{}, fmt.Errorf("read stream: %w", err)
	}

	return Message{Role: RoleAssistant, Content: content.String()}, usage, nil
}
```

The struct it decodes into. Note how deep `content` sits:

```go
type wireChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
```

If you write `Delta` as a top-level field instead, it compiles. It decodes without a
single error. And it gives you an empty string for every message, so stdout is a blank
line and the program exits 0. That is exactly what `prints-reply` reports as
`actual: "\n"`.

### Why the scanner buffer has to be raised

`bufio.Scanner` holds a 64 KiB limit per line by default. A message carrying a large
tool argument, or a provider that batches replies, can go over it. When it does,
`Scan` returns `false` and the only sign is `scanner.Err()` returning
`bufio.ErrTooLong`. Without that check you get a short answer, no error, and nothing to
go on.

Raising the cap to 4 MiB is a guess, and every agent guesses. Production code either
raises it (pi does) or replaces the scanner with a reader that has no line limit. We
raise it and write down the trade.

### Why `[DONE]` is checked before parsing

It is not JSON. `json.Unmarshal` on `[DONE]` fails with `invalid character 'D'`, which
the naive version reports as a decoding error, on a stream that was perfectly healthy.
This is the most common first-run failure in the chapter, and it looks like the server
is broken.

## Job 5: build the answer once

```go
msg := Message{Role: RoleAssistant, Content: content.String()}
return msg, usage, nil
```

Nothing clever. The point is *where* it happens: after the loop, once. Code that builds
the `Message` inside the loop and returns early ends up with a message holding only the
last piece. The loop's job is to fill in the running values; the return's job is to take
one copy at the end.

---

## The four fixes

Each is a real failure with a check that catches it. Left is what a careful first
attempt does. Right is the fix.

**1. Overwrite instead of add** (`prints-reply`)

```diff
- msg.Content = choice.Delta.Content
+ content.WriteString(choice.Delta.Content)
```

**2. Ignore the scanner's error** (`truncated-stream`)

```diff
  for scanner.Scan() { ... }
+ if err := scanner.Err(); err != nil {
+     return Message{}, Usage{}, err
+ }
  return Message{Content: content.String()}, usage, nil
```

**3. Only decode messages that carry text** (`debug-usage`)

```diff
- if choice.Delta.Content == "" { continue }
  if chunk.Usage != nil { usage = ... }
```

**4. Use the background context** (`request-timeout`)

```diff
- http.NewRequest(http.MethodPost, url, body)
+ http.NewRequestWithContext(ctx, http.MethodPost, url, body)
```

That is the whole chapter: one parser, one way to fail, one lifetime. The other two
checks (`request-shape`, `http-error`) are structural. You either have them, or your
first run says so immediately.

## What production adds

| Feature | Where | Why it is not here |
|---|---|---|
| retry with backoff and `Retry-After` | `api/openai-completions.ts` | retries are their own chapter, later in the course |
| prompt caching headers | `api/openai-prompt-cache.ts` | provider-specific, nothing to learn |
| about thirty providers | `providers/*.ts` | one teaches the idea; thirty teach maintenance |
| abort in the middle of a tool call | `agent/agent-loop.ts` | the Steer and abort chapter |
| counting tokens before sending | `agent/context_tokens.go` in PiG | the Context budget chapter |

The one idea worth remembering for later: `pi` counts tokens *before* it sends, so it
can refuse a request instead of learning about the overflow from a 400. That is a design
choice, and it is much easier to add while `Chat` is still small.

## What to compare against your own

Look at `reference/ch01` and check these. Everything outside the list is style.

- [ ] separate request types, not `json` tags on the types you use everywhere
- [ ] `*int` for optional numbers
- [ ] the `client.Do` error returned untouched
- [ ] a typed `*HTTPError`
- [ ] the scanner buffer raised
- [ ] `[DONE]` handled before parsing
- [ ] `scanner.Err()` checked
- [ ] the message built after the loop

Every line in that list is a bug waiting for a later chapter. That is why it is short.
