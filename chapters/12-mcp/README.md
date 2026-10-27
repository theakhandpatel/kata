# 12 — MCP

**Part V · Compose** · Difficulty: hard, advanced · 90 to 120 minutes · 8 graded cases

The agent gains tools that were never compiled into it. A server process speaks
JSON-RPC over stdio, and its tools appear in the request beside the built-in ones.

| | |
|---|---|
| **How it works** | [overview.md](overview.md) — the framing, the handshake, and why names are namespaced |
| **What to build** | [problem.md](problem.md) — the contract, the graded claims, the hints |
| **Full implementation** | [example.md](example.md) — annotated, after an attempt |
| **You edit** | `agent/mcp.go`, `agent/cmd/agent/main.go` |
| **Submit** | `kata submit 12` |

## Done when

`kata submit 12` reports 8/8, and this works with a real server:

```sh
./agent -p "echo hello through the server" -mcp "npx -y @modelcontextprotocol/server-everything"
```

---

This is the last chapter. Read [where-next.md](../where-next.md) for what to build
on top of the agent you now have.
