# Tools error shape

Type: research
Status: open
Blocked by: (nothing)

## Question

How do pi and crush represent a *failing* tool result?

`isError` flag, structured `details`, truncated output caps, exit codes, and
whether a failed tool ends the turn. Read crush `internal/agent/hooked_tool.go`
and pi's tool result types. Deliverable: the minimal error contract chapter 03
grades, plus where truncation limits live (tool vs harness).
