# Where next

You have an agent that talks to a model, runs tools, remembers a session, holds a
context budget, accepts steering, reads skills, delegates to a subagent, and calls
tools that live in another process.

This page lists what to build on top of it. Every entry names the seam it attaches
to and the claim that proves it works. A claim you cannot test is a wish.

## The rule

Write the case first.

A feature with no failing case is indistinguishable from a feature that is broken.
Every chapter in this course arrived with a suite that failed before the code
existed. Keep doing that and the seams will keep holding.

## Next week

Six changes, each a day's work, each attaching to a seam you already have.

| Build | Seam | Claim | Size |
|---|---|---|---|
| Recall what was pruned | new `Tool` plus the budget decorator | a pruned message can be fetched by id | small |
| Summarise instead of dropping | budget decorator | the request is under budget and contains one summary entry naming the count | medium |
| A schema budget | budget decorator | no request advertises more than N tools | small |
| Cache accounting | provider decorator | the request prefix is byte-identical across turns, and hits are reported | small |
| Parallel tool calls | loop | two calls run concurrently and results keep source order | medium |
| A second wire format | `Provider` | the same suite passes against two providers | medium |

### Recall what was pruned

Chapter 07 drops old messages and leaves a marker. The content is still in the
session file, so make it reachable: give every dropped message an id, and add a
`recall` tool that returns one by id.

The claim: with a pruned session, a `recall` call returns text that was absent from
the request. This is what pi-condense does with `context_tree_query`, and it turns
pruning from lossy into reversible.

### Summarise instead of dropping

Instead of a marker, insert a summary message that lists what was removed.

The claim: after compaction the budget holds, and the request contains exactly one
summary entry naming how many messages it replaces. The hard part is that a summary
needs a model call, which means the thing that trims the request now makes requests.
That is a loop, so decide where it lives before you write it.

### A schema budget

Chapter 07 counts tool schemas but never reduces them. Add a ceiling on how many
tools a request may advertise, and unmount the least recently used until it fits.

The claim: with 20 tools registered and a ceiling of 5, the request advertises 5,
and a call to an unmounted tool returns a result that says so instead of crashing.

Two designs exist. Dropping tools is simple. Deferring them, where one meta-tool
loads another tool's schema on demand, is what fx does, and it is the better design
for a large catalogue.

### Cache accounting

Providers charge less when the start of a request is unchanged. You already keep the
system message stable. Now measure it: hash the serialised prefix on each request,
report whether it matched the previous one, and accumulate prompt tokens against
cache hits and misses.

The claim: two turns in a row produce the same prefix hash, and the reported hit
count increases.

### Parallel tool calls

Chapter 04 runs a tool batch one call at a time. Run them concurrently instead, and
append the results in the order the model asked for them.

The claim: two calls take roughly one call's wall time, and the result order still
matches the call order. The second half is the part that breaks, because completion
order is not source order.

### A second wire format

Add Anthropic's Messages API beside the OpenAI-compatible one.

The claim: the chapter 01 to 03 suites pass against both providers without changing
a case. If they do not, the provider seam was leaky, and you have found out cheaply.

## Later

Bigger changes. Each one alters a structure rather than adding to it.

| Build | Seam | What changes | Size |
|---|---|---|---|
| Sessions as a tree | `Recorder` | a message points at its parent; branching and forking become possible | large |
| Background subagents | `ToolSource` plus a queue | a child returns a receipt and delivers later | large |
| MCP resources and prompts | MCP client | the protocol grows past `tools/` | medium |
| A terminal UI | event stream | the events you emit become a rendered transcript | large |
| An extension API | every seam | in-process plugins register tools and contributors | large |
| Evals | nothing | a second harness that scores outcomes, not correctness | large |

### Sessions as a tree

Today a session is a line. Make each record point at a parent, and the file becomes a
tree: branch from any point, fork a conversation, keep a compaction beside the
original instead of over it.

The claim: forking at message 5 produces a session whose request contains messages 1
to 5 and then the new turn, and the original file still reads back unchanged.

pi's session format is the reference. A line-shaped file cannot express this, so the
change is structural, not additive.

### Background subagents

Chapter 10 blocks the parent while the child works. The alternative returns a receipt
immediately and delivers the answer later as a separate message.

The claim: a parent that starts a child and then answers something else receives the
child's result in a later turn, exactly once, without a second `tool_result` for the
original call.

The hard parts are ordering and cancellation: a result can arrive when no call is
waiting, and a parent can be interrupted while its child is still running.

### MCP resources and prompts

Your client speaks `tools/`. MCP also defines resources (readable documents) and
prompts (reusable templates). Add `resources/list` and `resources/read`, expose them
through a skill-like contributor, and keep the tool adapter untouched.

The claim: a resource appears in the system prompt by name, and reading it returns
the server's text.

### A terminal UI

The event stream is already the right shape: `turn_start`, `text`, `tool_call`,
`tool_result`, `turn_end`, `budget`, `steer`, `agent_end`. A UI is a renderer over
those events plus a way to type steering messages.

Bubble Tea is the Go default. The claim that matters: the same events render to a
terminal and to a text log, so the UI cannot contain logic the log does not have.

### An extension API

Every chapter added a seam. An extension API turns those seams into a public surface:
a plugin registers tools, contributors, and event handlers, and receives the
registry and the provider.

This is what pi ships, and the honest version is a large design. Read how pi's
`AgentLoopConfig` exposes hooks before you invent one.

### Evals

Everything in this course grades correctness: did the request have the right shape,
did the result pair with its call. Nothing grades quality: was the answer any good.

Evals need a real model, money, and judgement, and they cannot gate a run. Build them
as a separate tool that scores a transcript after the fact, and never let them decide
whether the harness works.

## What not to build

- **A second pi.** Feature parity is not a goal. The machine is.
- **A feature whose only evidence is that the program still runs.** That is what a
  case is for.
- **A live-model check inside the graded suite.** It is slow, paid, and random. Keep
  it outside, clearly labelled, and never let it gate progress.
- **An abstraction before the second use.** Every seam in this course appeared when
  two chapters needed it, not before.

## Read these

| Concern | File |
|---|---|
| Loop shape, event order, result batching | PiG `agent/agent_loop.go` |
| Provider adapters, streaming, caching | pi `packages/ai/src/api/` |
| Permission policy as a service | crush `internal/permission/` |
| MCP over stdio, hand-rolled | PiG and pi MCP clients |
| Session tree and compaction entries | pi `packages/durable/`, `docs/compaction.md` |
| Pruning with recovery | pi-condense, `context_tree_query` |
| Deferred tool schemas | toolflow, stage-gated mounting |
| A minimal harness in Zig | `vercel-labs/fx` |

## One last thing

The harness is not the interesting part. The interesting part is that you can now
read a harness and tell what it does, because you have built one and watched it fail
in specific, named ways.

That is what the cases bought you.
