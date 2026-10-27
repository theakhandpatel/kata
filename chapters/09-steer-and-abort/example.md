# 09 Example

*Full, annotated implementation. Read it after `kata submit 09` passes.*

The compilable files are `reference/ch09/agent/`. This is the first chapter that
edits the loop, and the reasoning for that is below.

---

## The shape

```
steering.go   StdinSteering: a goroutine, a lock, a drain
loop.go       a steering hook, the new done condition, the partial on error
provider      the parser returns the partial message with its error
main.go       run() returns an exit code; abort prints and exits 130
```

## Step 1: the parser keeps what it read

```go
if err := scanner.Err(); err != nil {
	return Message{Role: RoleAssistant, Content: content.String()}, usage,
		fmt.Errorf("read stream: %w", err)
}
```

Before this chapter the same line returned `Message{}`. That single change is what
makes an interruption recoverable: the caller now has both the text and the reason
it stopped.

Note that the error still wraps the original. `errors.Is(err, context.Canceled)`
has to keep working, because that is how `main` decides between 130 and 1.

## Step 2: the loop keeps it too

```go
if err != nil {
	if msg.Content != "" {
		history = append(history, msg)
	}
	reason := "error"
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		reason = "aborted"
	}
	emit(Event{Type: EventAgentEnd, Turns: turn, Reason: reason})
	return Result{Message: msg, History: history, Usage: total, Turns: turn}, err
}
```

Three things happen before the return, and each is graded:

- the partial message enters history, so the session can record it;
- `agent_end` is still emitted, so a consumer's stream terminates properly;
- the reason distinguishes a cancellation from a real error.

A consumer that only sees `agent_end` without a reason cannot tell "the user
stopped it" from "the network died", and those want different messages on screen.

## Step 3: steering is drained after the turn, not before

```go
steered := false
if opts.Steering != nil {
	for _, m := range opts.Steering() {
		emit(Event{Type: EventSteer, Turn: turn, Text: m.Content})
		history = append(history, m)
		steered = true
	}
}

emit(Event{Type: EventTurnEnd, Turn: turn})

if !askedForTools && !steered {
	emit(Event{Type: EventAgentEnd, Turns: turn, Reason: "done"})
	return ...
}
```

The placement is the design. Drain before the reply and the message is part of the
prompt being answered. Drain after the tools and before the next request, and it
arrives exactly where a person would have spoken.

`steered` exists because of one case: the model finished, and the person then said
something. Without the flag the loop returns a final answer and silently discards
the message.

## Step 4: Drain takes, clears, and returns immediately

```go
func (s *StdinSteering) Drain() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil
	}
	out := s.queue
	s.queue = nil
	return out
}
```

`out := s.queue; s.queue = nil` is the one-liner that prevents replay. Assigning
`out := s.queue[:]` and leaving the queue intact looks identical in a single-turn
test and repeats the instruction forever in a real run.

The reading goroutine:

```go
go func() {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}
		s.mu.Lock()
		s.queue = append(s.queue, Message{Role: RoleUser, Content: line})
		s.mu.Unlock()
	}
}()
```

Empty lines are skipped rather than queued, because a queued empty message makes
the model answer nothing and wastes a turn.

## Step 5: `run` returns an exit code

```go
func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
```

`os.Exit` inside `run` would skip the deferred `rec.Close()` and could drop the last
session write. Returning a code keeps the cleanup in `run`, where the defers live.

```go
if errors.Is(err, context.Canceled) {
	if res.Message.Content != "" {
		fmt.Println(res.Message.Content)
	}
	fmt.Fprintln(os.Stderr, "agent: aborted")
	return 130, nil
}
```

130 is 128 + SIGINT. It is conventional, and it means a shell script can tell an
interrupted run from a failed one without parsing stderr.

Note that persistence happens before this branch. The order is: run, persist,
report. Reversed, an interrupted run loses its turn.

## Why the loop is edited here

The context budget chapter could wrap the provider, because a budget only rewrites a request on its
way out. Steering does something a wrapper cannot: it changes the history the loop
owns, and it changes when the loop stops.

```
  decorator (ch07):  request in, request out
  loop hook (ch08):  history in, history plus a message out
```

The rule is "touch the loop only for loop state". This is loop state, and it is the
only chapter where the rule is exercised.

## The diffs

**1. Returning an empty message on stream failure** (`abort-keeps-partial`)

```diff
- return Message{}, Usage{}, fmt.Errorf("read stream: %w", err)
+ return Message{Role: RoleAssistant, Content: content.String()}, usage, ...
```

**2. Merging steering into the prompt** (`steer-enters-after-turn`)

```diff
- prompt = prompt + "\n" + strings.Join(lines, "\n")
+ history = append(history, Message{Role: RoleUser, Content: line})
```

**3. Draining without clearing** (`steer-not-duplicated`)

```diff
- out := s.queue
+ out := s.queue; s.queue = nil
```

**4. Stopping on the model's word alone** (`steer-continues-after-done`)

```diff
- if len(msg.ToolCalls) == 0 { return }
+ if len(msg.ToolCalls) == 0 && !steered { return }
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch09/agent/loop.go` | 165 | the new done condition, the abort path |
| `reference/ch09/agent/steering.go` | 80 | the goroutine, the lock, the clear |
| `reference/ch09/agent/provider_openai.go` | 300 | partial message beside the error |
| `reference/ch09/agent/cmd/agent/main.go` | 175 | exit codes and persist-before-report |

## Comparison checklist

- [ ] parser returns the partial message with the stream error
- [ ] error still wraps the original, so `errors.Is` works
- [ ] loop appends the partial when non-empty
- [ ] `agent_end` emitted on abort with reason `aborted`
- [ ] steering drained after tools, before the next request
- [ ] drain clears the queue under a lock and never blocks
- [ ] empty lines skipped
- [ ] done condition is `no tool calls && nothing steered`
- [ ] session persisted before the abort return
- [ ] exit 130 for SIGINT, 1 for errors

Differences outside this list are style.
