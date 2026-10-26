# Hooks boundary

Type: grilling
Status: open
Blocked by: 01, 06

## Question

Do hooks earn their own chapter, or are they a thin wrapper over the
before/after tool-call callbacks we already teach in chapter 04?

The candidate intercept points are: before request, before tool call (with
veto), after tool call (with rewrite), after turn, on session start. If the
honest answer is "one callback with a struct argument", it belongs inside the
tools chapter and does not deserve a chapter number.

Held until `docs/research/deep-harness-survey.md` lands, because the deciding
evidence is how much policy lives in pi's extension handlers and crush's
`internal/hooks` versus how much is just plumbing.

If it graduates: the gradable claim is that a hook can block a tool call and the
loop records an error tool_result instead of executing it — pi calls this
`beforeToolCall` returning `{block:true}`.
