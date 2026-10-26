# 07 — Problem

> **Your task:** give the agent a token ceiling, estimate what a request costs,
> drop the oldest history when it does not fit, mark the cut, and report the
> accounting as an event.

**You edit:** `agent/budget.go`, `agent/events.go`, `agent/cmd/agent/main.go`
**Given:** `agent/loop.go`, `agent/provider_openai.go`, `agent/session.go`
**Time:** 90 to 120 minutes

---

## The contract

```go
// budget.go
const DefaultMaxContext = 8000

func EstimateTokens(msgs []Message, tools []ToolDef) int
func Fit(msgs []Message, tools []ToolDef, budget int) ([]Message, int)
func WithBudget(p StreamingProvider, tools *Registry, budget int, emit Emitter) StreamingProvider

// events.go
const EventBudget = "budget"
// Event gains: Tokens int, Budget int, Pruned int
```

One flag, one event, one marker:

```sh
agent -p "..." -max-context 2000 -events
```

```json
{"type":"budget","turn":1,"tokens":1840,"budget":2000,"pruned":31}
```

The marker inserted where history was removed:

```
[context pruned: 31 earlier messages omitted to fit 2000 tokens]
```

## What the verifier does

It builds the binary, writes a session file in the chapter 06 format, runs the
program with a small `-max-context`, then reads the **budget events** on stderr and
the **request messages** the server recorded. The estimate is graded through the
event, so nothing reaches into your package.

## The verifier will assert that

- `budget-event-reported` — at least one `budget` event, with a positive `tokens`
  and the configured `budget`.
- `estimate-counts-tools` — a request with one short prompt estimates at 50 tokens
  or more, because three tool schemas are not free.
- `estimate-grows-with-history` — twelve prior messages estimate higher than two.
- `ceiling-respected` — with `-max-context 500`, every budget event reports at most
  500 tokens.
- `prunes-when-over` — a 40-message session at that ceiling reports `pruned > 0`
  and sends far fewer messages than it loaded.
- `prune-marker-present` — the request carries the marker text.
- `no-orphaned-tool-pairs` — after a cut, no `tool` message appears whose
  `toolCallId` was not announced by an earlier assistant message.
- `keeps-system-and-newest` — the system message is first and still contains the
  `AGENTS.md` body, and the last message is the prompt that was just typed.

## Notes

- **Wrap the provider, do not edit the loop.** `WithBudget` returns a
  `StreamingProvider` that fits `req.Messages` before delegating. Every turn goes
  through it, including later ones.
- **Keep the system message out of the walk.** Fit the conversation after index 0,
  then reassemble.
- **Walk backwards.** Keeping the newest is the point; the cut belongs at the old
  end.
- **Always keep one message.** A single message larger than the budget is a fact,
  not something pruning can fix. Sending nothing is worse.
- **Drop orphaned tool results after the cut**, and count them as pruned.
- **Divide by four once, at the end.** Per-message division under-counts.
- **Deliberate omission:** no summarisation and no recall tool. The session file
  keeps everything; the request keeps less.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

Estimate everything a request sends, fit the conversation to the ceiling before
each request, and say what you dropped.
</details>

<details><summary>Rung 2 — the APIs</summary>

`json.Marshal` on a tool's `Parameters` for its size. A backwards `for` over the
message slice. A struct that implements `StreamingProvider` and delegates.
</details>

<details><summary>Rung 3 — what the code does</summary>

`EstimateTokens` sums role, content, tool-call fields and per-message overhead,
adds each tool's name, description and marshalled schema, divides by four.
`Fit` keeps `msgs[0]` when it is a system message, walks the remainder backwards
accumulating messages until the estimate would exceed the budget, drops leading
tool results, then returns the system message, one marker message when anything was
dropped, and the kept tail. `WithBudget` returns a decorator whose `Stream` fits,
emits the budget event, and delegates.

`kata hint 07 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 07` reports 8/8, and:

- a long session still gets an answer under a small ceiling;
- the event numbers agree with what the server received;
- you can explain why a decorator was the right place for this policy.
