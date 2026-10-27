# 05 Prompt assembly

**Part III · Remember** · Difficulty: easy-medium · 45 to 60 minutes · 8 graded cases

The agent learns to read the project's own instructions before it starts. It also
stops pasting a prompt and starts assembling one, which is what makes chapters 09 and 10
possible later.

| | |
|---|---|
| **Worked example** | [example.md](example.md): the finished code, after an attempt |
| **You edit** | `agent/prompt.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 05` |

## Done when

`kata submit 05` reports 8/8, and this works against a real endpoint:

```sh
printf '# Rules\n\nUse tabs.\n' > AGENTS.md
./agent -p "what rules apply here?" -events
```

---

Next: **06 Sessions**, where a run stops being a single process and starts
being something you can resume.
