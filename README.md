# Kata

Build a coding-agent harness in Go, with a grader that tells you what is broken.

Kata is a course of eleven chapters and one local tool. You write the harness:
the provider client, the tools, the loop, sessions, the context budget,
steering, skills, subagents, and an MCP client. The tool runs your code against a
scripted model and reports which claim failed and why.

```sh
git clone <this repo> && cd kata
go build -o kata ./cmd/kata

./kata                 # where you are, what to do next
./kata read 01         # the task for chapter 01
./kata submit 01       # compile, grade, report
```

No account, no API key, no Docker, no network after the clone.

## Why a grader

A harness sits around a model that is random, slow, and paid for. That makes the
obvious teaching method, "run it and see", useless: the answer changes each time
and a failure costs money.

Kata replaces the model with a scripted HTTP server and keeps the wire. You
speak real Server-Sent Events to a real socket. Only the reply is fixed. So every
check is instant, offline, and byte-precise, and a failure names the mechanism
rather than showing a diff of prose.

```
  FAIL 04 surfaces a non-2xx response as *agent.HTTPError
        why: A 429 is not a parse failure. Wrapping it in a typed error is
             what lets the loop later retry with backoff.
        field: error type
        expected: "*agent.HTTPError"
        actual:   provider: HTTP 429: {"error":"slow down"}
        hint: Check resp.StatusCode before you touch the body.
        more: kata hint 01 http-error
```

## The arc

| # | Chapter | You prove |
|---|---|---|
| 01 | Talk to the model | request shape, SSE reassembly, framing, errors, cancellation |
| 02 | Tool calling | tool-call fragments folded by index, ids omitted on continuations |
| 03 | Tools that touch disk | `read`, `write`, `bash` honour the schema, errors, truncation |
| 04 | The agent loop | event order, one result per call, clean termination |
| 05 | Prompt assembly | `AGENTS.md` discovery and attribution |
| 06 | Sessions | JSONL append, replay, resume, idempotence |
| 07 | Context budget | a hard ceiling, no orphaned pairs, marked pruning, recall |
| 08 | Steer and abort | abort mid-stream, a queued message entering at the seam |
| 09 | Skills | frontmatter, lazy body load, arguments, stacking |
| 10 | Subagent | fresh context, scoped tools, exactly one result |
| 11 | MCP (advanced) | stdio JSON-RPC, initialize, `tools/list`, `tools/call` |

Chapter 11 is optional. Chapters 01 to 04 give you a working agent you can point at
Ollama or any OpenAI-compatible endpoint.

When the arc is done, [chapters/where-next.md](chapters/where-next.md) lists what to
build on top: recovery of pruned context, a schema budget, parallel tool calls, a
second wire format, and the bigger structural changes after those.

## What each chapter contains

| File | Read when | Contains |
|---|---|---|
| `overview.md` | before you start | how the mechanism works, with diagrams |
| `problem.md` | at the keyboard | the task, the graded claims, the hint ladder |
| `example.md` | after an attempt | a full annotated implementation |

`problem.md` states every claim the verifier makes, by case id. If a claim and a
case ever disagree, the case is correct and the page is a bug.

## Commands

```sh
kata                      status: current chapter, next action
kata submit [NN]          compile, grade, record
kata submit NN --solution grade the reference instead of your code
kata read [NN] [doc]      problem (default), overview, example
kata hint NN <case-id>    three rungs, from nudge to answer
kata list                 the arc, grouped into parts
kata progress             what passed, and when
kata verify               prove each chapter rejects its starter and accepts its reference
kata reset [NN]           restore agent/ to the state before a chapter
kata read where-next      what to build after the course
kata site                 render the course as a static website in site/dist
```

`kata site` writes plain HTML and one stylesheet. Open `site/dist/index.html`
directly, or serve the directory. Diagrams load from a CDN; offline, the diagram
source stays readable in place.

## How it works

Every chapter leaves you with a program you can run. Chapter 01 gives you an
application that answers a prompt; chapter 04 gives you one that completes a
multi-step task; chapter 09 one that follows skills you write.

The checks build `./agent/cmd/agent`, start a scripted server, point
`OPENAI_BASE_URL` at it, and run your binary. They assert on stdout, stderr, exit
codes, and the requests your program sent. Nothing imports your package, so the
internal design is yours.

```sh
OPENAI_BASE_URL=http://localhost:11434/v1 ./agent -p "list the files here"
```

Design notes, diagrams, and the grading pipeline are in
[docs/architecture.md](docs/architecture.md). The chapter file contract is in
[docs/chapter-format.md](docs/chapter-format.md).

## Requirements

Go 1.26 or newer. Nothing else. The reference harness uses only the standard
library.

## What Kata is not

Kata teaches the machine, not every attachment. It does not ship a terminal UI,
an extension runtime, an LSP client, provider authentication, image input, or a
cloud runner. Chapter 11 covers MCP because the client is small enough to teach.
For a full harness, read [pi](https://github.com/earendil-works/pi),
[crush](https://github.com/charmbracelet/crush), and
[PiG](https://github.com/MichaelKinsy/PiG).

## Contributing

A chapter is done when `kata verify` is clean: its reference passes every case
and its starter fails at least one. Hints are validated in both directions, so a
case without a ladder fails the test suite. Prose follows
[docs/writing-style.md](docs/writing-style.md).

## License

MIT. See [LICENSE](LICENSE).
