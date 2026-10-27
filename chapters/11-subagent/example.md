# 10 Example

*Full, annotated implementation. Read it after `kata submit 10` passes.*

The compilable files are `reference/ch10/agent/subagent.go` and
`reference/ch10/agent/cmd/agent/main.go`.

---

## The shape

```
subagent.go  TaskTool: fresh history, child loop, one answer
main.go      build child registry first, then the task tool, then the parent registry
```

## Step 1: a tool whose body is an agent

```go
type TaskTool struct {
	Provider StreamingProvider
	Tools    *Registry
	System   string
	MaxSteps int
}
```

It implements the same `Tool` interface as `read`. That is why nothing else had to
change: the registry already dispatches by name, the loop already appends results,
and the parent already sees one message per call.

A subagent is not a new mechanism. It is a tool that happens to run a loop.

## Step 2: the child's history is two messages

```go
history := []Message{
	{Role: RoleSystem, Content: t.System},
	{Role: RoleUser, Content: args.Prompt},
}
```

This is the whole of "isolated context". No parent messages, no summary, no prefix
transcript. Two messages.

It also explains the tool's description, which says the child cannot see the
conversation. Without that sentence, models write task prompts like "fix the bug you
were just looking at", which is unanswerable in a fresh context.

## Step 3: recursion is impossible, not discouraged

```go
baseTools := []agent.Tool{&agent.ReadTool{...}, &agent.WriteTool{...}, &agent.BashTool{...}}
childTools := agent.NewRegistry(baseTools...)
task := &agent.TaskTool{Provider: provider, Tools: childTools, System: system}
tools := agent.NewRegistry(append(baseTools, task)...)
```

The child registry is created before `task` exists, so it cannot contain it. The
parent registry is the same set plus `task`.

Compare with the runtime-guard version:

| | registry omission | depth counter |
|---|---|---|
| can regress silently | no | yes, by one line |
| error message | "no such tool: task" | "max depth exceeded" |
| test needed | the absence is the test | the counter's arithmetic is the test |

`append(baseTools, task)` mutates `baseTools` if it has spare capacity. In this
program `baseTools` is a fresh literal, so nothing else observes the change. In
general, `append` to a slice you did not just create is a trap; build the second
registry from an explicit union or a fresh slice if that ever stops being true.

## Step 4: one answer, or one error

```go
res, err := RunLoop(ctx, t.Provider, t.Tools, history, LoopOptions{MaxSteps: steps})
if err != nil {
	partial := strings.TrimSpace(res.Message.Content)
	if partial == "" {
		return "", fmt.Errorf("subagent failed: %w", err)
	}
	return "", fmt.Errorf("subagent failed after %d turns: %w (partial answer: %s)",
		res.Turns, err, truncate(partial, 500))
}

answer := strings.TrimSpace(res.Message.Content)
if answer == "" {
	return "", errors.New("subagent returned no answer")
}
return answer, nil
```

Every path returns exactly once. The two ways to get two results for one call are:

- returning `(text, err)` together, and
- writing the result yourself *and* returning an error so the registry writes another.

Neither happens here because the function returns a string or an error, never both.

The partial answer inside the error is deliberate. When a child stops at its step
cap, the parent's model reads "step limit reached ... partial answer: I found three
of the five TODOs" and can decide what to do next. Without it, the parent knows only
that something failed.

## Step 5: the step cap is the child's, not the parent's

```go
steps := t.MaxSteps
if steps <= 0 {
	steps = DefaultSubagentSteps
}
```

The child does not inherit the parent's `-max-steps`. A parent with a generous budget
should not be able to hand that budget to five children; the cap is per run.

The parent is blocked while the child works, so an optimistic child cap is also a
latency bug.

## Step 6: the build order in main

```
1. root
2. skills + catalog
3. system message        ← the child needs it
4. provider (budget)     ← the child needs it
5. base tools
6. child registry        ← no task
7. task tool             ← needs 3, 4, 6
8. parent registry       ← base + task
```

That order is the chapter's structural content. Every dependency points backwards,
which is why the tools could not be built first as in earlier chapters.

## The diffs

**1. Passing the parent's history to the child** (`child-gets-fresh-context`)

```diff
- history := append(parentHistory, Message{Role: RoleUser, Content: args.Prompt})
+ history := []Message{{Role: RoleSystem, Content: t.System}, {Role: RoleUser, Content: args.Prompt}}
```

**2. Adding the task tool to the child registry** (`child-cannot-recurse`)

```diff
- tools := NewRegistry(append(base, task)...); task.Tools = tools
+ task.Tools = NewRegistry(base...)
```

**3. Swallowing a child failure** (`child-failure-is-a-result`)

```diff
- if err != nil { return "done", nil }
+ if err != nil { return "", fmt.Errorf("subagent failed: %w", err) }
```

**4. Returning both a result and an error** (`exactly-one-result`)

```diff
- return partial, err
+ return "", fmt.Errorf("subagent failed: %w (partial answer: %s)", err, partial)
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch10/agent/subagent.go` | 130 | the tool, the child loop, the single return |
| `reference/ch10/agent/cmd/agent/main.go` | 190 | the dependency order |

## Comparison checklist

- [ ] `task` advertised to the parent, not to the child
- [ ] child history is exactly system plus task prompt
- [ ] child registry built before the task tool exists
- [ ] child runs the real loop with the same provider
- [ ] child step cap is separate from the parent's
- [ ] exactly one return on every path
- [ ] child failure becomes an error result with the partial answer
- [ ] empty child answer becomes an error
- [ ] parent continues after the result

Differences outside this list are style.
