# 01 Talk to the model

**Part I · Wire** · Difficulty: easy-medium · 30 to 50 minutes · 9 graded cases

You write the one function every later chapter calls. It asks a model a question and
puts the reply back together from the pieces it arrives in.

| | |
|---|---|
| **How it works** | [overview.md](overview.md): why the model sits behind a function, and how a reply arrives |
| **What to build** | [problem.md](problem.md): the task, the nine checks, three hints if you need them |
| **Full implementation** | [example.md](example.md): the finished code, explained |
| **You edit** | `agent/provider_openai.go`, and nothing else |
| **You are given** | `agent/types.go`, `agent/cmd/agent/main.go`, `internal/fake/` |
| **Check your work** | `kata submit 01` |

Read `overview.md` first, then work from `problem.md`. Open `example.md` after you pass,
or after you have tried something and got stuck.

## Done when

`kata submit 01` reports 9/9, and you can run this:

```sh
./agent -p "hello"
```

---

Next: **02 Tool calling**, where the model starts asking you to read files.
