# 08 Steer and abort

**Part IV · Extend** · Difficulty: medium-hard · 75 to 100 minutes · 8 graded cases

A run stops being something you wait for. You can interrupt it and keep what
arrived, or say something while it works and have it land at the turn boundary.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), where an interrupt lands, and why steering is a boundary problem |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/loop.go`, `agent/steering.go`, `agent/provider_openai.go`, `agent/events.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 08` |

## Done when

`kata submit 08` reports 8/8, and this works against a real endpoint:

```sh
printf 'also check the tests\n' | ./agent -p "add a flag" -steer-stdin
```

---

Next: **09 Skills: advertise**, where the agent starts listing the instructions you
wrote for it, without loading them.
