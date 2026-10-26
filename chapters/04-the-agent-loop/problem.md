# 04 — Problem

> **Your task:** replace the single hop with a loop that runs until the model has
> no more tools, cap the number of turns, and report what happened as events.

**You edit:** `agent/loop.go`, `agent/events.go`,
`agent/provider_openai.go`, `agent/cmd/agent/main.go`
**Given:** `agent/tools.go`, `agent/tool.go`, `agent/types.go`
**Time:** 60 to 90 minutes

---

## The contract

```go
// loop.go
type StreamingProvider interface {
    Provider
    Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error)
}

type LoopOptions struct {
    MaxSteps int      // 0 means 20
    Emit     Emitter  // nil discards
}

type Result struct {
    Message Message   // the final assistant message
    History []Message // including every tool call and result
    Usage   Usage     // summed across requests
    Turns   int
}

func RunLoop(ctx context.Context, p StreamingProvider, tools *Registry,
    history []Message, opts LoopOptions) (Result, error)
```

```go
// events.go
type Event struct { Type string; Turn int; ID, Name, Text string; IsError bool; Bytes, Turns int; Reason string }
type Emitter func(Event)
func JSONEmitter(w io.Writer) Emitter
```

The program gains two flags:

```sh
agent -p "..." -max-steps 3        # stop after 3 model requests
agent -p "..." -events             # JSON events on stderr
```

`Provider` keeps its single `Chat` method. `Stream` is declared on a wider
interface so that chapters 01 to 03 still compile: a provider that cannot stream
is still a provider.

## What the verifier does

It builds the binary, scripts a conversation, runs the program, and inspects:

1. **the request trace** — how many turns, and whether each turn carried the
   previous results in order;
2. **stderr** — the JSON events, parsed as a sequence;
3. **stdout and the exit code**.

## The verifier will assert that

- `loops-until-done` — a task needing two tools makes three requests and prints
  the final answer.
- `preserves-result-order` — two calls in one message return two results in the
  same order, with matching ids.
- `step-cap-stops` — with `-max-steps 3` and a model that always asks for a tool,
  the run makes exactly three requests, exits 1, and names the step limit.
- `events-sequence` — every `tool_call` id has exactly one `tool_result` with the
  same id, and `agent_end` appears once, last.
- `events-text-deltas` — the `text` events concatenate to the final answer, in at
  least two deltas.
- `single-turn-no-tools` — a plain reply produces one `turn_start`, one
  `turn_end`, one request.
- `continues-after-tool-error` — a failed tool does not end the run; a later turn
  succeeds, and the events mark the failed result.
- `turn-count-matches-requests` — `agent_end.turns` equals the number of requests
  the server recorded.

## Notes

- **Announce before executing.** Emit every `tool_call` in the batch, then run
  them. A reader that sees only results cannot tell a slow tool from a missing
  one.
- **`turn_end` closes a turn, `agent_end` closes the run.** On the step limit,
  still emit `agent_end` with reason `step_limit`, then return an error.
- **The cap is a ceiling, not a schedule.** Exit on the model's reply; use the cap
  only when the reply never stops asking.
- **`Chat` becomes `Stream(ctx, req, nil)`.** Do not keep two parsers.
- **Deliberate omission:** tools still run one at a time. Concurrency arrives in
  chapter 10, where a subagent makes it necessary.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

The reply is the exit condition. The cap is a safety net around it. The events are
the same facts, written down in order.
</details>

<details><summary>Rung 2 — the APIs</summary>

`json.NewEncoder(w).Encode` per event. `errors.Join` or a sentinel for the step
limit. A function parameter for the delta callback.
</details>

<details><summary>Rung 3 — what the code does</summary>

Loop from turn 1 to max. Emit `turn_start`, call `provider.Stream` with a callback
that emits `text`, append the message. If there are no tool calls, emit
`turn_end` and `agent_end` with reason `done` and return. Otherwise emit every
`tool_call`, run each call, emit `tool_result` with the same id, append the result
messages in order, emit `turn_end`. After the loop, emit `agent_end` with reason
`step_limit` and return an error wrapping a sentinel.

`kata hint 04 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 04` reports 8/8, and:

- the agent completes a task that needs three tool calls;
- you can explain why the exit condition and the cap are different mechanisms;
- you can read an event stream and tell what the program did, in order.
