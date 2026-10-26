# 01 — Talk to the model

> **You will build:** a client for any OpenAI-compatible endpoint that turns a
> Server-Sent-Events stream back into one `Message`.
>
> **Provable when:** `craft test 01` is green — and green *again* after every
> later chapter, because later chapters keep calling this.
>
> **Time:** 20–40 minutes if you have written an HTTP client before.

---

## Why this is chapter one

Every harness is a loop around a function that talks to a model. That function
is the only part of the system that is not yours: it fails on someone else's
schedule, costs money per call, answers differently every time, and is
impossible to test.

So you put a seam around it on day one. Not as a design flourish — as the thing
that makes the next nine chapters gradable at all.

```go
type Provider interface {
    Chat(ctx context.Context, req Request) (Message, Usage, error)
}
```

Above the seam: deterministic. Below it: a stranger.

`agentcraft/internal/fake` is a scripted server sitting below the fake side of
that seam. It is not a mock of your code; it is a real HTTP server speaking the
real wire format, with the model's behaviour replaced by a fixture. When your
checks pass, you have not "mocked the HTTP call" — you have genuinely spoken the
protocol, just to a server whose answers you control.

This is the single most important habit in the whole course:

> **You cannot test an agent. You can test the harness around the model.**

---

## The wire format, precisely

A request is one `POST {baseURL}/chat/completions`:

```http
POST /v1/chat/completions HTTP/1.1
Content-Type: application/json
Authorization: Bearer <key>

{"model":"gpt-4o-mini",
 "messages":[{"role":"system","content":"You are terse."},
             {"role":"user","content":"hi"}],
 "stream":true}
```

The response is `text/event-stream`. Each event is a line beginning `data: `
followed by a JSON chunk, then a blank line:

```
data: {"choices":[{"delta":{"role":"assistant","content":""},"index":0}]}

data: {"choices":[{"delta":{"content":"The quick "},"index":0}]}

data: {"choices":[{"delta":{"content":"brown fox."},"index":0}],"usage":{"prompt_tokens":42,"completion_tokens":7,"total_tokens":49}}

data: [DONE]
```

Three properties matter, and each has a graded failure mode:

1. **Deltas append.** `"The quick "` then `"brown fox."` means `"The quick brown
   fox."`. If you assign, you get the last fragment. One-chunk tests pass;
   real answers truncate. → case `text-reassembly`
2. **`[DONE]` is not JSON.** It terminates the stream. Parsing it is a crash; not
   handling it is an infinite wait.
3. **Usage arrives last, and only once.** It rides on a chunk with an empty
   delta. If you only decode chunks that carry content, you never see it, and
   chapter 07 (context budget) becomes impossible. → case `usage`

There is also SSE framing you must tolerate and mostly ignore: `:` comment
lines, `event:` and `id:` fields, and blank lines between events. Harnesses that
split on `\n\n` work until a provider sends a comment; scanners are simpler.

---

## Two failures worth internalising now

**A 429 is not a parse error.** Check the status before you touch the body, and
return a typed error:

```go
type HTTPError struct { Status int; Body string }
```

Later this is what lets the loop back off and retry instead of dying, and lets
the CLI say "rate limited, retrying in 4s" instead of "invalid character 'e'".
→ case `http-error`

**A truncated stream is not a short answer.** `bufio.Scanner` stops on error and
remembers it. If you never call `scanner.Err()`, a dropped connection looks like
a successful, brief reply — and your agent silently "finishes" mid-sentence.
This is the bug essentially every harness ships once.
→ case `truncated-stream`

---

## How to work

```bash
craft test 01          # grade everything in this chapter
craft hint 01 usage    # stuck? three tiers, escalating
craft test 01 --solution   # only after a real attempt
```

Read the failure, not the hint, first. The failure names the mechanism; the hint
only names the API.

You are editing exactly one file: `harness/provider_openai.go`. `harness/types.go`
is scaffolding. The checks in `chapters/01-chat-with-the-model/checks/` import
your package through the `Provider` interface, which is why they will keep
working when you replace your implementation in chapter 02.

---

## Reading, if you want the real thing

- `packages/ai/src/api/openai-completions.ts` in
  [`earendil-works/pi`](https://github.com/earendil-works/pi) — pi's version of
  this exact file, with retries, prompt caching, and ten providers of
  accumulated scar tissue. Note how the seam is the same shape you just wrote.
- `internal/agent/` in [`charmbracelet/crush`](https://github.com/charmbracelet/crush)
  — the Go idiom, including `convert_test.go`, which does what your checks do.

---

## Definition of done

- [ ] `craft test 01` → 6/6
- [ ] You can explain, without looking, why `scanner.Err()` matters
- [ ] You know which of the three traps bit you, and why the test caught it

Next: **02 — Tool calling**, where the same stream carries JSON that arrives in
pieces, and the fragments are keyed by an index rather than an id.
