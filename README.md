# Kata

Build a coding-agent harness in Go, with a grader that tells you what is broken.

Read it online at **[kata.akhand.cc](https://kata.akhand.cc)**.

You use a coding agent every day. You type what you want, it reads files, runs commands,
and comes back with an answer. When it works it looks like magic, and when it does not you
are left guessing whether the model was confused or the program around it was.

This is a course about the program around it.

Over twenty-one chapters you write one in Go, from an empty stub to something that can read
a file, run a test, remember yesterday's conversation, and hand a whole job to a second copy
of itself. You start by making a single HTTP request to a model. You end with about 3,200
lines you understand completely, and you can point at the line that decides whether your
agent keeps going or stops.

```sh
git clone <this repo> && cd kata
go build -o kata ./cmd/kata

./kata                 # where you are, what to do next
./kata read 01         # the task for chapter 01
./kata submit 01       # compile, grade, report
```

No account, no API key, no Docker, no network after the clone.

## Why you get graded

Learning this from a tutorial does not work. A model is random, slow and costs money per
call, so "run it and see" teaches you nothing: it worked yesterday, it failed today, and you
cannot tell whether you broke something or the sample answer moved.

So the course ships a second program that plays the model. It is a real HTTP server speaking
real Server-Sent Events to a real socket. Only the words in the reply are fixed. Every check
runs in a second, offline, and fails with a sentence about the thing you got wrong:

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

That is the thing a book cannot do: it tells you whether you built it.

## The arc

Twenty-one chapters, in eight parts. Each one ends with something you can run and a sentence
you can say about it.

| # | Chapter | By the end |
|---|---|---|
| 01 | Talk to the model | `agent -p hello` prints a reply, and you know why it needed a stream parser |
| 02 | Tool calling | the model can ask for a tool, and you can answer one you do not have |
| 03 | Running a tool | it reads a file you named, refuses one outside the project, and caps a huge one |
| 04 | Tools that touch disk | it writes a file, and a command that never ends cannot hang it |
| 05 | The agent loop | it finishes a task that needs three tool calls, and stops |
| 06 | Prompt assembly | it follows the instructions in your repo's `AGENTS.md` |
| 07 | Sessions | it remembers yesterday, without repeating itself |
| 08 | Context budget | a long session still fits the window, and says what it dropped |
| 09 | Steer and abort | Ctrl-C keeps the half answer you already read |
| 10 | Skills: advertise | the model can see the skills in `skills/` by name |
| 11 | Skills: invoke | `/deploy staging` expands into the skill you wrote |
| 12 | Subagent | it hands a whole job to a fresh context and gets one answer back |
| 13 | MCP | it calls a tool that lives in another process |
| 14 | Retries and rate limits | a server that fails once still answers |
| 15 | Bounding a tool result | a 50 KB result arrives as a summary it can follow up |
| 16 | The tool schema budget | twenty tools, four schemas, all reachable |
| 17 | Skill scopes | a skill in the repo beats a general one with the same name |
| 18 | Building the request body | a request with no limit carries no limit key |
| 19 | Session storage | conversations in SQLite: list them, resume one, continue the last |
| 20 | What a conversation costs | `-usage` reports tokens and money per session |
| 21 | Configuring a provider | `agent.json`, a key file, and `-config` saying where each value came from |

Chapter 13 is optional. Chapters 01 to 05 already give you an agent you can point at Ollama
or any OpenAI-compatible endpoint and use.

When the arc is done, [chapters/where-next.md](chapters/where-next.md) lists what to build
on top: recovery of pruned context, parallel tool calls, a second wire format, and the
bigger structural changes after those.

## What a chapter contains

One page to work from, one page to compare against.

`README.md` holds the task, how the mechanism works, the claims the verifier makes, and
three hints if you get stuck. `example.md` is the finished implementation, annotated, and
you open it after a real attempt.

The claims in the page name the case ids from `checks/`. If a claim and a case ever
disagree, the case is right and the page is a bug. A test enforces this.

## Commands

```sh
kata                      status: current chapter, next action
kata submit               grade from your frontier, stop at the first failure
kata submit NN            grade one chapter
kata submit all           grade everything, do not stop
kata submit NN --solution grade the reference instead of your code
kata read [NN] [doc]      the chapter, or its example
kata hint NN <case-id>    three hints, from nudge to answer
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
`site/dist`: search, dark mode, heading anchors, nested navigation, syntax highlighting and
diagrams all come from the theme. It needs Node once, to install VitePress; taking the
course does not.

## How it works

Every chapter leaves you with a program you can run. Chapter 01 gives you one that answers a
prompt, chapter 05 one that completes a multi-step task, chapter 11 one that follows skills
you write.

The checks build `./agent/cmd/agent`, start a scripted server, point `OPENAI_BASE_URL` at it,
and run your binary. They assert on stdout, stderr, exit codes, and the requests your program
sent. Nothing imports your package, so the internal design is yours.

```sh
OPENAI_BASE_URL=http://localhost:11434/v1 ./agent -p "list the files here"
```

Design notes, diagrams and the grading pipeline are in
[docs/architecture.md](docs/architecture.md). The chapter contract is in
[docs/chapter-format.md](docs/chapter-format.md). How pi, crush, PiG and fx are built is in
[docs/designs.md](docs/designs.md).

## Requirements

Go 1.26 or newer.

The agent you build uses only the standard library, and keeps doing so through all
twenty-one chapters. The `kata` tool depends on Bubble Tea and Lip Gloss for the terminal
interface. Publishing the site additionally needs Node, for VitePress; nothing you do as a
learner does.

## What Kata is not

Kata teaches the machine, not every attachment. It does not ship a terminal UI, an extension
runtime, an LSP client, provider authentication, image input, or a cloud runner. Chapter 13
covers MCP because the client is small enough to teach. For a full harness, read
[pi](https://github.com/earendil-works/pi),
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

A chapter is done when `kata verify` is clean: its reference passes every case and its
starter fails at least one. Hints are validated in both directions, so a case without a
ladder fails the test suite. Prose follows [docs/writing-style.md](docs/writing-style.md),
and a test rejects the vocabulary it forbids.

## License

MIT. See [LICENSE](https://github.com/theakhandpatel/kata/blob/main/LICENSE).
