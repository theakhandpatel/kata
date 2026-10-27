# 08 Example

*Full, annotated implementation. Read it after `kata submit 08` passes.*

The compilable files are `reference/ch08/agent/budget.go`,
`reference/ch08/agent/events.go` and `reference/ch08/agent/cmd/agent/main.go`.
The loop and the provider are unchanged.

---

## The shape

```
budget.go   EstimateTokens, Fit, WithBudget, budgetedProvider
events.go   three new fields and the budget event type
main.go     -max-context, and one wrapper around the provider
```

## Step 1: estimate everything that is sent

```go
func EstimateTokens(msgs []Message, tools []ToolDef) int {
	bytes := 0
	for _, m := range msgs {
		bytes += len(m.Role) + len(m.Content) + 4
		for _, tc := range m.ToolCalls {
			bytes += len(tc.ID) + len(tc.Name) + len(tc.Arguments)
		}
	}
	for _, t := range tools {
		bytes += len(t.Name) + len(t.Description)
		if schema, err := json.Marshal(t.Parameters); err == nil {
			bytes += len(schema)
		}
	}
	return bytes / 4
}
```

Three decisions:

- **Tool calls inside messages count.** An assistant message carrying a 3 KB
  argument string is not free, and neither is the tool result that follows it.
- **A fixed +4 per message.** Every message carries a role and separators. Without
  it, a hundred empty messages estimate to zero.
- **One division at the end.** Dividing per message rounds each cost down and
  under-counts long conversations badly.

## Step 2: fit from the newest end

```go
kept := make([]Message, 0, len(rest))
for i := len(rest) - 1; i >= 0; i-- {
	candidate := append([]Message{rest[i]}, kept...)
	trial := append(append([]Message{}, system...), candidate...)
	if len(kept) > 0 && EstimateTokens(trial, tools) > budget {
		break
	}
	kept = candidate
}
```

`len(kept) > 0` is the guard that keeps at least one message. Without it, a single
message bigger than the budget produces an empty conversation, and the model gets a
system message and nothing to do.

Recomputing the estimate per candidate is O(n²) in the worst case. For a
conversation of a few hundred messages that is microseconds, and it is much easier
to get right than incremental accounting. Say so in a comment if you rewrite it.

## Step 3: repair the boundary

```go
for len(kept) > 0 && kept[0].Role == RoleTool {
	kept = kept[1:]
	dropped++
}
```

Walking backwards means the cut can land between an assistant message and the tool
results that answer it. A tool result whose call is missing makes the request
invalid: providers reject it with a message about orphaned tool calls.

The cheapest correct fix is to drop the orphan. The alternative, pulling its call
back in, can exceed the budget again.

## Step 4: one marker, not one per drop

```go
if dropped > 0 {
	out = append(out, Message{
		Role:    RoleSystem,
		Content: fmt.Sprintf("[context pruned: %d earlier messages omitted to fit %d tokens]", dropped, budget),
	})
}
```

A system-role message is used because inserting a second user message changes the
shape of the conversation, and a user message in the middle of history is a lie
about who said what.

The number is included so a person reading `-events` output can see the difference
between "one message was enormous" and "thirty messages were trimmed".

## Step 5: the decorator

```go
func (b *budgetedProvider) Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error) {
	b.calls++

	fitted, pruned := Fit(req.Messages, req.Tools, b.budget)
	b.emit(Event{
		Type:   EventBudget,
		Turn:   b.calls,
		Tokens: EstimateTokens(fitted, req.Tools),
		Budget: b.budget,
		Pruned: pruned,
	})

	req.Messages = fitted
	return b.inner.Stream(ctx, req, onText)
}
```

`req` is passed by value, so reassigning `req.Messages` affects only this call.
That matters: the loop keeps its own history and must not be silently truncated by
a request that mutated it.

`budgetedProvider` implements both `Chat` and `Stream`, so it satisfies
`StreamingProvider`. `Chat` delegates to `Stream`, which means the budget applies
to whichever entry point the caller uses.

## Step 6: wiring order

```go
tools := agent.NewRegistry(...)

provider := agent.WithBudget(
	agent.NewOpenAICompat(baseURL, key, model),
	tools, *maxContext, emit,
)
```

The decorator needs the tool schemas, so tools are built first. That ordering is
the only structural change in `main`.

`WithBudget` returns the provider unchanged when `budget <= 0`, which makes
`-max-context 0` mean "off" rather than "budget of zero tokens".

## The diffs

**1. Counting only messages** (`estimate-counts-tools`)

```diff
- for _, m := range msgs { bytes += len(m.Content) }
+ for _, t := range tools { bytes += len(t.Name) + len(t.Description) + len(schema) }
```

**2. Threshold instead of ceiling** (`ceiling-respected`)

```diff
- if EstimateTokens(history) > budget { history = Fit(history) }   // once, at start
+ // inside the provider decorator, on every request
```

**3. Dividing per message** (`estimate-grows-with-history`)

```diff
- total += (len(m.Content) + 4) / 4
+ bytes += len(m.Content) + 4; ... ; return bytes / 4
```

**4. Ignoring the orphan** (`no-orphaned-tool-pairs`)

```diff
- return append(system, kept...)
+ for len(kept) > 0 && kept[0].Role == RoleTool { kept = kept[1:]; dropped++ }
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch08/agent/budget.go` | 170 | estimate, fit, the decorator |
| `reference/ch08/agent/events.go` | 75 | the extra fields, and why they are optional |
| `reference/ch08/agent/cmd/agent/main.go` | 165 | wiring order |

## Comparison checklist

- [ ] estimate counts messages, tool calls and tool schemas
- [ ] one division by four, at the end
- [ ] system message kept and excluded from the walk
- [ ] newest messages kept, at least one always
- [ ] leading tool results dropped after the cut
- [ ] one marker message naming the count and the budget
- [ ] budget applied per request through a provider decorator
- [ ] `budget <= 0` disables rather than empties
- [ ] event reports tokens, budget and pruned per request

Differences outside this list are style.
