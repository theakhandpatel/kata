# Map — lathe

## Destination

A ten-chapter, offline-graded Go course that teaches building a coding-agent
harness: `lathe test NN` gives a truthful per-case verdict with no API key,
no network, and no flakiness, and the reference implementations pass every
chapter. Reached when chapters 01–10 all have a lesson, checks, hints, and a
cumulative solution, and `lathe test all --solution` is green.

## Notes

**Domain:** Go; LLM provider wire protocols; agent loops; terminal tooling.
Audience: a proficient Go developer who has used a coding agent but never built
one. Effort is *execution-bearing*: this map carries the build, not just the
decisions (override of the skill's plan-only default, at the user's request).

**Standing decisions (settled before charting, not re-litigable):**

- The product is the verifier. The harness is the byproduct; pi feature parity is
  explicitly out of scope.
- Grading seam: in-process `harness.Provider` facade. One binary-level smoke
  check may be added later; no protocol-driven grading.
- Determinism: scripted fixtures for 100% of grades. Any live-model mode is
  non-graded and must never gate progress.
- Provider order: OpenAI-compatible wire format first (works offline against
  Ollama/llama.cpp, no key), second wire format in a later chapter to teach the
  abstraction.
- One continuous codebase; chapters are cumulative; solutions overlay in order
  and live in a separate Go module so the root module never compiles them.
- Scope: 11 chapters. ch07 is "context budget & pruning" (pruning is the budget
  mechanism, not a separate topic). ch11 is MCP, advanced and deliberately last.
  TUI, extension runtime, LSP, OAuth, RPC/SDK, telemetry are out.
- Deep source survey commissioned before chapters 02+ are specified:
  `docs/research/deep-harness-survey.md`. Chapter scope for hooks is held until
  that evidence lands (ticket 11).
- Feedback contract: every case has `why` (mechanism), expected/actual evidence,
  and a three-rung hint ladder. A "not implemented" error must never satisfy a
  negated assertion.

**Skills every session should consult:** `grilling`, `domain-modeling`,
`writing-for-agents`.

**Research in flight (all at `thinking: max`, all evidence-first: `path:line` +
verbatim excerpts + an explicit "could not determine"):**

| Stream | Sources | Output |
|---|---|---|
| A. Go architecture | crush, PiG, pi | `docs/research/go-architecture.md` |
| B. Subagent plugins | nicobailon/pi-subagents, tintinweb/pi-subagents, pi-subagent-manager | `docs/research/subagent-architectures.md` |
| C. Context pruning | pi-context-prune, pi-condense, toolflow | `docs/research/context-pruning.md` |
| D. Self-testing | PiG `test/parity`, crush test doubles, pi evals | `docs/research/harness-self-testing.md` |
| E. Minimal harness | vercel-labs/fx (Zig, 6.66 MiB, embeddable) | `docs/research/fx-zig-harness.md` |

**Extraction rule (how research becomes course content).** Every stream must
produce three artefacts, in this order, and the third is the syllabus:

1. *What the code actually does* — the reconstructed architecture, not the
   README's claim.
2. *The minimal teaching model* — the smallest correct Go design, as type
   declarations, with an explicit list of what was removed and why removing it
   is safe.
3. *The gap* — the failure modes a competent Go developer ships when
   reimplementing (2) without knowing (1). **Each failure mode becomes one graded
   case.** The distance between the real implementation and the minimal one is
   the curriculum.

Module D is load-bearing for the product itself: lathe's verifier is the
same artefact as PiG's parity harness, one scale down.

**Design triangle.** The five streams are chosen so the course has three
reference points rather than one:

| | Repo | Proves | Cost |
|---|---|---|---|
| Minimal | `vercel-labs/fx` | fewest concepts that still work | opaque to most learners |
| Idiomatic Go | `crush` | what Go wants to look like at scale | 49 packages |
| Gold standard | `pi` | complete event/extension model | TypeScript, 11 packages |

Chapters 01-04 should land closest to fx's concept count, not crush's. Portability
of fx is a *design* question, not a language question: the size win is Zig's
(allocators, comptime, no runtime) and does not port; the concept win might.

## Decisions so far

None yet — nothing has been resolved through a ticket. Chapter 01 was
built directly from the standing decisions above.

## Not yet specified

- Whether hooks earn their own chapter or are a thin wrapper over the
  before/after tool-call callbacks (ticket 11, awaiting survey evidence).
- Where pruning sits: a separate mechanism from the budget, or the same chapter
  with elision + summary as two graded variants.
- MCP transport scope: stdio only, or stdio + streamable HTTP (ticket 12).
- Whether a chapter may have a "stretch" case that is not required to pass.
- How the course handles a learner who refuses the seam and rewrites `types.go`.
- Calibration: are 6 cases per chapter the right size, or does chapter 04 need
  twelve?
- Whether solutions get their own parity-style golden transcripts (PiG's
  approach) or stay plain file overlays.

## Out of scope

- Reimplementing pi's extension runtime, LSP integration, or TUI.
- MCP transports beyond stdio (streamable HTTP / SSE) — the handshake and tool
  mapping are the lesson; transport plumbing is not.
- Provider auth flows (OAuth, SSO, Bedrock) — a bearer token covers the lesson.
- Multi-modal content (images, audio).
- A hosted version, accounts, or a web runner.
- Windows-specific process-group handling in the bash tool (note it, do not gate).
