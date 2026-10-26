# Fake interleaving

Type: task
Status: open
Blocked by: (nothing)

## Question

Prove `internal/fake` can interleave text and tool-call fragments the way real
providers do.

The current fake emits all text, then all tool calls. Real streams can interleave
delta types, omit `id`/`name` on continuation fragments, and finish with
`finish_reason: tool_calls`. Before chapter 02 is written, extend the fake and
add a test that a naive index-less parser fails and an index-keyed one passes.

Doing, not deciding: the chapter cannot be specified until the fixture can
express the bug.
