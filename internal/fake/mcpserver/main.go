// Command mcpserver is a scripted MCP server used by the chapter 11 checks.
//
// It speaks newline-delimited JSON-RPC 2.0 on stdin and stdout, which is the stdio
// transport MCP defines. It is deliberately small: initialize, tools/list, tools/call,
// and shutdown. Everything else is ignored, because that is what a notification is.
//
// Set MCP_LOG to a file path to record the sequence of methods it received. That log
// is how the checks prove the handshake happened in order.
// Set MCP_FAIL=1 to make every tools/call return a JSON-RPC error.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func main() {
	var log *os.File
	if path := os.Getenv("MCP_LOG"); path != "" {
		log, _ = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		defer func() {
			if log != nil {
				_ = log.Close()
			}
		}()
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	respond := func(id json.RawMessage, result any) {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		})
		fmt.Fprintf(out, "%s\n", body)
		_ = out.Flush()
	}
	fail := func(id json.RawMessage, code int, message string) {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": code, "message": message},
		})
		fmt.Fprintf(out, "%s\n", body)
		_ = out.Flush()
	}

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if log != nil {
			fmt.Fprintln(log, req.Method)
		}

		switch req.Method {
		case "initialize":
			respond(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "demo", "version": "1.0"},
			})

		case "notifications/initialized":
			// A notification carries no id and expects no reply.

		case "tools/list":
			respond(req.ID, map[string]any{"tools": []any{
				map[string]any{
					"name":        "echo",
					"description": "Echo the given text back.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
						},
						"required": []string{"text"},
					},
				},
				map[string]any{
					// Same name as a built-in tool, on purpose.
					"name":        "read",
					"description": "Read a file on the server side.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
					},
				},
			}})

		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)

			if os.Getenv("MCP_FAIL") == "1" {
				fail(req.ID, -32000, "server refused the call")
				continue
			}

			switch params.Name {
			case "echo":
				text, _ := params.Arguments["text"].(string)
				// Two content blocks, to prove the client joins them.
				respond(req.ID, map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "echo: " + text},
					map[string]any{"type": "text", "text": "(second block)"},
				}})
			case "read":
				respond(req.ID, map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "mcp-read-content"},
				}})
			default:
				fail(req.ID, -32601, "unknown tool: "+params.Name)
			}

		case "shutdown":
			respond(req.ID, map[string]any{})
			return

		default:
			// Unknown notifications are ignored, which is what the spec asks for.
		}
	}
}
