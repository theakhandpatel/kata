# Pi event taxonomy

Type: research
Status: open
Blocked by: (nothing)

## Question

Which agent events must the loop emit, and which are pi-specific ceremony?

Read pi's `packages/agent/src/types.ts` and `agent-loop.ts` for the full
`AgentEvent` union, plus `docs/how-pi-works.md`. Then decide the *minimum* set
that makes chapter 04 gradable: which events are load-bearing for the
event-grammar assertion, which only exist for a TUI we are not building, and
which are genuinely needed for steering, abort, and usage accounting.

Deliverable: a table of event -> why it exists -> graded or not.
