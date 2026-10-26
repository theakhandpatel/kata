# 05 — Prompt assembly

**Part III · Remember** · Difficulty: easy-medium · 45 to 60 minutes · 8 graded cases

The agent learns to read the project's own instructions before it starts. It also
stops pasting a prompt and starts assembling one, which is what makes chapter 09
possible later.

| | |
|---|---|
| **How it works** | [overview.md](overview.md) — the stable prefix, discovery order, and why assembly beats pasting |
| **What to build** | [problem.md](problem.md) — the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md) — annotated, after an attempt |
| **You edit** | `agent/prompt.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 05` |

## Done when

`kata submit 05` reports 8/8, and this works against a real endpoint:

```sh
printf '# Rules\n\nUse tabs.\n' > AGENTS.md
./agent -p "what rules apply here?" -events
```

---

Next: **06 — Sessions**, where a run stops being a single process and starts
being something you can resume.
