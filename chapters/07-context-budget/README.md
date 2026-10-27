# 07 Context budget

**Part III · Remember** · Difficulty: hard · 90 to 120 minutes · 8 graded cases

The conversation finally outgrows the window. The agent has to decide what to
drop, and be able to say why.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), two budgets, ceilings versus thresholds, and why pruning is a decorator |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/budget.go`, `agent/events.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 07` |

## Done when

`kata submit 07` reports 8/8, and this works against a real endpoint:

```sh
./agent -p "..." -session long.jsonl -max-context 2000 -events 2>&1 >/dev/null | grep budget
```

---

Next: **08 Steer and abort**, where a run stops being something you wait for.
