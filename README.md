# agentcraft

Build a coding-agent harness in Go, one chapter at a time, where every chapter
tells you exactly what is wrong.

This is not a tutorial you read. It is a **graded build**: you edit `harness/`,
run `craft test 01`, and get a report that names the mechanism you got wrong —
the way CodeCrafters does, but for something CodeCrafters cannot grade, because a
model's output is random, slow, and paid for.

## The one idea

An agent is not testable. A harness is.

So the course is built around a seam you implement in chapter 01:

```go
type Provider interface {
    Chat(ctx context.Context, req Request) (Message, Usage, error)
}
```

Everything above the seam — loop, tools, sessions, context budget, skills,
subagents — is deterministic. `internal/fake` is a scripted
OpenAI-compatible server that plays the model, so every check is offline, instant,
free, and byte-precise.

That leaves exactly one thing that is genuinely hard to fake: a real model. So
that is the one thing no chapter requires.

## Run it

```bash
go build -o craft ./cmd/craft     # or: go run ./cmd/craft ...

./craft list              # chapters + status
./craft test 01           # grade chapter 01
./craft hint 01 usage     # three-tier hint ladder for one case
./craft test 01 --solution   # grade the reference implementation
./craft progress          # the whole arc
```

Requires Go. No API key, no network, no Docker, no account.

## The arc

Ten chapters. Each one is a *falsifiable* claim about the harness you built.

| # | Chapter | The claim you can prove |
|---|---|---|
| 01 | Talk to the model | request shape + SSE reassembly, byte-exact |
| 02 | Tool calling | streamed `tool_call` fragments reassembled by index |
| 03 | Tools that touch disk | `read`/`write`/`bash` honour schema, errors, truncation |
| 04 | The agent loop | event log matches a grammar; no orphan `tool_result` |
| 05 | Prompt assembly | `AGENTS.md` discovery, injection, attribution |
| 06 | Sessions | JSONL append, replay, resume, idempotence |
| 07 | Context budget | never exceeds budget; tool pairs never split |
| 08 | Interrupt & steering | abort mid-stream; queued message enters at the seam |
| 09 | Skills | frontmatter discovery, lazy body load, args, stacking |
| 10 | Subagent | child loop, isolated context, result returned |

## Deliberately out of scope

Feature parity with [pi](https://github.com/earendil-works/pi) is not the goal and
never was. The course teaches the *machine*, not every attachment. Skipped on
purpose, with pointers instead of chapters:

TUI · MCP · extension/plugin runtime · LSP · multi-provider fan-out beyond two
wire formats · OAuth · images · RPC/SDK · packaging · telemetry.
([crush](https://github.com/charmbracelet/crush) is the Go reference for most of
those; [PiG](https://github.com/MichaelKinsy/PiG) is pi ported to Go line by
line.)

## Layout

```
harness/            ← the only place you edit
internal/fake/      scripted OpenAI-compatible server (the seam's far side)
internal/grader/    assertion engine: expected/actual, diff, Why, hints
internal/hints/     the hint ladder for every case
chapters/NN-*/      README.md (lesson) + checks/ (what "done" means)
solutions/          reference implementations, cumulative, separate Go module
cmd/craft/          the runner
```

## How grading works

Each chapter's `checks/` is an ordinary Go test that imports your `harness`
package through the seam. Cases do not stop each other, so one run gives you the
whole picture. A failed case prints four things:

```
  FAIL 04 surfaces a non-2xx response as *harness.HTTPError
        why: A 429 is not a parse failure. Wrapping it in a typed error is
             what lets the loop later retry with backoff.
        field: error type
        expected: "*harness.HTTPError"
        actual:   provider: HTTP 429: {"error":"slow down"}
        hint: Check resp.StatusCode before you touch the body.
        more: craft hint 01 http-error
```

`why` is the mechanism, `expected`/`actual` is the evidence, `hint` is rung one
of three. The solution never gates your progress; `--solution` runs in a
throwaway copy of the repo and is not recorded.

## Adding a chapter

1. `chapters/NN-slug/README.md` — the lesson, ending in a definition of done.
2. `chapters/NN-slug/checks/chNN_test.go` — cases in the `grader.Suite` DSL.
3. `internal/hints/hints.go` — three rungs per case.
4. `solutions/chNN/` — only the files this chapter changes.
5. `craft test NN` fails; `craft test NN --solution` passes. That pair is the
   contract.
