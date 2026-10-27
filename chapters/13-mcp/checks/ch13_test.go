//go:build kata_checks

package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 13 -- MCP over stdio.
//
// The agent spawns a server, shakes hands, and offers the server's tools. The
// scripted server records every method it receives, so the handshake and the call
// sequence are graded directly.
func TestChapter13(t *testing.T) {
	grader.Suite(t, "13", "MCP over stdio", []grader.Case{
		{
			ID:    "mcp-tools-advertised",
			Title: "advertises the server's tools next to the built-in ones",
			Why: "A tool the model cannot see is a tool it will never call. The server's " +
				"list has to be merged into the registry the loop already sends.",
			Hints: hints.Get("13", "mcp-tools-advertised"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := runAgent(t, dir, srv, log, "-p", "hi", "-mcp", mcpserver(t))
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				names := toolNames(srv.Last().Tools)
				for _, want := range []string{"read", "write", "bash", "task", "demo.echo", "demo.read"} {
					if !contains(names, want) {
						return &grader.Mismatch{Label: "tools", Want: want, Got: names}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-initialize-handshake",
			Title: "initializes before listing tools",
			Why: "MCP requires initialize, then the initialized notification, before any " +
				"other request. Skipping it usually works against a lenient server and " +
				"fails against a strict one.",
			Hints: hints.Get("13", "mcp-initialize-handshake"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, log, "-p", "hi", "-mcp", mcpserver(t)).ExitCode); err != nil {
					return err
				}
				got := logLines(t, log)
				want := []string{"initialize", "notifications/initialized", "tools/list"}
				if len(got) < len(want) {
					return &grader.Mismatch{Label: "server methods", Want: strings.Join(want, ", "), Got: strings.Join(got, ", ")}
				}
				for i := range want {
					if got[i] != want[i] {
						return &grader.Mismatch{Label: "method " + itoa(i+1), Want: want[i], Got: got[i]}
					}
				}
				return nil
			},
		},
		{
			ID:    "mcp-tool-call",
			Title: "runs a call through the server and returns its text",
			Why: "The adapter is only worth having if a call reaches the server and the " +
				"answer reaches the model. Both halves cross a process boundary, so " +
				"either can be silently dropped.",
			Hints: hints.Get("13", "mcp-tool-call"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t,
					callTool("call_1", "demo.echo", `{"text":"hello"}`),
					fake.Reply{Text: "the server said hello"},
				)

				res := runAgent(t, dir, srv, log, "-p", "echo hello", "-mcp", mcpserver(t))
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if !contains(logLines(t, log), "tools/call") {
					return &grader.Mismatch{Label: "server methods", Want: "tools/call", Got: logLines(t, log)}
				}
				back, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(back, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "echo: hello") {
					return &grader.Mismatch{Label: "tool result", Want: "the server's text", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-content-blocks-joined",
			Title: "joins multiple content blocks into one result",
			Why: "MCP returns content as a list of blocks. Taking only the first loses " +
				"most of the answer, and taking only text blocks correctly ignores " +
				"images the model cannot use here.",
			Hints: hints.Get("13", "mcp-content-blocks-joined"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t,
					callTool("call_1", "demo.echo", `{"text":"two blocks"}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, log, "-p", "echo", "-mcp", mcpserver(t)).ExitCode); err != nil {
					return err
				}
				back, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(back, "call_1")
				if err != nil {
					return err
				}
				for _, want := range []string{"echo: two blocks", "(second block)"} {
					if !strings.Contains(toolMsg.Content, want) {
						return &grader.Mismatch{Label: "tool result", Want: want, Got: toolMsg.Content}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-name-collision",
			Title: "namespaces a server tool that shares a built-in name",
			Why: "This server exposes read. If names are not namespaced, the server " +
				"silently replaces the built-in, and a file the model thought it read " +
				"from disk came from somewhere else.",
			Hints: hints.Get("13", "mcp-name-collision"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t,
					callTool("call_1", "demo.read", `{"path":"a.txt"}`),
					fake.Reply{Text: "done"},
				)
				writeFile(t, filepath.Join(dir, "a.txt"), "local-file-contents")

				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, log, "-p", "read it", "-mcp", mcpserver(t)).ExitCode); err != nil {
					return err
				}
				names := toolNames(srv.Last().Tools)
				if !contains(names, "read") || !contains(names, "demo.read") {
					return &grader.Mismatch{Label: "tools", Want: "both read and demo.read", Got: names}
				}
				back, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(back, "call_1")
				if err != nil {
					return err
				}
				if strings.Contains(toolMsg.Content, "local-file-contents") {
					return &grader.Mismatch{Label: "tool result", Want: "the server's answer", Got: "the built-in read answered"}
				}
				if !strings.Contains(toolMsg.Content, "mcp-read-content") {
					return &grader.Mismatch{Label: "tool result", Want: "mcp-read-content", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-error-is-a-result",
			Title: "turns a JSON-RPC error into an error result",
			Why: "A server that refuses a call has not broken the agent. The model needs " +
				"to read the refusal and try something else.",
			Hints: hints.Get("13", "mcp-error-is-a-result"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t,
					callTool("call_1", "demo.echo", `{"text":"hi"}`),
					fake.Reply{Text: "that did not work"},
				)

				res := runAgentEnv(t, dir, srv, log, map[string]string{"MCP_FAIL": "1"},
					"-p", "echo", "-mcp", mcpserver(t))
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				back, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(back, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "refused") {
					return &grader.Mismatch{Label: "tool result", Want: "the server's refusal", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-unavailable-is-survivable",
			Title: "keeps running when the server cannot start",
			Why: "An optional extension that fails must not take the agent with it. The " +
				"person asked a question, not for a working plugin.",
			Hints: hints.Get("13", "mcp-unavailable-is-survivable"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t, fake.Reply{Text: "answered without extensions"})

				res := runAgent(t, dir, srv, log, "-p", "hi", "-mcp", "/nonexistent/mcp-server")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "answered without extensions\n", res.Stdout); err != nil {
					return err
				}
				for _, name := range toolNames(srv.Last().Tools) {
					if strings.Contains(name, ".") {
						return &grader.Mismatch{Label: "tools", Want: "no MCP tools", Got: name}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "mcp-no-call-when-unused",
			Title: "lists tools without calling any",
			Why: "Discovery is not usage. A program that calls every tool to see what it " +
				"does pays real money for a catalogue entry.",
			Hints: hints.Get("13", "mcp-no-call-when-unused"),
			Run: func(t *testing.T) error {
				dir, log := workspace(t), logPath(t)
				srv := fake.New(t, fake.Reply{Text: "no tools needed"})

				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, log, "-p", "just answer", "-mcp", mcpserver(t)).ExitCode); err != nil {
					return err
				}
				for _, method := range logLines(t, log) {
					if method == "tools/call" {
						return &grader.Mismatch{Label: "server methods", Want: "no tools/call", Got: strings.Join(logLines(t, log), ", ")}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

func mcpserver(t *testing.T) string { return blackbox.MCPServer(t) }

func runAgent(t *testing.T, dir string, srv *fake.Server, log string, args ...string) blackbox.Result {
	t.Helper()
	return runAgentEnv(t, dir, srv, log, nil, args...)
}

func runAgentEnv(t *testing.T, dir string, srv *fake.Server, log string, extra map[string]string, args ...string) blackbox.Result {
	t.Helper()
	env := map[string]string{
		"OPENAI_BASE_URL": srv.BaseURL(),
		"OPENAI_API_KEY":  "test-key",
		"MCP_LOG":         log,
	}
	for k, v := range extra {
		env[k] = v
	}
	return blackbox.RunIn(t, dir, env, args...)
}

func workspace(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func logPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mcp.log")
}

func logLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func callTool(id, name, args string) fake.Reply {
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}

func requestN(srv *fake.Server, n int) (fake.Recorded, error) {
	reqs := srv.Requests()
	if len(reqs) <= n {
		return fake.Recorded{}, &grader.Mismatch{Label: "requests", Want: "at least " + itoa(n+1), Got: itoa(len(reqs))}
	}
	return reqs[n], nil
}

func toolByID(req fake.Recorded, id string) (fake.RecordedMessage, error) {
	for _, m := range req.Messages {
		if m.Role == "tool" && m.ToolCallID == id {
			return m, nil
		}
	}
	return fake.RecordedMessage{}, &grader.Mismatch{Label: "tool result " + id, Want: "present", Got: "missing"}
}

func toolNames(tools []map[string]any) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
