# 03 Tools that touch disk

**Part II · Act** · Difficulty: medium · 60 to 90 minutes · 8 graded cases

The agent gains `write` and `bash`. The tools themselves are short. The hard part
is that both can produce unbounded output, and one can run forever.

| | |
|---|---|
| **How it works** | [overview.md](overview.md), untrusted arguments, unbounded output, unbounded time |
| **What to build** | [problem.md](problem.md), the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md), annotated, after an attempt |
| **You edit** | `agent/tools.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 03` |

## Done when

`kata submit 03` reports 8/8, and this works against a real endpoint:

```sh
./agent -p "write a file called notes.md with three bullet points about Go, then show it to me"
```

---

Next: **04 The agent loop**, which replaces the single hop with a loop, adds a
step cap, and starts grading the order of what happens.
