# 01 Talk to the model

**Part I · Wire** · Difficulty: easy-medium · 30–50 minutes · 9 graded cases

You will build the one function everything else in this course calls: a client
for an OpenAI-compatible endpoint that turns a Server-Sent-Events stream back
into a single `Message`.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), the seam, the delta log, the failure map |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/provider_openai.go` only |
| **Given to you** | `agent/types.go`, `internal/fake/` (a scripted server) |
| **Submit** | `kata submit 01` |
| **Reference** | `reference/ch01/` |

Read `overview.md` first. Then work from `problem.md`. Open `example.md` only
after `kata submit 01` passes or after you have been genuinely stuck.

## Done when

`kata submit 01` reports 9/9, and you can answer the self-check at the bottom
of `overview.md` without scrolling up.

---

Next: **02 Tool calling**, where the same stream carries JSON that arrives in
pieces and the fragments are keyed by an index rather than an id.
