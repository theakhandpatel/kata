# Kata

Build a coding-agent harness in Go, with a grader that tells you what is broken.

Read it online at **[kata.akhand.cc](https://kata.akhand.cc)**.

You use a coding agent every day. You type what you want, it reads files, runs
commands, and comes back with an answer. When it works it looks like magic, and when
it does not you are left guessing whether the model was confused or the program
around it was.

This is a course about the program around it.

Over twelve chapters you write one in Go, from an empty stub to something that can
read a file, run a test, remember yesterday's conversation, and hand a whole job to a
second copy of itself. You start by making a single HTTP request to a model. You end
with 2,000 lines you understand completely, and you can point at the line that decides
whether your agent keeps going or stops.

```sh
git clone <this repo> && cd kata
go build -o kata ./cmd/kata

./kata                 # where you are, what to do next
./kata read 01         # the task for chapter 01
./kata submit 01       # compile, grade, report
```

No account, no API key, no Docker, no network after the clone.

## Why you get graded

Here is the problem with learning this from a tutorial. A model is random, slow, and
costs money per call, so "run it and see" teaches you nothing. It worked yesterday. It
failed today. You cannot tell whether you broke something or the sample answer moved.

So the course ships a second program that plays the model. It is a real HTTP server
speaking real Server-Sent Events to a real socket. Only the words in the reply are
fixed. That means every check runs in a second, offline, and fails with a sentence
about the thing you got wrong:

```
  FAIL 04 surfaces a non-2xx response as *agent.HTTPError
        why: A 429 is not a parse failure. Wrapping it in a typed error is
             what lets the loop later retry with backoff.
        field: error type
        expected: "*agent.HTTPError"
        actual:   provider: HTTP 429: {"error":"slow down"}
        hint: Check resp.StatusCode before you touch the body.
```

That is the whole idea, and it is the thing this course does that a book cannot: it
tells you whether you built it.

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

Twelve chapters. Each one ends with something you can run and a sentence you can say
about it.

| # | Chapter | By the end |
|---|---|---|
| 01 | Talk to the model | `agent -p hello` prints a reply, and you know why it needed a stream parser |
| 02 | Tool calling | it reads a file you named, after asking for the tool by name |
| 03 | Tools that touch disk | it writes what you asked for, and a runaway command cannot hang it |
| 04 | The agent loop | it finishes a task that needs three tool calls, and stops |
| 05 | Prompt assembly | it follows the instructions in your repo's `AGENTS.md` |
| 06 | Sessions | it remembers yesterday, without repeating itself |
| 07 | Context budget | a long session still fits the window, and says what it dropped |
| 08 | Steer and abort | Ctrl-C keeps the half answer you already read |
| 09 | Skills: advertise | the model can see the skills in `skills/` by name |
| 10 | Skills: invoke | `/deploy staging` expands into the skill you wrote |
| 11 | Subagent | it hands a whole job to a fresh context and gets one answer back |
| 12 | MCP (advanced) | it calls a tool that lives in another process |

Chapter 12 is optional. Chapters 01 to 04 already give you an agent you can point at
Ollama or any OpenAI-compatible endpoint and use.

When the arc is done, [chapters/where-next.md](chapters/where-next.md) lists what to
build on top: recovery of pruned context, a schema budget, parallel tool calls, a
second wire format, and the bigger structural changes after those.

## What each chapter contains

Three pages, and you read them at different times. `overview.md` explains how the
mechanism works, before you start. `problem.md` is what you keep beside you at the
keyboard. `example.md` is a full annotated implementation, and you open it after a real
attempt, not before.

`problem.md` lists every claim the verifier makes, by case id. If a claim and a case
ever disagree, the case is right and the page is a bug.

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
kata site                 build the website into site/dist (VitePress)
kata ui                   terminal interface: pick a chapter, grade it
kata watch [NN]           re-grade on every save
kata doctor               check the environment
```

`kata site` generates a VitePress project from the same markdown and builds it into
`site/dist`: search, dark mode, heading anchors, nested navigation, syntax
highlighting and diagrams all come from the theme. It needs Node once, to install
VitePress; taking the course does not.

## How it works

Every chapter leaves you with a program you can run. Chapter 01 gives you an
application that answers a prompt; chapter 04 gives you one that completes a
multi-step task; chapter 10 one that follows skills you write.

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

Go 1.26 or newer.

The agent you build uses only the standard library, and keeps doing so through all
twelve chapters. The `kata` tool depends on Bubble Tea and Lip Gloss for the terminal
interface. Publishing the site additionally needs Node, for VitePress; nothing you do
as a learner does.

## What Kata is not

Kata teaches the machine, not every attachment. It does not ship a terminal UI,
an extension runtime, an LSP client, provider authentication, image input, or a
cloud runner. Chapter 11 covers MCP because the client is small enough to teach.
For a full harness, read [pi](https://github.com/earendil-works/pi),
[crush](https://github.com/charmbracelet/crush), and
[PiG](https://github.com/MichaelKinsy/PiG).

## Publishing

Netlify builds the site from source; nothing generated is committed.

```
go run ./cmd/kata site   →   site/src (generated VitePress project)   →   site/dist
```

`netlify.toml` runs that command and publishes `site/dist`. VitePress validates every
internal link, so a broken cross-reference fails the build instead of shipping.

To preview locally:

```sh
kata site                         # build once, output in site/dist
cd site && npx vitepress dev src  # live reload while editing chapters
```

## Contributing

A chapter is done when `kata verify` is clean: its reference passes every case
and its starter fails at least one. Hints are validated in both directions, so a
case without a ladder fails the test suite. Prose follows
[docs/writing-style.md](docs/writing-style.md).

## License

MIT. See [LICENSE](https://github.com/theakhandpatel/kata/blob/main/LICENSE).
