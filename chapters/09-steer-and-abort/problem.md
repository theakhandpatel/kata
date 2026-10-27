# 08 Problem

> **Your task:** keep the partial answer when a run is interrupted, and let lines
> typed during a run join it at the next turn boundary.

**You edit:** `agent/loop.go`, `agent/steering.go`, `agent/provider_openai.go`,
`agent/events.go`, `agent/cmd/agent/main.go`
**Given:** `agent/budget.go`, `agent/session.go`, `agent/prompt.go`
**Time:** 75 to 100 minutes

---

## The contract

```go
// loop.go
type LoopOptions struct {
    MaxSteps int
    Emit     Emitter
    Steering func() []Message   // drained at every turn boundary
}

// steering.go
func NewStdinSteering(r io.Reader) *StdinSteering
func (s *StdinSteering) Start()
func (s *StdinSteering) Drain() []Message

// events.go
const EventSteer = "steer"
```

One flag:

```sh
printf 'also check the tests\n' | agent -p "add a flag" -steer-stdin
```

Exit codes: `0` success, `1` error, **`130` interrupted**.

## What the verifier does

It runs the real binary. For steering it supplies stdin and inspects every recorded
request. For abort it starts the process, waits 250 ms, sends SIGINT while the
scripted server is still streaming, then reads stdout, the exit code, the events and
the session file.

## The verifier will assert that

- `steer-continues-after-done`: a run whose model answered, then received a
  steered line, makes two requests and prints the second answer.
- `steer-enters-after-turn`: request 1 contains no steering text; request 2 is
  `system, user, assistant, user(steer)`, in that order.
- `steer-not-duplicated`: one steered line appears exactly once across all
  requests.
- `no-steer-single-request`: with empty stdin the run costs one request.
- `steer-event-reported`: exactly one `steer` event, carrying the text.
- `abort-keeps-partial`: exit code 130, and stdout is a non-empty prefix of the
  streamed reply.
- `abort-emits-event`: the event stream ends with `agent_end` whose reason is
  `aborted`.
- `abort-persists-session`: the session file contains the user message and an
  assistant record with the beginning of the interrupted reply.

## Notes

- **The parser returns the partial message with the error.** Not an empty message.
  This is the difference between interruption and data loss.
- **`agent_end` is emitted on every path**, including abort. The reason field is
  what distinguishes outcomes.
- **Drain clears.** Take the slice, set it to nil, release the lock.
- **Drain never blocks.** Reading happens on its own goroutine; the loop only
  inspects what has already arrived.
- **The done condition changes**: no tool calls *and* nothing steered.
- **Persist before returning.** A cancelled run still produced turns, and the
  session must match what the person saw.
- **Deliberate omission:** no interactive terminal, no queue persistence, no
  process-group teardown.

## Hints

<details><summary>Rung 1, the mechanism</summary>

Interruption is a normal outcome with its own exit code and its own event reason.
Steering is a queued message that enters at the end of a turn, never inside the
prompt being answered.
</details>

<details><summary>Rung 2, the APIs</summary>

`context.Canceled`, `errors.Is`, `signal.NotifyContext`, `bufio.Scanner` on a
goroutine, `sync.Mutex`, `os.Exit` avoided by returning an exit code from `run`.
</details>

<details><summary>Rung 3, what the code does</summary>

In the parser, return `Message{Role: RoleAssistant, Content: content.String()}`
beside the stream error. In the loop, on error append that partial message when it
is non-empty, emit `agent_end` with reason `aborted` when the error is a
cancellation, and return. After the tool batch, drain steering: emit a `steer` event
per message and append each as a user message. Stop only when the reply had no tool
calls and nothing was steered. In `main`, persist the session, then on
`context.Canceled` print the partial answer, write `agent: aborted` to stderr, and
return exit code 130.

`kata hint 08 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 08` reports 8/8, and:

- Ctrl-C during a long answer leaves the text you already read on screen;
- a line typed mid-run lands in the next request as its own message;
- you can explain why the done condition gained an `and`.
