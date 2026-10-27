# 02 Example

*Full, annotated implementation. Read it after `kata submit 02` passes, then
compare.*

The compilable files are `reference/ch02/agent/`. This document takes them apart
in the order the thinking happens.

---

## The shape

Three new pieces and one change:

```
agent/tool.go     Registry: name -> Tool, and Tool -> ToolDef
agent/tools.go    ReadTool: resolve, check, read, truncate
agent/provider_openai.go   learn tool_calls on the way out and in
agent/cmd/agent/main.go    one hop: call, run tools, call again
```

The registry is the only place that knows the set of tools. The provider knows
the wire format. `main` knows the sequence. Keeping those three apart is what
makes chapter 03 a two-file change.

## Step 1: the registry is a lookup, not a switch

```go
type Registry struct {
	byName map[string]Tool
	order  []string // declaration order
}

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

`order` is not decoration. The schema list sits at the start of every request, and
a stable prefix is what lets a provider cache it. A map iteration order would
change the bytes on every call.

## Step 2: a call can fail in four ways, and all four are results

```go
func (r *Registry) Run(ctx context.Context, call ToolCall) ToolResult {
	t, ok := r.Lookup(call.Name)
	if !ok {
		return ToolResult{Content: "no such tool: " + call.Name, IsError: true}
	}
	if !json.Valid([]byte(call.Arguments)) {
		return ToolResult{Content: "arguments are not valid JSON: " + call.Arguments, IsError: true}
	}
	out, err := t.Execute(ctx, json.RawMessage(call.Arguments))
	if err != nil {
		return ToolResult{Content: t.Name() + ": " + err.Error(), IsError: true}
	}
	return ToolResult{Content: out}
}
```

The `json.Valid` check is not defensive padding. The arguments are text the model
wrote. Without it, `json.Unmarshal` inside the tool fails, the error propagates,
and the program exits on a model mistake. The model never learns what went wrong,
which is the one thing it can fix.

Note what is **not** here: no logging, no retry, no permission prompt. Those are
policy, and chapter 08 is where that policy gets an interface of its own.

## Step 3: the workspace check is on the resolved path

```go
func (t *ReadTool) resolve(p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	full, _ := filepath.Abs(filepath.Join(t.Root, p))
	rel, _ := filepath.Rel(t.Root, full)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace: %s", p)
	}
	return full, nil
}
```

Checking the string for `..` is not enough: `a/../../etc/passwd` contains `..` but
so do legitimate names, and a symlink defeats a string check entirely. Resolve
first, then ask whether the result is still inside the root.

## Step 4: truncation must be visible

```go
buf := make([]byte, limit+1)
n, err := f.Read(buf)
if err != nil && n == 0 {
	return "", err
}
if n > limit {
	return string(buf[:limit]) + fmt.Sprintf(
		"\n... [truncated: %d of %d bytes shown]", limit, info.Size()), nil
}
```

Reading `limit+1` bytes is how you learn there was more. Two consequences:

- the model is told it did not see everything, so it can ask for a specific
  range instead of guessing;
- the result has a hard ceiling, so one large file cannot consume the context
  budget that chapter 07 will enforce.

## Step 5: the one hop, stated plainly

```go
msg, usage, err := provider.Chat(ctx, agent.Request{Messages: history, Tools: tools.Defs()})
history = append(history, msg)

if len(msg.ToolCalls) > 0 {
	history = append(history, runTools(ctx, tools, msg.ToolCalls)...)
	msg, usage, err = provider.Chat(ctx, agent.Request{Messages: history, Tools: tools.Defs()})
	history = append(history, msg)
}

fmt.Println(msg.Content)
```

`runTools` returns one message per call, in order:

```go
out = append(out, agent.Message{
	Role:       agent.RoleTool,
	Content:    res.Content,
	ToolCallID: call.ID,
	Name:       call.Name,
})
```

`ToolCallID` is the field that makes the conversation valid. Lose it and the next
request is rejected with a message about orphaned tool calls.

The hop is visible on purpose. Chapter 04 turns the `if` into a `for` and adds a
step cap.

## Step 6: grouping by index

```go
acc := calls[tc.Index]
if acc == nil {
	acc = &callAccumulator{}
	calls[tc.Index] = acc
}
if tc.ID != "" { acc.id = tc.ID }        // only the first fragment has it
if tc.Function.Name != "" { acc.name = tc.Function.Name }
acc.args.WriteString(tc.Function.Arguments)  // always append
```

Three rules, each with a failing case:

| Rule | Wrong version | Case |
|---|---|---|
| key by index | key by id | `folds-arguments` |
| set id only when present | overwrite with the empty id | `folds-arguments` |
| append arguments | assign the last fragment | `folds-arguments` |

Then emit in index order, because the assistant message's tool call order is what
the results must match.

## The diffs

**1. Group by id instead of index** (`folds-arguments`)

```diff
- acc := calls[tc.ID]           // "" on continuation fragments
+ acc := calls[tc.Index]
```

**2. Return the error instead of a result** (`missing-file`)

```diff
- if err != nil { return "", err }        // becomes a fatal error upstream
+ return ToolResult{Content: "no such file: " + args.Path, IsError: true}
```

**3. String check for path escape** (`refuses-escape`)

```diff
- if strings.Contains(p, "..") { reject }
+ full, _ := filepath.Abs(filepath.Join(root, p))
+ rel, _ := filepath.Rel(root, full)
+ if rel == ".." || strings.HasPrefix(rel, "../") { reject }
```

**4. Unbounded read** (`truncates-large-file`)

```diff
- data, _ := os.ReadFile(full)
+ buf := make([]byte, limit+1)
+ n, _ := f.Read(buf)
+ if n > limit { return string(buf[:limit]) + marker }
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch02/agent/tool.go` | 95 | registry, result conversion |
| `reference/ch02/agent/tools.go` | 140 | path resolution, truncation |
| `reference/ch02/agent/provider_openai.go` | 284 | tool_calls in both directions |
| `reference/ch02/agent/cmd/agent/main.go` | 150 | the hop |

## Comparison checklist

- [ ] registry preserves declaration order
- [ ] unknown tool, bad JSON and tool errors all become `IsError` results
- [ ] path check on the resolved path, absolute paths refused
- [ ] read is bounded, and the cut is marked
- [ ] tool results carry `tool_call_id` and keep call order
- [ ] arguments grouped by index, appended not assigned
- [ ] one request when there are no tool calls

Differences outside this list are style.
