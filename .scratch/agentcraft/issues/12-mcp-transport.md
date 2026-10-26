# MCP transport

Type: grilling
Status: open
Blocked by: (nothing)

## Question

What is the smallest MCP client that is still honest, and can it be graded
offline?

Candidate scope: spawn the server as a subprocess, speak JSON-RPC 2.0 over
stdio with newline-delimited framing, `initialize` handshake with protocol
version negotiation, `tools/list` mapped into our `ToolDef`, `tools/call`
mapped into a `ToolResult`, then `shutdown`.

Open sub-questions:
- stdio only, or also streamable HTTP? (Recommendation: stdio only. The
  handshake and schema mapping are the lesson; transport plumbing is not.)
- Does the chapter require progress notifications and `notifications/tools/list_changed`,
  or are those out of scope?
- Where does the MCP tool sit in the registry: namespaced `server.tool`, or flat?
  Namespacing is the honest answer for collisions, and it is cheap to grade.

Grading approach to validate: a fake MCP server shipped as a small Go binary in
`internal/fake/mcpserver`, spawned by the check, so the learner's client is
graded against a real subprocess speaking real JSON-RPC — same trick as the fake
HTTP provider, one layer down.
