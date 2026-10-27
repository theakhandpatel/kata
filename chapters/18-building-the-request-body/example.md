# 18 Example

*The finished implementation, taken apart. Open this after `miyagi submit 18` passes.*

The file it describes is `reference/ch18/agent/request.go`.

---

## A map, not a struct

```go
func BuildRequestBody(model string, req Request) map[string]any {
	body := map[string]any{
		"model":    model,
		"messages": toWireMessages(req.Messages),
		"stream":   req.Stream,
	}
	...
}
```

A struct with `json:"max_tokens,omitempty"` cannot express what this chapter needs. Go's
`omitempty` treats `0`, `""`, `false` and `nil` as empty, so an `int` field at zero is
omitted whether the caller meant "no limit" or "a limit of zero". Those are different
requests, and only one of them is valid.

A pointer would work, and it is what earlier chapters used. A map works better here
because presence becomes a lookup rather than a nil check, and because the builder's whole
job is deciding what is present.

## Presence, one field at a time

```go
if req.MaxTokens > 0 {
	body["max_tokens"] = req.MaxTokens
}
if req.Temperature > 0 {
	body["temperature"] = req.Temperature
}
```

"Greater than zero" rather than "not nil", because the field on `Request` is a plain
number. The meaning is: the caller set a limit. A limit of zero is not a limit.

If a future field needed "zero is meaningful", it would need a pointer or a separate
boolean. Saying that here is cheaper than discovering it in production.

## Tools: present even when empty

```go
if req.Tools != nil {
	tools := make([]map[string]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	body["tools"] = tools
}
```

`make(..., 0, len)` produces an empty array rather than `null` when the slice is empty.
`json.Marshal` of a nil slice writes `null`, and a server expecting an array rejects it.

The `req.Tools != nil` guard is the only place the nil-versus-empty distinction survives
all the way to the wire. Earlier chapters filled the slice from a registry, so it is
usually non-nil; a request built by hand may not be.

## The nested shape

The wire format for a tool is two levels deep:

```json
{"type": "function", "function": {"name": "read", "description": "...", "parameters": {}}}
```

`type` is always `"function"`. There was a time when other types existed; there is one
now, and it is still required.

## Messages in order

```go
func toWireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, wireMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  ...,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		})
	}
	return out
}
```

A straight walk. No sorting, no grouping by role, no deduplication. The conversation is a
sequence, and the server reads it as one.

The empty-message case is worth a thought: a `Content` of `""` on an assistant message
with tool calls is correct, because the message carries calls instead of text. A server
that receives text and calls together rejects it.

## Why the provider changed

```go
body, err := json.Marshal(BuildRequestBody(model, req))
```

One line replaces the inline struct literal. The provider keeps its job, which is
transport and streaming, and the body builder keeps its job, which is presence.

That split is what makes this chapter testable at all: the checks read the decoded body,
so they do not care how it was produced.

## The fixes that matter

**1. Struct with omitempty** (`unset-optionals-absent`)

```diff
- type wireRequest struct { MaxTokens int `json:"max_tokens,omitempty"` }
+ if req.MaxTokens > 0 { body["max_tokens"] = req.MaxTokens }
```

**2. Sending zero** (`set-optionals-present`)

```diff
- body["max_tokens"] = req.MaxTokens        // always, even at zero
+ if req.MaxTokens > 0 { body["max_tokens"] = req.MaxTokens }
```

**3. Nil tools marshalled as null** (`empty-tool-list-is-sent`)

```diff
- body["tools"] = toWireTools(req.Tools)     // nil becomes null
+ tools := make([]map[string]any, 0, len(req.Tools))
+ body["tools"] = tools
```

**4. A doubled slash** (`base-url-trailing-slash`)

```diff
- url := p.BaseURL + "/chat/completions"
+ url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
```

## What to compare against your own

- [ ] all three required fields present on every request
- [ ] unset options absent, set options present with the right values
- [ ] an empty tool list is an array, not null
- [ ] messages keep their order and their roles
- [ ] both headers set
- [ ] the base URL is trimmed
- [ ] the body is built in one place

Everything outside that list is style.
