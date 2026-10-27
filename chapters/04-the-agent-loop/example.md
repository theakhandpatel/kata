# 04 Example

*Full, annotated implementation. Read it after `kata submit 04` passes.*

The compilable files are `reference/ch04/agent/`. This document takes them apart
in the order the thinking happens.

---

## The shape

```
events.go    Event, the six type constants, Emitter, JSONEmitter
loop.go      RunLoop: the state machine, the cap, the accounting
provider     Chat becomes Stream(ctx, req, nil); readSSE gains a callback
main.go      -max-steps and -events, one call to RunLoop
```

`RunLoop` is the only function that knows the order. Everything it calls knows
only its own job, which is why chapter 08 can add cancellation without touching
the registry or the parser.

## Step 1: the loop is a state machine with one exit

```go
for turn := 1; turn <= max; turn++ {
	emit(Event{Type: EventTurnStart, Turn: turn})

	msg, usage, err := provider.Stream(ctx, req, func(text string) {
		emit(Event{Type: EventText, Turn: turn, Text: text})
	})
	if err != nil {
		return Result{...}, err
	}
	history = append(history, msg)

	if len(msg.ToolCalls) == 0 {
		emit(Event{Type: EventTurnEnd, Turn: turn})
		emit(Event{Type: EventAgentEnd, Turns: turn, Reason: "done"})
		return Result{Message: msg, History: history, Turns: turn}, nil
	}

	// ... announce, run, append results, turn_end
}
```

The `for` bound and the `if` exit are both present and they mean different things.
The `if` is the program working. The bound is the program refusing to work
forever. Deleting either one breaks a different case.

## Step 2: announce the batch, then run it

```go
for _, call := range msg.ToolCalls {
	emit(Event{Type: EventToolCall, Turn: turn, ID: call.ID, Name: call.Name})
}

for _, call := range msg.ToolCalls {
	res := tools.Run(ctx, call)
	emit(Event{Type: EventToolResult, Turn: turn, ID: call.ID, Name: call.Name,
		IsError: res.IsError, Bytes: len(res.Content)})
	history = append(history, Message{
		Role: RoleTool, Content: res.Content, ToolCallID: call.ID, Name: call.Name,
	})
}
```

Two loops over the same slice, and that is not an accident.

The first publishes intent: this is what we are about to do. The second publishes
outcome. A UI needs the first one, because otherwise a call that takes four
seconds looks like a program that has hung, and a call that crashes leaves no
trace of what it was.

`Bytes` rather than the content in the event is deliberate: the event stream is a
log, not a second copy of the conversation. Chapter 07 uses those byte counts.

## Step 3: the cap is a distinct failure

```go
emit(Event{Type: EventAgentEnd, Turns: max, Reason: "step_limit"})
return Result{History: history, Usage: total, Turns: max},
	fmt.Errorf("%w after %d turns; the model kept asking for tools", ErrStepLimit, max)
```

Three details:

- `agent_end` is still emitted, with a reason that is not `done`. A consumer
  should not have to infer the outcome from an absent event.
- The error wraps a sentinel, so a caller can distinguish "the model gave up" from
  "the network died". `main` prints it and exits 1.
- The result still carries the history, because the caller may want to resume or
  inspect it.

## Step 4: the accounting

```go
total = addUsage(total, usage)
```

`Chat` returned usage per request. A run makes several, and the number a user
cares about is the total. Summing in the loop is the only place that has them all.

## Step 5: the provider grows a seam without breaking anyone

```go
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error) {
	return p.Stream(ctx, req, nil)
}

func (p *OpenAICompat) Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error) {
	// ... same request, same parser, one extra callback
}
```

Inside the parser, one line:

```go
if choice.Delta.Content != "" && onText != nil {
	onText(choice.Delta.Content)
}
content.WriteString(choice.Delta.Content)
```

Note what did **not** change: the wire format, the error handling, the truncation
guard. The seam opened and the rest stayed still. That is the difference between a
refactor and a rewrite, and it is why `Provider` was left alone and a wider
interface was declared beside it:

```go
type StreamingProvider interface {
	Provider
	Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error)
}
```

Chapters 01 to 03 satisfy `Provider` and keep compiling.

## Step 6: events as JSON lines

```go
func JSONEmitter(w io.Writer) Emitter {
	enc := json.NewEncoder(w)
	return func(e Event) { _ = enc.Encode(e) }
}
```

`Encoder.Encode` writes a newline and flushes through the writer, which is what
makes a long run followable with `tail -f`. One object per line also means a
reader can ignore lines it does not understand, which is how the checks tolerate a
program that also logs to stderr.

## The diffs

**1. Continuing after the last tool instead of on the reply** (`single-turn-no-tools`)

```diff
- for i := 0; i < max; i++ { doTurn() }        // exactly N turns
+ if len(msg.ToolCalls) == 0 { finish() }      // until the model is done
```

**2. Collecting results out of order** (`preserves-result-order`)

```diff
- for _, call := range calls { go func(){ results = append(results, run(call)) }() }
+ for _, call := range calls { results = append(results, run(call)) }
```

**3. Ending without saying why** (`step-cap-stops`)

```diff
- return Result{}, errors.New("too many turns")
+ emit(Event{Type: EventAgentEnd, Turns: max, Reason: "step_limit"})
+ return Result{...}, fmt.Errorf("%w after %d turns", ErrStepLimit, max)
```

**4. Two parsers** (`events-text-deltas`)

```diff
- func readSSE(r io.Reader) (Message, Usage, error)
- func readSSEStreaming(r io.Reader, cb func(string)) (Message, Usage, error)
+ func readSSE(r io.Reader, onText func(string)) (Message, Usage, error)
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch04/agent/loop.go` | 130 | state machine, cap, accounting |
| `reference/ch04/agent/events.go` | 60 | the event shape and the grammar |
| `reference/ch04/agent/provider_openai.go` | 290 | the seam opening |
| `reference/ch04/agent/cmd/agent/main.go` | 100 | wiring flags to options |

## Comparison checklist

- [ ] exit condition is the model's reply, not the counter
- [ ] cap is positive, configurable, and reports a reason
- [ ] `agent_end` emitted once, last, on both success and cap
- [ ] results appended in source order with matching ids
- [ ] `turn_end` per turn, `agent_end` per run
- [ ] text deltas emitted from the parser, not reconstructed afterwards
- [ ] `Chat` delegates to `Stream`; one parser
- [ ] usage summed across turns
- [ ] step limit is a distinct, testable error

Differences outside this list are style.
