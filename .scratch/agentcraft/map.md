# Map — agentcraft

## Destination

A ten-chapter, offline-graded Go course that teaches building a coding-agent
harness: `craft test NN` gives a truthful per-case verdict with no API key,
no network, and no flakiness, and the reference implementations pass every
chapter. Reached when chapters 01–10 all have a lesson, checks, hints, and a
cumulative solution, and `craft test all --solution` is green.

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
- Scope: 10 chapters, listed in `README.md`. TUI, MCP, extensions, LSP, OAuth,
  RPC/SDK, telemetry are out.
- Feedback contract: every case has `why` (mechanism), expected/actual evidence,
  and a three-rung hint ladder. A "not implemented" error must never satisfy a
  negated assertion.

**Skills every session should consult:** `grilling`, `domain-modeling`,
`writing-for-agents`.

## Decisions so far

None yet — nothing has been resolved through a ticket. Chapter 01 was
built directly from the standing decisions above.

## Not yet specified

- How much of the loop's event taxonomy is worth emitting, versus how much is
  pi-specific ceremony. Depends on the research ticket.
- Whether a chapter may have a "stretch" case that is not required to pass.
- How the course handles a learner who refuses the seam and rewrites `types.go`.
- Calibration: are 6 cases per chapter the right size, or does chapter 04 need
  twelve?
- Whether solutions get their own parity-style golden transcripts (PiG's
  approach) or stay plain file overlays.

## Out of scope

- Reimplementing pi's extension runtime, MCP client, LSP integration, or TUI.
- Provider auth flows (OAuth, SSO, Bedrock) — a bearer token covers the lesson.
- Multi-modal content (images, audio).
- A hosted version, accounts, or a web runner.
- Windows-specific process-group handling in the bash tool (note it, do not gate).
