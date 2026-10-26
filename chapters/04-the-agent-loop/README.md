# 04 — The agent loop

**Part II · Act** · Difficulty: medium · 60 to 90 minutes · 8 graded cases

The agent stops doing one hop. It keeps asking the model, running tools, and
asking again until the model answers, with a cap so it cannot run forever. It also
starts reporting what it does, as events.

| | |
|---|---|
| **How it works** | [overview.md](overview.md) — the loop as a state machine, and the event grammar |
| **What to build** | [problem.md](problem.md) — the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md) — annotated, after an attempt |
| **You edit** | `agent/loop.go`, `agent/events.go`, `agent/provider_openai.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 04` |

## Done when

`kata submit 04` reports 8/8, and this works against a real endpoint:

```sh
./agent -p "find every TODO in this directory and summarise them" -events
```

---

Next: **05 — Prompt assembly**, where the agent learns to read the project's own
instructions before it starts.
