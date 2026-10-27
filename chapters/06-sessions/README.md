# 06 Sessions

**Part III · Remember** · Difficulty: medium · 60 to 90 minutes · 8 graded cases

A run stops being one process. The agent records the conversation to a file and
picks it up on the next run, without duplicating a single turn.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), append-only logs, what to store, what to rebuild |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/session.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 06` |

## Done when

`kata submit 06` reports 8/8, and this works against a real endpoint:

```sh
./agent -p "remember: the deploy branch is release" -session work.jsonl
./agent -p "which branch do we deploy from?"        -session work.jsonl
```

---

Next: **07 Context budget**, where the conversation finally outgrows the
window and something has to give.
