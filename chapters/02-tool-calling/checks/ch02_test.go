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

// Chapter 02 -- tool calling.
//
// The program must advertise a tool, receive a call, run it, and ask the model
// again with the result. Every case runs the real binary in a temporary
// workspace, so the file tools operate on real files.
func TestChapter02(t *testing.T) {
	grader.Suite(t, "02", "tool calling", []grader.Case{
		{
			ID:    "advertises-read",
			Title: "advertises the read tool on the first request",
			Why: "A model only calls what it can see. The schema has to travel with the " +
				"request, and it has to name the parameter the tool actually reads.",
			Hints: hints.Get("02", "advertises-read"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t, fake.Reply{Text: "nothing to do"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "say hi").ExitCode); err != nil {
					return err
				}

				tools := srv.Last().Tools
				if len(tools) == 0 {
					return &grader.Mismatch{Label: "tools", Want: "at least one tool definition", Got: "none"}
				}
				fn, ok := findTool(tools, "read")
				if !ok {
					return &grader.Mismatch{Label: "tools", Want: "a tool named read", Got: toolNames(tools)}
				}
				params, _ := fn["function"].(map[string]any)
				schema, _ := params["parameters"].(map[string]any)
				props, _ := schema["properties"].(map[string]any)
				if _, ok := props["path"]; !ok {
					return &grader.Mismatch{Label: "read.parameters", Want: "a path property", Got: props}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "reads-a-file",
			Title: "runs the call and sends the file back to the model",
			Why: "The second request must carry the file contents as a tool message that " +
				"names the call it answers. A result that is not paired with its call is " +
				"rejected by every real provider.",
			Hints: hints.Get("02", "reads-a-file"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"hello.txt": "hello from the workspace"})
				srv := fake.New(t,
					callRead("call_1", "read", `{"path":"hello.txt"}`),
					fake.Reply{Text: "The file says hello."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read hello.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "The file says hello.\n", res.Stdout); err != nil {
					return err
				}

				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if err := grader.Eqf("tool content", "hello from the workspace", toolMsg.Content); err != nil {
					return err
				}
				if err := grader.Eqf("tool_call_id", "call_1", toolMsg.ToolCallID); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "folds-arguments",
			Title: "reassembles arguments split across chunks",
			Why: "Argument text arrives in fragments. Continuation fragments carry only the " +
				"index, never the id or the name, so folding by index is the only reliable " +
				"way to rebuild one call.",
			Hints: hints.Get("02", "folds-arguments"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"notes.txt": "split across chunks"})
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_9", Name: "read", Args: ""}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"pa`}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `th":"no`}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `tes.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read notes")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if err := grader.Eqf("tool content", "split across chunks", toolMsg.Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-file",
			Title: "reports a missing file as a tool result, not a crash",
			Why: "A failed tool is information the model can act on. Exiting instead " +
				"removes the agent's chance to try a different path.",
			Hints: hints.Get("02", "missing-file"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callRead("call_2", "read", `{"path":"nope.txt"}`),
					fake.Reply{Text: "I could not find that file."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read nope.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "I could not find that file.\n", res.Stdout); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "no such file") {
					return &grader.Mismatch{Label: "tool content", Want: "a 'no such file' message", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "refuses-escape",
			Title: "refuses a path outside the workspace",
			Why: "The model's arguments are untrusted input. A path that climbs out of the " +
				"workspace turns a demo into a data leak.",
			Hints: hints.Get("02", "refuses-escape"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"ok.txt": "fine"})
				srv := fake.New(t,
					callRead("call_3", "read", `{"path":"../../etc/passwd"}`),
					fake.Reply{Text: "I cannot read that."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read passwd")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if strings.Contains(toolMsg.Content, "root:") {
					return &grader.Mismatch{Label: "tool content", Want: "a refusal", Got: "file contents leaked"}
				}
				if !strings.Contains(toolMsg.Content, "escape") {
					return &grader.Mismatch{Label: "tool content", Want: "a message naming the escape", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "truncates-large-file",
			Title: "truncates a large file and says so",
			Why: "Model context is the scarce resource. Pasting a large file costs the " +
				"whole budget, and cutting it silently teaches the model that it saw " +
				"everything.",
			Hints: hints.Get("02", "truncates-large-file"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"big.txt": strings.Repeat("a", 50_000)})
				srv := fake.New(t,
					callRead("call_4", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "read it"},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read big.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if len(toolMsg.Content) > 25_000 {
					return &grader.Mismatch{Label: "tool content length", Want: "at most 25000 bytes", Got: len(toolMsg.Content)}
				}
				if !strings.Contains(toolMsg.Content, "truncated") {
					return &grader.Mismatch{Label: "tool content", Want: "a truncation marker", Got: "no marker"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "invalid-arguments",
			Title: "survives arguments that are not valid JSON",
			Why: "The model produces the arguments as text, and text can be malformed. " +
				"Passing it to json.Unmarshal without a check crashes the program on a " +
				"model mistake.",
			Hints: hints.Get("02", "invalid-arguments"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_5", Name: "read", Args: `{"path":`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "That call was malformed."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "not valid JSON") {
					return &grader.Mismatch{Label: "tool content", Want: "a message about invalid JSON", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "text-only-single-request",
			Title: "makes one request when no tool is needed",
			Why: "A program that always runs a second turn doubles the cost of every " +
				"answer and hides the difference between a tool call and a plain reply.",
			Hints: hints.Get("02", "text-only-single-request"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t, fake.Reply{Text: "No tools needed."})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "say hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "No tools needed.\n", res.Stdout); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

func env(baseURL string) map[string]string {
	return map[string]string{
		"OPENAI_BASE_URL": baseURL,
		"OPENAI_API_KEY":  "test-key",
	}
}

// callRead scripts one assistant message that requests the read tool.
func callRead(id, name, args string) fake.Reply {
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}

// workspace creates a temporary directory with the given files.
func workspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func requestN(srv *fake.Server, n int) (fake.Recorded, error) {
	reqs := srv.Requests()
	if len(reqs) <= n {
		return fake.Recorded{}, fmt.Errorf("expected at least %d requests, got %d", n+1, len(reqs))
	}
	return reqs[n], nil
}

func lastToolMessage(req fake.Recorded) (fake.RecordedMessage, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "tool" {
			return req.Messages[i], nil
		}
	}
	return fake.RecordedMessage{}, fmt.Errorf("no tool message in the second request; messages were %v", roles(req.Messages))
}

func roles(msgs []fake.RecordedMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role
	}
	return out
}

func findTool(tools []map[string]any, name string) (map[string]any, bool) {
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if !ok {
			continue
		}
		if fn["name"] == name {
			return t, true
		}
	}
	return nil, false
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
