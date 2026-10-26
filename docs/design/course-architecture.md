# Course architecture

**Status: design.** Chapter prose is frozen until research streams A, D and E
land. Settled and built: the verifier, the CLI, the chapter format. This document
is the contract the research has to satisfy.

---

## 1. Build order: reference first, then subtract

Your instinct is right, and it is the difference between a course that composes
and one that rots. But it only works with one rule attached.

### The order

```
        1. build the finished harness            (the reference)
        2. tag it                                git tag reference
        3. derive chapter N by DELETION          reference minus chapters N+1..11
        4. the lesson is the diff N → N+1
```

Not: write chapter 1, then 2, then 3, hoping the abstraction survives contact
with chapter 7.

### Why this order and not the obvious one

**Invariants are discovered by building, not imagined.** Concrete example from
research stream C: I would have written chapter 07's claim as "the request never
exceeds the budget", then discovered while building that *no production
implementation actually enforces a ceiling* — `pi-condense` uses a threshold
trigger, not a limit. A hard ceiling requires estimating tokens **before** sending
(which pi does in `agent/context_tokens.go`). That is a design decision that
ripples backwards into chapter 05's prompt assembly. Write chapter 05 first and
you rewrite it.

**Deletion is cheap; retrofitting is not.** If chapter 3's abstraction breaks in
chapter 7, deleting back from a working reference costs minutes. Fixing forward
costs a rewrite of chapters the learner has already completed — and any learner
who passes chapter 3 keeps a mental model you have now invalidated.

**Monotonicity is a testable property.** If chapter N's code is a strict subset
of chapter N+1's, then:
- `solutions/chNN` is *the reference at that stage* — one artifact, maintained
  once;
- `lathe submit --solution` proves every chapter is still satisfiable after any
  change to the reference;
- chapter N+1 can never invalidate chapter N's checks, because N+1 only adds.

Without monotonicity, "cumulative regression" (our strongest grading property) is
a hope rather than a guarantee.

### The failure mode this rule exists to prevent

**The reference must be an oracle, not a mold.**

If `checks/` assert file layout, package names, or function names, the course
becomes "reproduce this tree", which is copy-typing with extra steps and teaches
nothing about design. So:

| Assert | Never assert |
|---|---|
| what crosses the `Provider` seam | which file the code lives in |
| the event sequence emitted | the names of internal types |
| files created on disk by a tool | how a tool is registered |
| the session file's format | the struct that writes it |
| the request trace | the function that builds it |

The reference exists to prove the claims are *satisfiable*, not to be matched.
A learner who satisfies every claim with a completely different internal design
has passed, and should.

---

## 2. What the reference harness is

The minimum that satisfies all eleven chapters. Not a product.

| | |
|---|---|
| Language | Go, stdlib only (no third-party deps in the reference) |
| Size target | 1,500–2,500 LOC |
| Provider | one OpenAI-compatible client; a second wire format only if ch02 needs it |
| Tools | `read`, `write`, `bash` |
| TUI | none |
| MCP | stdio only, chapter 11 |
| Extensions | none — hooks are chapter 08's business or not a chapter at all |
| Runtime | one binary, `cmd/harness`, prompt via `-p`, transcript to stdout |

```
cmd/harness/main.go     CLI contract: -p prompt, -m model, -s session
harness/provider.go     the seam + wire
harness/tool.go         Tool interface + registry
harness/tools_*.go      read, write, bash
harness/loop.go         the loop, events, steering, abort
harness/prompt.go       base instructions + AGENTS.md discovery
harness/session.go      JSONL append, replay, resume
harness/budget.go       token estimate, pruning, recovery
harness/skill.go        discovery, lazy load, args, stacking
harness/subagent.go     child loop, isolation, result envelope
harness/mcp.go          stdio JSON-RPC client
```

### The CLI contract (decided here, because chapters depend on it)

Learner's binary: `./harness -p "<prompt>"`. Final answer to **stdout**, nothing
else. Debug to **stderr**. Exit 0 on success, non-zero on failure. This is what
CodeCrafters does (`./your_program.sh -p "..."`), and it is what makes
out-of-process checks possible for chapters 03, 04, 10 and 11 without a facade.

---

## 3. The verifier

### Built

| Component | Job |
|---|---|
| `internal/fake` | scripted OpenAI-compatible SSE server; records every request |
| `internal/grader` | case DSL: `why`, expected/actual, three-rung hint, suite report |
| `internal/hints` | ladder table, keyed by chapter + case |
| `cmd/lathe` | `submit`, `read`, `hint`, `list`, `progress`, build triage |
| `solutions/chNN` | cumulative overlay, graded in a throwaway copy |

### Missing, and needed by which chapter

