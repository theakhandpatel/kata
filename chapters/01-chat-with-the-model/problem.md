# 01 — Problem

> **Your task:** implement `OpenAICompat.Chat` so that it speaks the
> OpenAI-compatible streaming protocol and returns one fully reassembled
> assistant message.

**You edit:** `harness/provider_openai.go`
**Do not edit:** `harness/types.go`, `internal/fake/`, `chapters/`
**Time:** 20–40 minutes

---

## The contract

```go
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error)
```

`Request` carries `Model`, `Messages`, `Tools`, `MaxTokens`, `Temperature`.
`Message` is `{Role, Content, ToolCalls, ToolCallID, Name}`. `Usage` is
`{PromptTokens, CompletionTokens, TotalTokens}`. All defined in
`harness/types.go` — read that file first, it is short and it is scaffolding.

The endpoint is `{BaseURL}/chat/completions`, where `BaseURL` already ends in
`/v1`. Auth is `Authorization: Bearer {APIKey}`. The request must set
`"stream": true`; the server answers with `text/event-stream`.

If the model field on the request is empty, fall back to `p.Model`.

## What the verifier does

It starts a real HTTP server (`internal/fake`) that speaks the real wire format
and replays a scripted reply. Then it calls **your** `Chat` and inspects two
things:

1. **the request you sent** — decoded from the actual HTTP body and headers;
2. **the message you returned** — compared field by field.

No network, no API key, no model. The scripted server is a genuine SSE server;
you are genuinely parsing a stream.

## The verifier will assert that

- `request-shape` — the request goes to `/v1/chat/completions`, carries
  `Authorization: Bearer <key>`, sets `model` from the request (or `p.Model`),
  sets `"stream": true`, and serialises messages as `{role, content}` in order.
- `text-reassembly` — text arriving as several deltas is **appended**, and the
  returned `Message.Content` equals the full text; `Role` is `assistant`.
- `usage` — token counts reported on the final chunk are returned in `Usage`.
- `http-error` — a `429` returns an error that is `*harness.HTTPError` with
  `Status == 429`, not a parse failure.
- `truncated-stream` — a connection that dies mid-stream returns a non-nil error.
- `context-cancel` — a cancelled `ctx` produces an error wrapping
  `context.Canceled`.

## Notes

- **Three of these cases only fail for code that "looks right".** A one-chunk
  test passes whether you append or assign. Test with a long reply.
- **Edge case:** an empty reply body is not an error — return an empty
  `Message` with no error. The fake server can send chunks with no content.
- **Deliberate omission:** you are not implementing retries, backoff, prompt
  caching, or provider-specific options. pi does all of it in
  `packages/ai/src/api/openai-completions.ts`; read it after you pass.
- **Output contract:** `Chat` returns values, it does not print. If you add
  debugging, use `log.Printf` (stderr). From chapter 03 onward this becomes a
  hard rule: stdout is reserved for the program's final answer.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

Three separate things break here, and each has its own case: what you *send*,
how you *accumulate*, and how you *fail*.
</details>

<details><summary>Rung 2 — the APIs</summary>

`http.NewRequestWithContext`, `json.Marshal`, `bufio.Scanner` with a raised
`Buffer` limit, `strings.Builder`, `io.LimitReader`.

The scanner returns lines, not events. The line `data: [DONE]` terminates the
stream and is not JSON.
</details>

<details><summary>Rung 3 — what the code does</summary>

Build the body and POST it with the caller's context. Check the status code
before reading the body; on non-2xx, read at most a few KiB and return
`&harness.HTTPError{Status, Body}`. Otherwise scan lines, keep those starting
`data:`, skip `[DONE]`, decode each into a chunk struct, append `delta.content`,
and read `usage` when it is non-nil. After the loop, check `scanner.Err()` and
return it. Assemble the returned `Message` outside the loop.

`lathe hint 01 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`lathe submit 01` is green (`6/6`), and:

- you can explain why `scanner.Err()` matters, without looking;
- you know which of the three traps bit you, and which case caught it.
