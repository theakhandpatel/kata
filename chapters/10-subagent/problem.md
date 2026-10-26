# 10 — Problem

> **Your task:** add a `task` tool that runs a child agent from the task prompt
> alone, with its own tool set and step cap, and returns exactly one result.

**You edit:** `agent/subagent.go`, `agent/cmd/agent/main.go`
**Given:** `agent/loop.go`, `agent/tool.go`, `agent/prompt.go`
**Time:** 90 to 120 minutes

---

## The contract

```go
// subagent.go
const DefaultSubagentSteps = 8

type TaskTool struct {
    Provider StreamingProvider
    Tools    *Registry   // child tool set; must not contain a TaskTool
    System   string      // system message the child starts from
    MaxSteps int         // 0 means DefaultSubagentSteps
}

func (t *TaskTool) Name() string        // "task"
func (t *TaskTool) Execute(ctx context.Context, input json.RawMessage) (string, error)
```

Arguments: `{"description": string, "prompt": string}` with `prompt` required.

Construction order matters:

```go
base     := []agent.Tool{read, write, bash}
child    := agent.NewRegistry(base...)                    // no task tool
task     := &agent.TaskTool{Provider: provider, Tools: child, System: system}
parent   := agent.NewRegistry(append(base, task)...)      // task, once
```

## What the verifier does

It builds the binary and scripts a conversation that crosses both runs: a parent
turn that delegates, the child's turns, then the parent's next turn. Then it reads
every recorded request and distinguishes parent from child by shape.

## The verifier will assert that

- `task-tool-advertised` — the parent's request advertises `task`.
- `child-gets-fresh-context` — the child's request has exactly two messages, a
  system message and the task prompt, and contains none of the parent's text.
- `child-cannot-recurse` — the child's request does not advertise `task`, and it is
  a genuine child request (two messages).
- `result-returned-to-parent` — the parent's next request carries the child's answer
  as the tool result for the call id.
- `exactly-one-result` — that call has exactly one result, and the result is not an
  "unknown tool" error.
- `child-uses-tools` — the child can read a file, and its final answer reaches the
  parent.
- `child-failure-is-a-result` — a child that exhausts its step cap produces an error
  result naming the cap, and the parent still exits 0.
- `parent-continues-after-task` — two delegations in one run both complete and the
  parent prints its own answer after five requests.

## Notes

- **The child's registry defines what it can do.** Leave `task` out and recursion is
  impossible; there is no counter to get wrong.
- **Fresh means two messages.** System plus task prompt. Inheriting history defeats
  the purpose and doubles the cost.
- **Return the answer as a string from `Execute`.** `Registry.Run` turns it into the
  tool result, and turns an error into an `IsError` result.
- **Never return a success and an error.** Two results for one call is invalid.
- **Include the partial answer in a child failure message.** A half-finished child
  is still informative.
- **Deliberate omission:** no parallel children, no background delivery, no per-child
  model.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

A subagent is the same loop started with a fresh history and a smaller tool set. Its
final answer becomes the result of the call that asked for it.
</details>

<details><summary>Rung 2 — the APIs</summary>

`RunLoop` with a two-message history, `NewRegistry(base...)` before the task tool
exists, `append(base, task)` for the parent, `fmt.Errorf("%w")` to keep the child's
error inspectable.
</details>

<details><summary>Rung 3 — what the code does</summary>

`TaskTool.Execute` parses `prompt`, builds `[{system}, {user prompt}]`, calls
`RunLoop` with `MaxSteps`, and returns the final content. On error it returns an
error mentioning the step count and the partial answer. In `main`, build the base
tools, the child registry, the task tool with the provider and system message, then
the parent registry as base plus task. Build the system message before the task tool,
because the child needs it.

`kata hint 10 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 10` reports 8/8, and:

- asking the agent to use a subagent makes extra requests that never appear in the
  parent's history;
- a child that cannot finish still resolves the parent's call;
- you can explain why the recursion limit is structural rather than dynamic.