| Assertion family | Needed by | Shape |
|---|---|---|
| **event-sequence grammar** | 04, 08 | match the emitted event log against a declared grammar; report the first divergence with context |
| **golden request trace** | 02, 05, 07 | diff the full request body against a stored trace, with a field-level diff |
| **invariants / properties** | 07, 10 | "no orphan `tool_use`", "exactly one `tool_result` per call", run over every recorded request |
| **filesystem effects** | 03 | assert on a temp dir after a tool runs, including truncation caps |
| **subprocess / stdio** | 10, 11 | spawn a fake MCP server and a fake child harness; speak JSON-RPC over pipes |
| **determinism replay** | 06, 07 | run twice, assert identical requests and identical output |

The grammar matcher is the highest-risk build: it must produce a message a
learner can act on ("expected `tool_result` for id `call_2`, got `turn_end`"),
which means carrying source positions through the event log rather than diffing
strings.

---

## 4. Invariant catalogue

The core table. Every row becomes cases; the last column is where the evidence
comes from.

| Ch | Claim | Family | Caught failure | Evidence |
|---|---|---|---|---|
| 01 | text deltas append | golden | one-chunk tests pass | — |
| 01 | usage read from final chunk | golden | zero tokens forever | — |
| 01 | truncated stream errors | property | dropped connection looks complete | — |
| 02 | tool calls fold by **index** | golden | continuation fragments omit `id` | fake server |
| 03 | tool schema matches declaration | golden | model calls a tool that doesn't exist | — |
| 03 | tool errors are results, not panics | property | loop dies on `ENOENT` | stream A |
| 04 | every `tool_use` gets exactly one `tool_result` | grammar | orphaned call, hang | B (verified in both plugins) |
| 04 | loop stops when no tool calls | grammar | infinite loop, or early exit | — |
| 05 | `AGENTS.md` discovered and attributed | golden | silently ignored, or leaked sibling | — |
| 06 | replay is idempotent | property | resume doubles history | — |
| 07 | request never exceeds budget | property | **stronger than every production tool** | C (thresholds, not ceilings) |
| 07 | pruning never orphans a `tool_use` | grammar | `pi-context-prune` does exactly this | C |
| 07 | pruned content is marked, and recoverable | property | model re-reads the same file forever | C (`context_tree_query`) |
| 08 | abort mid-stream yields one well-formed result | grammar | partial assistant message | A |
| 09 | skill body loads on demand | golden | full catalog in every prompt | — |
| 10 | child sees no parent turns | golden | context bleed | B (fresh is default) |
| 10 | child result enters parent exactly once | property | duplicate `tool_result` | B |
| 11 | `tools/list` maps to `ToolDef` | golden | silent schema loss | — |

Two rows deserve emphasis because the research changed the design:

- **ch04/ch10 "exactly one `tool_result` per call."** Stream B verified that both
  subagent plugins answer a spawn `tool_use` exactly once on the sync path, and
  that background work returns a *receipt* then a later message — never a second
  result. That is a real, subtle contract, and it is gradeable.
- **ch07 "no orphaned `tool_use`."** Stream C found `pi-context-prune` deletes
  `toolResult` messages while keeping the assistant `toolCall`, so pi-ai
  synthesises `isError: "No result provided"`. A production tool ships this bug.
  It becomes a graded case, with the real code as the cautionary example.

---

## 5. CLI

```
lathe                       status: chapter, next action, one line each
lathe submit [NN]           compile triage → grade → next
lathe submit NN --solution  grade the reference in a throwaway copy
lathe read [NN] [doc]       problem (default) | overview | example
lathe hint NN <case-id>     three rungs
lathe list                  the arc, grouped into parts
lathe progress              what passed, when
```

Rules baked in: `--solution` never records progress; a build error is reported as
"not ready" and never as "failed cases"; the learner never types `go test`.

Next: `lathe watch` (re-submit on save), `lathe doctor` (Go version, pager,
terminal width), `lathe verify --live` (against Ollama, clearly non-graded).

---

## 6. Content pipeline

```
research stream  →  minimal teaching model  →  failure modes  →  cases  →  docs
   (path:line)        (Go declarations)        (naive bugs)     (grader)   (3 files)
```

Two rules keep the docs honest:

1. `overview.md` may not contain a working implementation.
2. `example.md` may not contain a new concept.

If an idea only appears in the example, it belongs in the overview first.

---

## 7. Blocked on research

