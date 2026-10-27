# 01 Problem

> **Your task:** make the program answer a question. Running `agent -p "hello"`
> should print the model's reply.

You are not writing a library for a test to call. You are writing a program a person
runs in a terminal. Every later chapter adds another feature to this same program.

**You edit:** `agent/provider_openai.go`
**You are given:** `agent/cmd/agent/main.go` (the program), `agent/types.go`
**Do not edit:** `internal/`, `chapters/`
**Time:** 30 to 50 minutes

---

## What is already there

The program exists. It reads a few flags and one line of output is your job:

```sh
agent -p "say something"          # the answer goes to stdout
agent -p "hi" -m gpt-4o-mini      # pick a different model
agent -p "hi" -timeout 5s         # give up after five seconds
```

It also reads configuration from the environment:

| Variable | What it is | Default |
|---|---|---|
| `OPENAI_BASE_URL` | the address of the server, ending in `/v1` | `http://localhost:11434/v1` |
| `OPENAI_API_KEY` | the password the server expects | empty |
| `HARNESS_MODEL` | which model to ask for, when `-m` is not given | `gpt-4o-mini` |
| `HARNESS_DEBUG` | set it to anything to print the token counts | not set |

The one part missing is this function:

```go
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error)
```

`Request`, `Message`, `Usage` and `HTTPError` are defined in `agent/types.go`. Open
that file first. It is short, and those four types are everything you need to return.

## What to send

An HTTP `POST` to `{BaseURL}/chat/completions`, with `"stream": true` in the body. If
you leave that field out, the server sends one big reply instead of pieces, and your
parser will sit there waiting for something that never comes.

The body has the model, the messages, and the stream flag. Nothing else is required.

## What comes back, exactly

The server answers with `text/event-stream`: a series of lines, each starting with
`data:` followed by a small JSON object. Here is one:

```json
{
  "choices": [
    { "index": 0, "delta": { "content": "The " }, "finish_reason": null }
  ],
  "usage": { "prompt_tokens": 42, "completion_tokens": 7, "total_tokens": 49 }
}
```

The text is at `choices[0].delta.content`. Not at the top of the object. That is the
detail people get wrong, and the reason a wrong parser can run without any error and
still produce nothing.

The `usage` field appears once, on the last message of the stream, and that message has
no text in it.

The very last line is not JSON:

```
data: [DONE]
```

It means the stream has ended. Skip it. Do not try to parse it.

## How you are checked

The verifier builds your program, starts a real HTTP server that replays a fixed
conversation, points `OPENAI_BASE_URL` at that server, and runs your binary. Then it
looks at three things:

1. what your program **printed**, and whether it exited 0 or 1;
2. what **request** it sent, decoded from the real headers and body;
3. whether the run made the expected number of requests.

Nothing calls your code directly. However you organise it is up to you.

## The nine things it checks

- `prints-reply`: stdout is the full text and one newline, exit code 0, even when the
  reply arrives in several pieces.
- `request-shape`: the request goes to `/v1/chat/completions`, sends
  `Authorization: Bearer <key>`, uses the model from `-m`, sets `"stream": true`, and
  sends the messages in order.
- `request-envelope`: `Content-Type: application/json` and
  `Accept: text/event-stream` are set.
- `optional-zero-values`: if no token limit or temperature is given, those keys are
  left out of the body instead of being sent as zero.
- `sse-framing`: comment lines, `event:` lines, CRLF line endings, and `data:` with no
  space after the colon are all still understood.
- `debug-usage`: with `HARNESS_DEBUG=1`, the token counts go to stderr and stdout still
  contains only the answer.
- `http-error`: a 429 exits 1, prints nothing to stdout, and mentions the status on
  stderr.
- `truncated-stream`: a connection that dies halfway exits 1 with an error.
- `request-timeout`: with `-timeout 50ms` and a server that never answers, the program
  gives up quickly and exits 1.

## Things worth knowing before you start

**Three of these only fail for code that looks right.** A reply in one piece passes
whether you overwrite or add. A server that answers instantly passes whether or not you
pass the context along. So test with a long reply and a slow server. Otherwise you will
pass this chapter with bugs that only show up in chapter 08.

**An empty reply is not an error.** Return an empty message and a nil error.

**Print once, at the end.** Do not print inside the loop that reads the stream. Collect
the text, then print it after the stream closes. Printing as you go looks fine in a
terminal and breaks the checks, which expect the answer and nothing else on stdout.

**Some things are left out on purpose.** No retries, no backoff, no prompt caching, no
provider-specific options. `pi` does all of that in
`packages/ai/src/api/openai-completions.ts`; read it after you pass, not before.

## If you get stuck

<details><summary>First hint: what kind of problem is this?</summary>

Three separate things can go wrong, and each has its own check: the request you send,
the way you put the pieces back together, and what you do when something fails. Work
out which one you are in before reading further.
</details>

<details><summary>Second hint: the tools you will need</summary>

`http.NewRequestWithContext` to build the request with the caller's context.
`json.Marshal` for the body. `bufio.Scanner` to read lines, with a larger buffer than
the default. `strings.Builder` to collect the text. `io.LimitReader` to avoid reading a
giant error page.

A scanner gives you lines, not events. The blank lines between events can be skipped.
</details>

<details><summary>Third hint: what the finished function does</summary>

Build the body, POST it with the context you were given. Look at the status code before
reading the body. If it is not 200 to 299, read a few KiB and return
`&agent.HTTPError{Status, Body}`.

Otherwise read line by line. Keep the lines that start with `data:`, skip `[DONE]`,
and decode each remaining line into a struct with `Choices[].Delta.Content` and a
`Usage`. Add each piece of text to the end of a `strings.Builder`. Remember the usage
when it appears.

After the loop, check `scanner.Err()` and return it if it is not nil. Then build the
message from what you collected and return it.

`kata hint 01 <case-id>` prints these same three hints in the terminal.
</details>

## Done when

`kata submit 01` reports 9/9, and:

- `./agent -p "hello"` works against Ollama or any OpenAI-compatible server;
- you can explain, without looking, why ignoring `scanner.Err()` is dangerous;
- you know which of the traps above caught you, and which check named it.
