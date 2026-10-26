# 01 — Example

*Full, annotated implementation. Open this after a real attempt — ideally after
`kata submit 01` passes, then compare. Reading it first turns the exercise into
copy-typing.*

The complete compilable file is `reference/ch01/agent/provider_openai.go`. This
document is that file taken apart, in the order the thinking happens.

---

## The shape, before the code

Four jobs, four functions. Knowing the split matters more than the code:

```
Chat
 ├── buildRequest   Request  →  *http.Request     (translate our types → wire)
 ├── do            send, check status, or fail    (transport + error policy)
 └── fold           io.Reader → Message, Usage    (the delta log → one value)
```

If you write `Chat` as one 90-line function you will pass, and you will hate it
in chapter 02 when the fold grows tool calls. Written as three functions, chapter
02 adds ~15 lines and touches nothing else.

## Step 1 — translate, don't leak

Your `Message` and the wire's message are similar but not the same. Keep a wire
type and a conversion function; never put `json:` tags on your domain types.

```go
// Why a separate type: the wire has `tool_calls` as a nested object, ours has
// ToolCalls as a flat slice. When those diverge in chapter 02, only this file
// changes.
type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Tools       []wireTool    `json:"tools,omitempty"`
}

func toWireMessages(msgs []Message) []wireMessage { /* role + content */ }
```

Two details that are cheap now and expensive later:

- `*int` / `*float64` for optionals. With `int`, zero means both "unset" and
  "zero", and you will one day send `max_tokens: 0`.
- The model falls back: `req.Model` first, then `p.Model`. Chapter 05 sends
  requests with no model when it reuses a session's provider.

## Step 2 — the request, and the three things that are graded

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

- `TrimRight(base, "/")` so both `http://x/v1` and `http://x/v1/` work. Half the
  "why is it 404" reports in the world are this.
- `NewRequestWithContext`, not `NewRequest`. This is the only place `ctx` enters,
  and it is what makes `context-cancel` pass.
- `stream: true` in the body is **required by the case**, even though our
  function returns a whole message. We stream because chapter 06 will want
  incremental events, and because learning the stream is the chapter.

## Step 3 — fail the way a caller can act on

```go
resp, err := client.Do(httpReq)
if err != nil {
	return Message{}, Usage{}, err   // already wraps ctx.Err() when cancelled
}
defer resp.Body.Close()

if resp.StatusCode < 200 || resp.StatusCode >= 300 {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return Message{}, Usage{}, &HTTPError{Status: resp.StatusCode, Body: string(snippet)}
}
```

Three decisions worth defending:

- **Return `client.Do`'s error unchanged.** It already wraps `context.Canceled`;
  wrapping it in a string would break `errors.Is` and fail `context-cancel`.
- **`LimitReader`, 4 KiB.** Error pages are sometimes megabytes of HTML. You want
  a diagnostic, not a memory event.
- **A typed error, not `fmt.Errorf`.** `*HTTPError` is what lets the loop in
  chapter 04 distinguish "retry with backoff" from "the model is wrong".

## Step 4 — fold the stream

This is the chapter. Everything above is plumbing.

```go
func readSSE(r io.Reader) (Message, Usage, error) {
	var (
		content strings.Builder
		usage   Usage
		calls   = map[int]*callAccumulator{}   // chapter 02; unused here
	)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE framing: blank separators, comments, and named fields other than data.
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

		if chunk.Usage != nil {          // ← arrives once, on the last chunk
			usage = Usage{ /* ... */ }
		}
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)   // ← append, never assign
			// tool_calls folding lives here from chapter 02
		}
	}
	if err := scanner.Err(); err != nil {              // ← the 3rd trap
		return Message{}, Usage{}, fmt.Errorf("read stream: %w", err)
	}

	return Message{Role: RoleAssistant, Content: content.String()}, usage, nil
}
```

### Why `scanner.Buffer` is not optional

`bufio.Scanner` has a 64 KiB default line limit. A chunk carrying a large tool
result argument, or a provider that batches aggressively, will exceed it. When it
does, `Scan` returns `false` and the *only* signal is `scanner.Err()` returning
`bufio.ErrTooLong`. You will see: a truncated answer, no error, no clue.

Raising the cap to 4 MiB is a guess; every harness guesses. Production code
(pi included) either raises it or replaces `Scanner` with a `Reader`-based
framing loop that has no per-line ceiling. We raise it, and note the tradeoff.

### Why `[DONE]` must be checked before unmarshalling

It is not JSON. `json.Unmarshal` on `[DONE]` returns
`invalid character 'D' looking for beginning of value`, which the naive
implementation wraps as "decode stream chunk" and reports as a protocol error —
on a completely healthy stream. This is the single most common first-run failure
in this chapter.

## Step 5 — assemble outside the loop

```go
msg := Message{Role: RoleAssistant, Content: content.String()}
return msg, usage, nil
```

Banal, but note *where* it happens: outside. Implementations that build the
`Message` inside the loop and return it early end up with a message that has only
the last delta. The loop's job is to mutate accumulators; the return's job is to
snapshot them once.

---

## The four diffs

Each of these is a real failure mode with a graded case. Left is what a
competent first implementation does; right is the fix.

**1. Assign instead of append** — `text-reassembly`

```diff
- msg.Content = choice.Delta.Content
+ content.WriteString(choice.Delta.Content)
```

**2. Ignore the scanner's error** — `truncated-stream`

```diff
  for scanner.Scan() { ... }
+ if err := scanner.Err(); err != nil {
+     return Message{}, Usage{}, err
+ }
  return Message{Content: content.String()}, usage, nil
```

**3. Only decode chunks with content** — `usage`

```diff
- if choice.Delta.Content == "" { continue }
  if chunk.Usage != nil { usage = ... }
```

**4. Background context** — `context-cancel`

```diff
- http.NewRequest(http.MethodPost, url, body)
+ http.NewRequestWithContext(ctx, http.MethodPost, url, body)
```

That is the whole chapter: one fold, one error policy, one lifetime. The other
two cases (`request-shape`, `http-error`) are structural and you either have them
or your first run tells you immediately.

## What production adds, and why we don't

| Feature | File in pi | Why it is out of scope here |
|---|---|---|
| Retry with backoff + `Retry-After` | `api/openai-completions.ts` | chapter 04 gives you one retry path; a full policy is a chapter of its own |
| Prompt caching headers | `api/openai-prompt-cache.ts` | provider-specific, no teaching value |
| ~30 provider wire formats | `providers/*.ts` | one adapter teaches the abstraction; thirty teach maintenance |
| Abort mid-tool-call | `agent/agent-loop.ts` | chapter 08 |
| Token counting before send | `agent/context_tokens.go` (PiG) | chapter 07 |

The one thing worth stealing early, when you get to chapter 07: pi estimates
tokens *before* sending, so it can refuse a request rather than discover the
overflow from a 400. That is a design decision, not a feature, and it is easier
to add while `Chat` is still small.

## Comparison checklist

When you look at `reference/ch01`, check whether yours differs in:

- [ ] separate wire types vs `json` tags on domain types
- [ ] `*int` for optionals
- [ ] `client.Do` error returned unwrapped
- [ ] typed `*HTTPError`
- [ ] `scanner.Buffer` raised
- [ ] `[DONE]` handled before `Unmarshal`
- [ ] `scanner.Err()` checked
- [ ] message assembled outside the loop

Differences outside this list are style. Differences inside it are bugs waiting
for a later chapter.
