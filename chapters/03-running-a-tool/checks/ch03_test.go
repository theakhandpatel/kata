//go:build kata_checks

package checks

import (
	"strings"
	"testing"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 03 -- running a tool.
//
// Chapter 02 taught the conversation. This chapter puts a real tool behind it: the
// model asks to read a file, the program reads it, and the contents go back as the
// answer to that specific call. The tool itself is small. The edges are the lesson:
// missing files, paths outside the workspace, and files too large to send.
func TestChapter02(t *testing.T) {
	grader.Suite(t, "03", "running a tool", []grader.Case{
		{
			ID:    "advertises-read",
			Title: "advertises the read tool and its path argument",
			Why: "A model only calls what it can see. The schema has to travel with the " +
				"request, and it has to name the argument the tool actually reads.",
			Hints: hints.Get("03", "advertises-read"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t, fake.Reply{Text: "nothing to do"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "say hi").ExitCode); err != nil {
					return err
				}
				fn, ok := support.FindTool(srv.Last().Tools, "read")
				if !ok {
					return &grader.Mismatch{Label: "tools", Want: "a tool named read", Got: support.ToolNames(srv.Last().Tools)}
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
			Hints: hints.Get("03", "reads-a-file"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"hello.txt": "hello from the workspace"})
				srv := fake.New(t,
					support.Call("call_1", "read", `{"path":"hello.txt"}`),
					fake.Reply{Text: "The file says hello."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read hello.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "The file says hello.\n", res.Stdout); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(second, "call_1")
				if err != nil {
					return err
				}
				if err := grader.Eqf("tool content", "hello from the workspace", toolMsg.Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-file",
			Title: "reports a missing file as a result, not a crash",
			Why: "A failed tool is information the model can act on. Exiting instead " +
				"removes its chance to try a different path.",
			Hints: hints.Get("03", "missing-file"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, nil)
				srv := fake.New(t,
					support.Call("call_2", "read", `{"path":"nope.txt"}`),
					fake.Reply{Text: "I could not find that file."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read nope.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(second, "call_2")
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
			Why: "The model's arguments are untrusted input. A path that climbs out of " +
				"the workspace turns a demo into a data leak.",
			Hints: hints.Get("03", "refuses-escape"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"ok.txt": "fine"})
				srv := fake.New(t,
					support.Call("call_3", "read", `{"path":"../../etc/passwd"}`),
					fake.Reply{Text: "I cannot read that."},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read passwd").ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(second, "call_3")
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
			Hints: hints.Get("03", "truncates-large-file"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"big.txt": strings.Repeat("a", 50_000)})
				srv := fake.New(t,
					support.Call("call_4", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "read it"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read big.txt").ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(second, "call_4")
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
			ID:    "text-only-single-request",
			Title: "makes one request when no tool is needed",
			Why: "A program that always runs a second turn doubles the cost of every " +
				"answer and hides the difference between a tool call and a plain reply.",
			Hints: hints.Get("03", "text-only-single-request"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t, fake.Reply{Text: "No tools needed."})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "say hi")
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
