# 11 Problem

> **Your task:** spawn an MCP server, shake hands with it, and offer its tools to the
> model under namespaced names.

**You edit:** `agent/mcp.go`, `agent/cmd/agent/main.go`
**Given:** `agent/tool.go`, `agent/loop.go`, `agent/subagent.go`
**Time:** 90 to 120 minutes

---

## The contract

```go
// mcp.go
const MCPProtocolVersion = "2024-11-05"

type MCPClient struct{ /* ... */ }

func StartMCP(ctx context.Context, command string) (*MCPClient, error)
func (c *MCPClient) Initialize(ctx context.Context) error
func (c *MCPClient) ListTools(ctx context.Context) error
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
func (c *MCPClient) Tools() []Tool
func (c *MCPClient) ServerName() string
func (c *MCPClient) Close() error
```

One flag, one process:

```sh
agent -p "..." -mcp "/path/to/server"
```

Tool names are `server.tool`, where `server` is the name reported by `initialize`,
lowercased and sanitised to `[a-z0-9-]`.

## What the verifier does

It builds the binary **and** a scripted MCP server. It runs the agent with `-mcp`
pointing at that server, then inspects two traces:

1. the **HTTP request trace** from the scripted model, for what was advertised and
   what tool result came back;
2. the **server's method log**, written by the fixture, for the handshake order and
   whether a call was made at all.

## The verifier will assert that

- `mcp-tools-advertised`: the request advertises `read`, `write`, `bash`, `task`,
  `demo.echo` and `demo.read`.
- `mcp-initialize-handshake`: the server received `initialize`,
  `notifications/initialized`, `tools/list`, in that order.
- `mcp-tool-call`: a call to `demo.echo` reaches `tools/call` on the server and its
  text comes back as the tool result.
- `mcp-content-blocks-joined`: both text blocks appear in the result.
- `mcp-name-collision`: `demo.read` returns the server's content while `read`
  remains the built-in.
- `mcp-error-is-a-result`: a JSON-RPC error becomes an error result, and the run
  exits 0.
- `mcp-unavailable-is-survivable`: a server that cannot start produces a warning,
  no MCP tools, and still answers.
- `mcp-no-call-when-unused`: a plain run performs no `tools/call`.

## Notes

- **One JSON object per line**, requests with an id, notifications without.
- **`initialize` first, then the notification, then `tools/list`.** A strict server
  waits for the notification.
- **Skip responses whose id is not yours.** Log notifications share the stream.
- **Namespace by server name**, or a server tool silently replaces a built-in.
- **Join text blocks**, ignore types you cannot use.
- **A JSON-RPC `error` becomes a Go error**, which the registry turns into an
  `IsError` result.
- **Startup failure is a warning, not an exit.** The agent is still useful.
- **Deliberate omission:** stdio only, tools only, no reconnection.

## Hints

<details><summary>Rung 1, the mechanism</summary>

A subprocess, one JSON object per line, ids to match answers. Spawn it, handshake,
ask for its tools, wrap each one.
</details>

<details><summary>Rung 2, the APIs</summary>

`exec.CommandContext` with `StdinPipe` and `StdoutPipe`, `bufio.Reader.ReadBytes('\n')`,
`encoding/json` on each line, `cmd.Process.Kill` on close.
</details>

<details><summary>Rung 3, what the code does</summary>

`StartMCP` runs `sh -c <command>`, keeps stdin and stdout. `call` writes a request with
an id and reads lines until one has that id, skipping notifications. `Initialize` sends
`initialize`, records `serverInfo.name`, then sends the `notifications/initialized`
notification. `ListTools` decodes `tools`. `CallTool` sends `tools/call` with the
original name and the arguments, then joins `content[].text` for blocks of type text.
`mcpTool` adapts one entry: `Name()` prefixes the server, `Parameters()` returns the
input schema, `Execute` forwards to `CallTool`. In `main`, warn and continue when the
server cannot start.

`kata hint 11 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 11` reports 8/8, and:

- the agent calls a tool that lives in another process;
- a server that is missing or broken does not stop it from answering;
- you can explain the difference between a request and a notification.
