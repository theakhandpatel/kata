# 18 Building the request body

**Part VI · Operate** · Difficulty: easy-medium · 30 to 45 minutes · 7 graded cases

## What you are building

The function that turns a request into the JSON a server reads.

```sh
$ ./agent -p "hi" -m a-model -max-tokens 128 -temperature 0.4
{
  "model": "a-model",
  "messages": [{"role": "user", "content": "hi"}],
  "stream": true,
  "max_tokens": 128,
  "temperature": 0.4
}
```

Seven fields, and every one of them is a decision about presence.

You may edit `agent/request.go`, `agent/provider_openai.go` and
`agent/cmd/agent/main.go`.

## Three fields are always there

```
  model      which model to ask
  messages   the conversation, in order
  stream     whether the reply arrives in pieces
```

Missing any of them is a 400 that reads like a broken client.

`stream` comes from the request now, rather than being hardcoded to true. A request that
asks for one buffered answer is a legitimate request, and the body should say what was
asked.

## Two fields are there only when set

```
  max_tokens    a token limit for the reply
  temperature   how much variation to allow
```

Zero is not absent. A provider reads `"max_tokens": 0` as "produce nothing", and
`"temperature": 0` as a deliberate choice of no variation. So neither key may appear
unless the caller set it.

```go
if req.MaxTokens > 0 {
	body["max_tokens"] = req.MaxTokens
}
```

That is the whole difference between a working client and one that returns empty answers
against a real provider and looks fine in every unit test.

## One field is present even when empty

```go
if req.Tools != nil {
	body["tools"] = ...
}
```

An empty tool list is a statement: this request has no tools. Omitting the key is a
different statement, and some servers reject it. The distinction only exists because the
field is a slice, which can be nil, rather than an int, which cannot.

## The header decides the response

```
  Content-Type: application/json      what you sent
  Accept: text/event-stream           what you can read
```

Without `Accept`, a server is free to answer with one buffered response. The body of that
response is valid JSON, and your line-by-line parser waits for lines that never come.

## The path

```go
strings.TrimRight(base, "/") + "/chat/completions"
```

`http://x/v1` and `http://x/v1/` both work. Without the trim, one of them produces a
doubled slash and a 404 that looks like a wrong route.

## What you need to build

```go
func BuildRequestBody(model string, req Request) map[string]any
```

And one field on the request type:

```go
type Request struct {
	// ...
	Stream bool   // ask for the reply as it arrives
}
```

Three flags, so the checks can set them:

```sh
agent -p "..." -m a-model -max-tokens 128 -temperature 0.4
```

## How you are checked

The verifier builds your program, runs it, and decodes the body the server received.

- `required-fields-always-present`: `model`, `messages` and `stream` are all present, and
  `stream` is `true`.
- `unset-optionals-absent`: with no limit or temperature given, neither key is present.
- `set-optionals-present`: with `-max-tokens 128 -temperature 0.4`, both arrive with
  those values.
- `messages-keep-their-order`: the system message is at index 0 and the prompt is at
  index 1, unchanged.
- `headers-decide-the-response`: `Content-Type` is `application/json` and `Accept`
  contains `text/event-stream`.
- `base-url-trailing-slash`: a base URL with a trailing slash still produces
  `/v1/chat/completions`.
- `empty-tool-list-is-sent`: `tools` is present and is an array.

## Notes

Build the body as a map rather than a struct. A struct with `omitempty` cannot tell
"absent" from "zero" for an `int`, which is the bug this chapter exists to prevent. A map
makes presence explicit.

Order the fields in the same order every time if you marshal a struct, because a body
whose key order changes between calls costs the provider its prefix cache.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either a required field is missing, an unset optional was sent as zero, or the headers do
not match the body.
</details>

<details><summary>Second hint: what you will need</summary>

`map[string]any` for the body, `strings.TrimRight` for the base URL, `json.Marshal` at
the end, and `http.Header.Set` for the two headers.
</details>

<details><summary>Third hint: the finished builder</summary>

Start the map with `model`, `messages` and `stream`. Add `max_tokens` and `temperature`
only when positive. Add `tools` when the slice is not nil, wrapping each definition as
`{"type":"function","function":{name, description, parameters}}`. Marshal the map, POST it
to the trimmed base URL plus `/chat/completions`, and set `Content-Type` and `Accept`.

`kata hint 18 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 18` reports 7/7, and a request with no limit or temperature carries neither
key.

[example.md](example.md) has the finished code, after you pass.