| Decision | Ticket | Waiting on |
|---|---|---|
| hooks: own chapter, or two callbacks inside ch04? | 11 | A (pi `AgentLoopConfig`), C (toolflow's guard) |
| 07 alone, or 07 + 08 for tool-schema pruning? | 13 | C (toolflow is a separate axis) |
| minimum event taxonomy for ch04's grammar | 01, 02 | A |
| MCP scope: stdio only? namespaced tools? | 12 | A |
| does the course get a live mode? | 10 | E (fx's testing) |

Nothing in §4 is final while those are open.

---

## 8. Docs delivery: same markdown, two renderers

Decision: **markdown stays the single source of truth, and a static site is a
renderer over it, not a second artifact.** No content lives only in the site.

```
chapters/NN-slug/{overview,problem,example}.md      ← truth
        │
        ├── lathe read NN [doc]        terminal, $PAGER, mermaid elided
        └── lathe site → site/docs/    VitePress input, mermaid rendered
```

Why VitePress over mdBook (the two prior-art courses use one each):

| | VitePress (walkinglabs) | mdBook (harness-101) |
|---|---|---|
| Nav | auto sidebar from config; prev/next | `SUMMARY.md` |
| Search | built-in local index | built-in |
| Diagrams | mermaid plugin | bundled `mermaid.min.js` |
| Extras | code groups, custom containers, dark mode | simpler, stabler |
| Needs | Node, at author time only | Rust binary |

`lathe` generates the sidebar and copies/links the markdown, so adding a chapter
never means editing site config. The learner never installs Node; only whoever
publishes the site does.

What to steal from walkinglabs' site, specifically:

1. **Three content types, not one.** They split `lectures/` (why), `projects/`
   (build), and `harness-designs/` (case studies of real harnesses: claude-code,
   codex, deepseek, pi). Our equivalent: `chapters/` (build), `concepts/` (why),
   and `designs/` (case studies). The `designs/` section is nearly free for us —
   it is what the five research streams are producing anyway.
2. **A `code/` directory per page** with runnable, copyable examples.
3. **Stable anchors** (`anchor-map.json`) so links survive edits. We get this by
   freezing heading text per page.

What not to steal: 15 translations (we have one learner), PDF export, and the
lecture-count ambition.

---

## 9. Prior art, and where this is actually different

Four projects occupy adjacent ground. Only one of them grades.

| Project | Format | Lang | Progressive code | Graded |
|---|---|---|---|---|
| **CodeCrafters** BYO Claude Code | hosted stages, `git push` | 10 | no | ✅ hosted tester |
| **walkinglabs/learn-harness-engineering** | VitePress, 14 lectures + 8 projects | TS snippets | no | ❌ |
| **IsuruMaduranga/harness-engineering-101** | mdBook, 16 ch + 4 appendices | Python, `v1_chat → v4_subagents` | ✅ | ❌ |
| **damiandemasi/build-your-own-agent** | single long post | TS, ~200 LOC | no | ❌ |
| **lathe** (this) | local chapters + CLI | Go | ✅ | ✅ offline, deterministic |

### The distinction that matters

**walkinglabs is not a competitor — it is a different subject.** Their lectures
are about *harness engineering as agent operations*: repo as system of record,
splitting AGENTS.md, session continuity, feature lists, declaring victory too
early, loop engineering. That is how to *use* a harness well. We teach how to
*build* one. They are complementary, and their `harness-designs/` pages (which
already cover pi) are the kind of case study our `designs/` section should hold.

**harness-engineering-101 is the closest competitor**, and it is genuinely good:
16 chapters, Python reference code that grows `v1_chat → v2_tools → v3_agent →
v4_subagents`, mdBook site. But it is a *book*: you read it, you copy the code,
nothing tells you whether yours works. It has no verifier, no cases, no hints,
no regression.

That defines our claim precisely:

> The only harness course that tells you whether you built it. Offline, in Go,
> with no API key and no flakiness.

Consequences for scope:

- **Do not compete on prose volume.** They have 16 chapters; we have 11, and ours
  are shorter, because the verifier carries the weight. Point learners at their
  book for the conceptual chapters we deliberately skip.
- **Their `v1→v4` naming validates the subtraction strategy** in §1: cumulative
  snapshots of one harness, named by capability. Adopt the same shape for
  `solutions/chNN`.
- **Their chapter titles are claims, ours are nouns.** "Context is a budget"
  beats "Context budget"; "It's just a JSON array" beats "Message model". Keep
  noun titles for nav stability, and add a one-line claim as the subtitle on
  every chapter page. Cheap, and it sets expectations.
- **Three topics they have that we do not**, worth tickets rather than chapters:
  prompt caching economics, debugging the transcript, guardrails/reflexes.

### Positioning statement

For a Go developer who has used a coding agent and wants to know how it works:
**lathe** is a local, graded build of a harness — eleven chapters, each one
proving a specific mechanism against a scripted model, with the failure message
naming the bug — unlike *harness-engineering-101*, which is a book with reference
code, or CodeCrafters, which grades a different, much shallower artefact.
