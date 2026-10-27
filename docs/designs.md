# How other harnesses are built

The course teaches one machine. These are four real ones, and what each is worth
reading for. The point is not to copy them; it is to see the same seams at different
scales.

| Project | Language | Size | Read it for |
|---|---|---|---|
| [pi](https://github.com/earendil-works/pi) | TypeScript | about 11 packages | the complete event and extension model |
| [PiG](https://github.com/MichaelKinsy/PiG) | Go | a line-by-line port of pi | the same design in Go, with parity tests |
| [crush](https://github.com/charmbracelet/crush) | Go | about 49 internal packages | what Go wants to look like at scale |
| [fx](https://github.com/vercel-labs/fx) | Zig | 719k lines, 667 files | the smallest concept count that still works |

## pi

The reference implementation of everything this course teaches, plus the parts it
deliberately skips: extensions, a tree-shaped session, compaction as a session
entry, thirty provider adapters, and a terminal UI.

Read these files in order.

| File | What it shows |
|---|---|
| `packages/agent/src/agent-loop.ts` | the loop: stream, buffer calls, run them, append results, repeat |
| `packages/agent/src/types.ts` | the event union, and the before/after tool hooks |
| `packages/ai/src/api/openai-completions.ts` | the provider client you wrote in chapter 01, with retries and caching |
| `packages/mcp/` | a hand-rolled MCP client over stdio |
| `docs/how-pi-works.md` | the architecture in prose, without code |

## PiG

pi rebuilt in Go as one binary, with a parity harness that runs the same scenario
against both implementations and diffs the results.

Read it when you want to see the course's architecture in the language you are
writing. Two things are worth more than the code: `test/parity/` is a working example
of a harness that tests a harness, and `docs/parity/PORT_MAP.md` shows how to keep a
port honest.

## crush

The most idiomatic Go harness. Its loop is not in the repository at all: it comes
from `charm.land/fantasy`, which makes crush an adapter around a shared loop. That is
worth knowing before you go looking for `loop.go`.

Read it for structure rather than for the loop.

| Package | What it shows |
|---|---|
| `internal/permission/` | a permission service, rather than a hook |
| `internal/agent/` | coordination, tool dispatch, queueing |
| `internal/mcp/` | MCP through the official Go SDK |
| `internal/lsp/` | what a real editor integration costs |
| `internal/ui/` | a Bubble Tea interface over agent events |

## fx

The minimal end of the spectrum, and the best evidence for the claim that a harness
is a library with injected capabilities rather than an application.

The marketing says 6.66 MiB. That number is a macOS arm64 build. Linux x86_64 is
13.39 MiB, and the source is larger than crush. What is genuinely smaller is the
concept count: a host supplies instructions, tools, network, storage and permissions,
and the core does the rest.

One measurement from it is worth carrying into your own work: a cold request was
23,884 bytes, and 15,560 of those were tool schemas. Sixty-five percent of a request
was definitions, before any conversation existed. That is why chapter 07 counts both
budgets.

## What the four agree on

- **One call, one result.** A tool call resolves exactly once, including on failure.
- **Results keep source order.** Execution order is yours; append order is not.
- **The model is behind a seam.** Every one of them can be pointed at a different
  provider without touching the loop.
- **Context is the scarce resource.** They differ in how they bound it, not in
  whether they do.

## What they do that this course does not

Extensions and plugins, a terminal UI beyond a chapter list, editor integration,
image input, provider authentication, background subagents, and evaluation harnesses.
[Where next](../chapters/where-next.md) lists which of those is worth building on top of what you
have, and which is a different project.
