# 01 — Problem

> **Your task:** make the program answer a prompt. `agent -p "hello"` should
> print the model's reply on stdout.

You are not writing a library for a test to call. You are writing the first
version of an application: a program a person runs in a terminal. Later chapters
add tools, loops and sessions to this same program.

**You edit:** `agent/provider_openai.go`
**Given:** `agent/cmd/agent/main.go` (the program), `agent/types.go`
**Do not edit:** `internal/`, `chapters/`
**Time:** 30 to 50 minutes

---

## The contract

The program is already written. It reads three inputs and prints one output:

```sh
agent -p "say something"          # the answer goes to stdout
agent -p "hi" -m gpt-4o-mini      # model override
agent -p "hi" -timeout 5s         # request timeout
```

It reads configuration from the environment:

| Variable | Used for | Default |
|---|---|---|
| `OPENAI_BASE_URL` | the endpoint, ending in `/v1` | `http://localhost:11434/v1` |
| `OPENAI_API_KEY` | the bearer token | empty |
| `HARNESS_MODEL` | model name when `-m` is absent | `gpt-4o-mini` |
| `HARNESS_DEBUG` | set to any value to log token usage to stderr | unset |

Then it needs one thing from you:

```go
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error)
```

`Request`, `Message`, `Usage` and `HTTPError` are in `agent/types.go`. Read that
file first. It is short and it is scaffolding.

The request goes to `{BaseURL}/chat/completions` with `"stream": true`. The server
answers with `text/event-stream`, one JSON object per `data:` line. This is the
chunk shape, and the nesting matters:

```json
{
  "choices": [
    { "index": 0, "delta": { "content": "The " }, "finish_reason": null }
  ],
  "usage": { "prompt_tokens": 42, "completion_tokens": 7, "total_tokens": 49 }
}
```

`content` is at `choices[0].delta.content`, not at the top level. `usage` appears
once, on the final chunk, whose `delta` is empty.

## What the verifier does

It builds `./agent/cmd/agent`, starts a scripted HTTP server that speaks the
real protocol, points `OPENAI_BASE_URL` at it, and runs your binary. Then it
checks three things:

1. what your program **printed** on stdout and stderr, and its exit code;
2. what **requests** it sent, decoded from the actual HTTP body and headers;
3. whether the conversation used exactly the scripted number of turns.

Nothing calls your package directly. The internal design is yours.

## The verifier will assert that

- `prints-reply` — stdout is the model's full text plus one newline, and the exit
  code is 0, even when the text arrives in several pieces.
- `request-shape` — the request goes to `/v1/chat/completions`, carries
  `Authorization: Bearer <key>`, sets the model from `-m`, sets `"stream": true`,
  and serialises messages as `{role, content}` in order.
- `request-envelope` — `Content-Type: application/json` and
  `Accept: text/event-stream` are both set.
- `optional-zero-values` — when no token limit or temperature is set, those keys
  are absent from the body rather than sent as zero.
- `sse-framing` — comment lines, `event:` fields, CRLF endings, and `data:` with
  no space after the colon are all tolerated.
- `debug-usage` — with `HARNESS_DEBUG=1`, stderr reports the prompt, completion
  and total token counts, and stdout still contains only the answer.
- `http-error` — a 429 exits 1, prints nothing on stdout, and names the status on
  stderr.
- `truncated-stream` — a connection that dies mid-stream exits 1 with an error,
  and the request was actually made.
- `request-timeout` — with `-timeout 50ms` and a server that never answers, the
  program exits 1 quickly, having made the request.

## Notes

Three of these cases only fail for code that looks right. A one-chunk reply
passes whether you append or assign. A server that answers instantly passes
whether or not you pass the context. Test with a long reply and a slow server.

- An empty reply is not an error. Return an empty message with no error.
- Do not print the answer with `fmt.Printf` inside the stream loop. Print once,
  at the end, after the stream closes.
- Deliberate omission: no retries, no backoff, no prompt caching, no extra
  provider options. pi does all of that in
  `packages/ai/src/api/openai-completions.ts`; read it after you pass.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

Three separate things break here, and each has its own case: what the request
looks like, how the deltas are folded, and how the program fails.
</details>

<details><summary>Rung 2 — the APIs</summary>

`http.NewRequestWithContext`, `json.Marshal`, `bufio.Scanner` with a raised
`Buffer` limit, `strings.Builder`, `io.LimitReader`.

The scanner returns lines, not events. The line `data: [DONE]` ends the stream
and is not JSON.
</details>

<details><summary>Rung 3 — what the code does</summary>

Build the body and POST it with the caller's ctx. Check the status before reading
the body; on non-2xx, read at most a few KiB and return
`&agent.HTTPError{Status, Body}`. Otherwise scan lines, keep those starting
`data:`, skip `[DONE]`, decode each into a chunk struct with
`Choices[].Delta.Content` and a top-level `Usage`, append every content delta, and
take `usage` when it is non-nil. After the loop, check `scanner.Err()` and
return it. Assemble the returned message outside the loop.

`kata hint 01 <case-id>` prints the same rungs from the CLI.
</details>

## Done when

`kata submit 01` reports 9/9, and:

- `./agent -p "hello"` works against Ollama or any OpenAI-compatible endpoint;
- you can explain why `scanner.Err()` matters without looking;
- you know which of the three traps bit you, and which case caught it.
