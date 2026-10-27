# 10 Skills: invoke

**Part IV · Extend** · Difficulty: medium · 30 to 50 minutes · 6 graded cases

Chapter 09 put skill names in the system message. Now `/name args` becomes the
skill's own words, with the arguments filled in.

| | |
|---|---|
| **Worked example** | [example.md](example.md): the finished code, after an attempt |
| **You edit** | `agent/skill.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 10` |

## Done when

`kata submit 10` reports 6/6, and this works against a real endpoint:

```sh
./agent -p "/deploy staging eu-west"
```

The request should contain the skill's body with `$0`, `$1` and `$ARGUMENTS` already
replaced.

---

Next: **11 Subagent**, where the agent hands a whole job to a fresh context.
