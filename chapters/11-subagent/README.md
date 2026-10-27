# 11 Subagent

**Part V · Compose** · Difficulty: hard · 90 to 120 minutes · 8 graded cases

The agent can hand a whole job to a child run. The child starts from the task
prompt, works with its own tools, and returns one answer. The parent's conversation
never sees the detail.

| | |
|---|---|
| **Worked example** | [example.md](example.md): the finished code, after an attempt |
| **You edit** | `agent/subagent.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 11` |

## Done when

`kata submit 11` reports 8/8, and this works against a real endpoint:

```sh
./agent -p "find every TODO in this repo and summarise them; use a subagent"
```

---

Next: **12 MCP**, where the agent gains tools it did not compile in.
