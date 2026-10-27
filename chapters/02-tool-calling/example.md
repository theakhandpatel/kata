# 02 Example

*The finished implementation, taken apart. Open this after a real attempt, or after
`miyagi submit 02` passes.*

The files it describes are `reference/ch02/agent/tool.go` and
`reference/ch02/agent/provider_openai.go`.

---

## The registry

```go
type Registry struct {
	byName map[string]Tool
	order  []string
}
```

Two fields, and the second one is not decoration. `byName` answers "is this tool
registered". `order` preserves declaration order so `Defs` returns the same list on
every request. A map iterated directly would reorder the schemas between calls, which
costs the model provider its prefix cache and makes request diffs unreadable.

```go
func (r *Registry) Defs() []ToolDef {
	out := make([]ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		out = append(out, ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return out
}
```

## Four ways a call fails, and all four are results

```go
func (r *Registry) Run(ctx context.Context, call ToolCall) ToolResult {
	t, ok := r.Lookup(call.Name)
	if !ok {
		return ToolResult{Content: "no such tool: " + call.Name, IsError: true}
	}
	if !json.Valid([]byte(call.Arguments)) {
		return ToolResult{
			Content: "arguments are not valid JSON: " + call.Arguments,
			IsError: true,
		}
	}
	out, err := t.Execute(ctx, json.RawMessage(call.Arguments))
	if err != nil {
		return ToolResult{Content: t.Name() + ": " + err.Error(), IsError: true}
	}
	return ToolResult{Content: out}
}
```

The order of those checks matters. The name is looked up first, because a tool that
does not exist has no schema to validate against. The JSON check comes second, so a
malformed call is reported as malformed rather than as a crash inside the tool. Only
then does the tool run.

`json.Valid` before `json.Unmarshal` is not padding. The arguments are text the model
wrote. Without the check, the decode fails inside `Execute`, the error propagates, and
the program exits on a mistake the model could have fixed if it had been told.

Every branch returns a `ToolResult`, never a Go error, because none of these are fatal.
The model reads the message and tries something else.

## Grouping the pieces

```go
acc := calls[tc.Index]
if acc == nil {
	acc = &callAccumulator{}
	calls[tc.Index] = acc
}
if tc.ID != "" {
	acc.id = tc.ID
}
if tc.Function.Name != "" {
	acc.name = tc.Function.Name
}
acc.args.WriteString(tc.Function.Arguments)
```

Three rules, each with a failure behind it:

| Rule | Wrong version | What breaks |
|---|---|---|
| key by index | key by id | everything after the first piece is lost |
| set the id only when present | overwrite with the empty id | the call has no id to pair with |
| append the arguments | assign them | only the last few characters survive |

Then emit in index order, because the assistant message's call order is what the
results must match.

```go
indexes := make([]int, 0, len(calls))
for i := range calls {
	indexes = append(indexes, i)
}
sort.Ints(indexes)
```

## Two directions of wire format

The provider already sends `messages`. It gains two shapes:

```go
type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}
```

An assistant message carries `tool_calls`. A tool message carries `tool_call_id` and
the content. A conversation with one and not the other is rejected, which is why the
next chapter's state machine has to append both halves.

## The stub

The chapter's checks need a registered tool to call, so this reference includes one:

```go
type StubTool struct{}
func (t *StubTool) Name() string { return "read" }
func (t *StubTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	return "stub", nil
}
```

It answers `"stub"` and nothing else. It exists to make the protocol testable: a name
the registry knows, so a call can be dispatched and a result observed. The next chapter
replaces it with something that opens a file.

## The fixes that matter

**1. Keying by id** (`folds-arguments`)

```diff
- acc := calls[tc.ID]
+ acc := calls[tc.Index]
```

**2. Dropping the id on later pieces** (`result-carries-call-id`)

```diff
- acc.id = tc.ID
+ if tc.ID != "" { acc.id = tc.ID }
```

**3. Returning an error instead of a result** (`unknown-tool-is-a-result`)

```diff
- if !ok { return nil, fmt.Errorf("no such tool: %s", call.Name) }
+ if !ok { return ToolResult{Content: "no such tool: " + call.Name, IsError: true} }
```

**4. Trusting the arguments** (`invalid-arguments`)

```diff
- json.Unmarshal([]byte(call.Arguments), &args)
+ if !json.Valid([]byte(call.Arguments)) { return errorResult }
```

## What to compare against your own

- [ ] the registry keeps declaration order
- [ ] unknown name, bad JSON and tool errors are all results
- [ ] pieces are grouped by index
- [ ] the id is set only when present
- [ ] calls are emitted in index order
- [ ] tool messages carry `tool_call_id`

Everything outside that list is style.
