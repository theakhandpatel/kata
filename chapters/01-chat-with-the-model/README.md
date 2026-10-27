# 01 Talk to the model

**Part I · Wire** · Difficulty: easy-medium · 30 to 50 minutes · 9 graded cases

## What you are building

A program that answers a question:

```sh
$ ./agent -p "say something"
The quick brown fox jumps over the lazy dog.
```

The program exists. It reads the flags, builds the configuration, prints the answer and
exits. One function is missing:

```go
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error)
```

Edit `agent/provider_openai.go` and nothing else. `agent/types.go` defines the four
types you return. `agent/cmd/agent/main.go` is the program.

## The request

`POST {BaseURL}/chat/completions`, with this body:

```json
{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "say something"}],
  "stream": true
}
```

`Content-Type: application/json`, `Accept: text/event-stream`, and
`Authorization: Bearer <key>` when there is a key. The base URL already ends in `/v1`.

`"stream": true` is not optional. Without it the server sends one large response and the
parser below waits for lines that never arrive.

| Variable | Meaning | Default |
|---|---|---|
| `OPENAI_BASE_URL` | server address, ending in `/v1` | `http://localhost:11434/v1` |
| `OPENAI_API_KEY` | the bearer token | empty |
| `HARNESS_MODEL` | model name when `-m` is absent | `gpt-4o-mini` |
| `HARNESS_DEBUG` | print token counts to stderr when set | unset |

## The response

`text/event-stream`: a sequence of lines, each beginning `data:` followed by a JSON
object. One object per line, one new piece of text per object.

```
data: {"choices":[{"delta":{"content":"The "}}]}
data: {"choices":[{"delta":{"content":"quick "}}]}
data: {"choices":[{"delta":{"content":"fox."}}]}
data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":42,"completion_tokens":7,"total_tokens":49}}
data: [DONE]
```

The text is at `choices[0].delta.content`:

```json
{
  "choices": [
    { "index": 0, "delta": { "content": "The " }, "finish_reason": null }
  ],
  "usage": { "prompt_tokens": 42, "completion_tokens": 7, "total_tokens": 49 }
}
```

`choices` is an array because one request may ask for several completions at once. This
one asks for a single answer, so it reads `choices[0]`.

`delta` means "the new part". The three messages above carry `"The "`, `"quick "` and
`"fox."`. Joined in order they are `"The quick fox."`. Building that string is the body
of the function.

Three properties of the stream:

- **Text accumulates.** Each message adds to the answer; none of them contains it.
- **`usage` arrives once, on the last message**, which carries no text. The server does
  not know the counts until it has finished writing.
- **`data: [DONE]` is not JSON.** It terminates the stream.

## What you are given

`Request`, `Message`, `Usage` and `HTTPError` in `agent/types.go`. `Request` carries the
model, the messages, the tool list, the token limit and the temperature. `Message` has a
role, content, tool calls and a tool call id. `HTTPError` holds a status and a body
excerpt.

## How you are checked

The verifier builds your binary, starts an HTTP server that speaks this protocol and
replays a fixed conversation, points `OPENAI_BASE_URL` at it, and runs your program. It
then reads what your program printed, its exit code, and the request it sent.

- `prints-reply`: stdout is the full text plus one newline, exit 0, across several pieces.
- `request-shape`: the path, the bearer token, the model from `-m`, `"stream": true`, and
  messages in order.
- `request-envelope`: `Content-Type` and `Accept` are set.
- `optional-zero-values`: with no token limit or temperature given, those keys are absent
  from the body.
- `sse-framing`: comment lines, `event:` lines, CRLF endings, and `data:` with no space
  after the colon.
- `debug-usage`: with `HARNESS_DEBUG=1`, counts to stderr and nothing extra on stdout.
- `http-error`: a 429 exits 1, prints nothing to stdout, names the status on stderr.
- `truncated-stream`: a connection that dies mid-stream exits 1 with an error.
- `request-timeout`: with `-timeout 50ms` and a server that never answers, exits 1
  quickly.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either the request is wrong, the parser does not recover the text, or a failure is
handled as success. The failing case id names which.
</details>

<details><summary>Second hint: what you will need</summary>

`http.NewRequestWithContext`, `json.Marshal`, `bufio.Scanner` with a buffer larger than
the default, `strings.Builder`, `io.LimitReader`.

A scanner yields lines, not events. Blank lines between events can be skipped.
</details>

<details><summary>Third hint: the finished function</summary>

Build the body and POST it with the context you were given. Read the status before the
body; if it is outside 200 to 299, read a few KiB and return
`&agent.HTTPError{Status, Body}`.

Otherwise scan lines. Keep those starting with `data:`, skip `[DONE]`, and decode each
into a struct holding `Choices[].Delta.Content` and a `Usage`. Append each piece of text
to a `strings.Builder`, and keep the usage when it appears.

After the loop, check `scanner.Err()`. Then build the message from the builder and
return it.

`kata hint 01 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 01` reports 9/9, and `./agent -p "hello"` answers against Ollama or any
OpenAI-compatible server.

[example.md](example.md) has the finished code, after you pass.

---

Next: **02 Tool calling**.
