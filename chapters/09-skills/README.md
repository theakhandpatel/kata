# 09 Skills: advertise

**Part IV · Extend** · Difficulty: easy-medium · 30 to 40 minutes · 4 graded cases

The agent learns to read instructions you wrote for it. A skill is a markdown file
with a one-line description. This chapter finds those files and puts their names in
the system message; **chapter 10 makes them runnable**.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), one file, two audiences |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/skill.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 09` |

## Done when

`kata submit 09` reports 4/4, and this works against a real endpoint:

```sh
mkdir -p skills/review
printf -- '---\nname: review\ndescription: Review the current diff.\n---\nRead the diff.\n' \
  > skills/review/SKILL.md
./agent -p "what skills do you have?"
```

The model should be able to tell you about `review` without reading its body.

---

Next: **10 Skills: invoke**, where `/name args` turns into the body.
