# 13 Example

*Full, annotated implementation. Read it after `kata submit 13` passes.*

The compilable files are `reference/ch13/agent/mcp.go` and
`reference/ch13/agent/cmd/agent/main.go`. The loop, the prompt and the registry are
unchanged.

---

## The shape

```
mcp.go      StartMCP, call, Initialize, ListTools, CallTool, mcpTool
main.go     spawn, handshake, merge tools, warn and continue on failure
```

## Step 1: a process, and two pipes

```go
cmd := exec.CommandContext(ctx, "sh", "-c", command)
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Start()
```

`sh -c` lets the flag carry arguments, which is how people actually write these
commands (`npx -y some-server`). The server's stderr is not attached: stdout is the
protocol, and mixing diagnostics into it breaks the framing on the first warning.

## Step 2: one request, one matching response

```go
func (c *MCPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := nextMCPID()
	// ... write {"jsonrpc":"2.0","id":id,"method":method,"params":params}\n

	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var resp mcpResponse
		if json.Unmarshal(line, &resp) != nil {
			continue
		}
		if resp.ID == nil || *resp.ID != id {
			continue   // a notification, or an answer to an abandoned request
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp %s: %s (%d)", method, resp.Error.Message, resp.Error.Code)
		}
		return resp.Result, nil
	}
}
```

The `continue` on a mismatched id is what keeps this working. Servers send
notifications for progress and logging, and a client that assumes "the next line is
my answer" deadlocks the first time one arrives.

The read runs in a goroutine so `ctx` can interrupt it. A timed-out request leaves
the connection unusable; the chapter says so instead of pretending otherwise.

## Step 3: the handshake, including the silent part

```go
result, err := c.call(ctx, "initialize", map[string]any{
	"protocolVersion": MCPProtocolVersion,
	"capabilities":    map[string]any{},
	"clientInfo":      map[string]any{"name": "kata-agent", "version": "1.0"},
})
// ... record serverInfo.name ...

return c.notify("notifications/initialized", nil)
```

The notification is the part people skip, because nothing visibly fails when you do:
a lenient server answers `tools/list` anyway. A strict one does not, and the failure
looks like a hang.

The server name is taken from the response rather than the command line, because the
name in the command is often `npx` or a path.

## Step 4: namespacing is derived, not hardcoded

```go
func (t *mcpTool) Name() string { return t.server + "." + t.info.Name }
```

`t.server` comes from `sanitizeName(init.ServerInfo.Name)`, which lowercases and
replaces anything outside `[a-z0-9-]`. Nothing is hardcoded to `demo`, so two servers
that both expose `read` produce `alpha.read` and `beta.read`.

Without the prefix, `read` from the server replaces the built-in `read`, and the
model's "I read the file" refers to a file on another machine.

## Step 5: content is a list

```go
var parts []string
for _, block := range call.Content {
	if block.Type == "text" && block.Text != "" {
		parts = append(parts, block.Text)
	}
}
text := strings.Join(parts, "\n")
```

Two decisions:

- **join, do not take the first.** A server that answers with a summary and a detail
  block expects both to arrive.
- **filter by type.** An image block is base64. Passing it through spends context on
  something the model cannot use.

`call.IsError` is honoured by returning an error, which the registry turns into an
`IsError` result. That distinction matters: a tool that ran and failed is not the
same as a transport that broke, but both end up as readable text for the model.

## Step 6: startup failure is a warning

```go
client, err := agent.StartMCP(ctx, *mcpCommand)
if err != nil {
	fmt.Fprintf(os.Stderr, "agent: mcp unavailable: %v\n", err)
} else {
	defer client.Close()
	if err := client.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "agent: mcp handshake failed: %v\n", err)
	} else if err := client.ListTools(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "agent: mcp tools/list failed: %v\n", err)
	} else {
		allTools = append(allTools, client.Tools()...)
	}
}
```

The nesting is deliberate: each step can fail without taking the run with it, and the
agent degrades to its built-in tools. The exit code stays 0, because the person's
question was answered.

The child registry is built from `baseTools` only, so a subagent does not inherit MCP
tools. That keeps the child's schema list small and keeps the dependency order simple.

## The diffs

**1. Skipping the initialized notification** (`mcp-initialize-handshake`)

```diff
  c.call(ctx, "initialize", ...)
+ c.notify("notifications/initialized", nil)
  c.call(ctx, "tools/list", nil)
```

**2. Using the server's tool name unchanged** (`mcp-name-collision`)

```diff
- func (t *mcpTool) Name() string { return t.info.Name }
+ func (t *mcpTool) Name() string { return t.server + "." + t.info.Name }
```

**3. Taking only the first content block** (`mcp-content-blocks-joined`)

```diff
- return call.Content[0].Text, nil
+ // join every text block
```

**4. Exiting when the server is missing** (`mcp-unavailable-is-survivable`)

```diff
- if err != nil { return 1, err }
+ fmt.Fprintf(os.Stderr, "agent: mcp unavailable: %v\n", err)
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch13/agent/mcp.go` | 300 | framing, handshake, adapter, namespacing |
| `reference/ch13/agent/cmd/agent/main.go` | 215 | degrade instead of failing |
| `internal/fake/mcpserver/main.go` | 150 | the other side of the protocol |

## Comparison checklist

- [ ] server spawned with `sh -c`, stdout reserved for the protocol
- [ ] requests carry an id, notifications do not
- [ ] responses with a foreign id are skipped
- [ ] `initialize` then `notifications/initialized` then `tools/list`
- [ ] server name taken from the response and sanitised
- [ ] tool names namespaced, built-ins untouched
- [ ] text blocks joined, other types ignored
- [ ] JSON-RPC error becomes a Go error, then an `IsError` result
- [ ] startup failure warns and continues
- [ ] no `tools/call` when the model asked for nothing

Differences outside this list are style.
