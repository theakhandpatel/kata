# 09 — Skills

**Part IV · Extend** · Difficulty: medium · 75 to 100 minutes · 8 graded cases

The agent starts reading instructions you wrote for it. A skill is a markdown file
with a one-line description; the description travels with every request, the body
only when you invoke it.

| | |
|---|---|
| **How it works** | [overview.md](overview.md) — catalogue versus body, placeholders, and stacking |
| **What to build** | [problem.md](problem.md) — the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md) — annotated, after an attempt |
| **You edit** | `agent/skill.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 09` |

## Done when

`kata submit 09` reports 8/8, and this works against a real endpoint:

```sh
mkdir -p skills/deploy
printf -- '---\nname: deploy\ndescription: Deploy to a region.\n---\nDeploy to $0 in $1.\n' \
  > skills/deploy/SKILL.md
./agent -p "/deploy staging eu-west"
```

---

Next: **10 — Subagent**, where the agent can hand a whole job to a fresh context.
