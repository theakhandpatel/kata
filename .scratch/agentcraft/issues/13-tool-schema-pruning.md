# Tool schema pruning

Type: grilling
Status: open
Blocked by: 04

## Question

Is pruned-context one chapter or two? There are two independent budgets and I
had collapsed them:

1. **Message budget** — the conversation and its tool outputs. Fixed by
   truncation, elision, summarization, or externalization.
2. **Schema budget** — the tool *definitions* in every request. Fixed by
   unmounting tools the model is not using.

`toolflow` implements the second one ("stage-gated capability mounting, prunes
unused tools"), and it is orthogonal to the first: a request can be under the
message budget and still pay for 40 tool schemas on every single call.

Open sub-questions:
- Should chapter 07 cover both, or does schema pruning earn chapter 08 and push
  everything down one?
- What is the gradable claim for schema pruning? Candidate: a request never
  carries more than N tool definitions, and a call to an unmounted tool produces
  a specific recoverable error rather than a crash.
- Does the course teach *deferring* schemas (a meta-tool that loads a tool's
  schema on demand) or only *dropping* them? Deferral is the more interesting
  design and is what fx does ("skill instructions and MCP tool schemas load when
  needed").

Affects the chapter count, so decide before 08 is written.
