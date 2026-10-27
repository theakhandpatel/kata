# 02 Tool calling

**Part I · Wire** · Difficulty: medium · 60 to 90 minutes · 8 graded cases

The agent stops being a chat program. It advertises a tool, receives a call,
runs it against a real file, and sends the result back so the model can answer.

| | |
|---|---|
| **Worked example** | [example.md](example.md): the finished code, after an attempt |
| **You edit** | `agent/provider_openai.go`, `agent/tool.go`, `agent/tools.go`, `agent/cmd/agent/main.go` |
| **Given** | `agent/types.go` (including the `Tool` interface) |
| **Submit** | `kata submit 02` |

## Done when

`kata submit 02` reports 8/8, and this works against a real endpoint:

```sh
echo "hello from a file" > notes.txt
./agent -p "read notes.txt and tell me what it says"
```

---

Next: **03 Tools that touch disk**, where `write` and `bash` join `read`, and
the hard part becomes bounding what a tool returns.
