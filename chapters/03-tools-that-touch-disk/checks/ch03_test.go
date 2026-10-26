//go:build kata_checks

package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 03 -- tools that touch disk.
//
// The agent gains write and bash, and every tool that can produce a lot of text
// has to bound what it returns. Each case runs the real binary in a temporary
// workspace and then looks at the disk as well as at stdout.
func TestChapter03(t *testing.T) {
	grader.Suite(t, "03", "tools that touch disk", []grader.Case{
		{
			ID:    "advertises-three-tools",
			Title: "advertises read, write and bash",
			Why: "The model can only call what the request advertises. A tool registered " +
				"but not advertised is dead code that looks alive.",
			Hints: hints.Get("03", "advertises-three-tools"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t, fake.Reply{Text: "nothing to do"})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				names := toolNames(srv.Last().Tools)
				for _, want := range []string{"read", "write", "bash"} {
					if !contains(names, want) {
						return &grader.Mismatch{Label: "tools", Want: want, Got: names}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "writes-a-file",
			Title: "writes a file the model asks for",
			Why: "The tool's effect is on disk, not in the conversation. A result that " +
				"says the write happened is not evidence that it did.",
			Hints: hints.Get("03", "writes-a-file"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callTool("call_1", "write", `{"path":"out.txt","content":"written by the agent"}`),
					fake.Reply{Text: "Done."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "write out.txt")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				onDisk, err := os.ReadFile(filepath.Join(dir, "out.txt"))
				if err != nil {
					return &grader.Mismatch{Label: "out.txt", Want: "a file on disk", Got: err.Error()}
				}
				if err := grader.Eqf("file contents", "written by the agent", string(onDisk)); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "write-creates-parents",
			Title: "creates missing parent directories",
			Why: "A model asked to add a test file will name a directory that does not " +
				"exist yet. Failing there sends it into a loop of mkdir calls it cannot " +
				"perform.",
			Hints: hints.Get("03", "write-creates-parents"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callTool("call_2", "write", `{"path":"a/b/c/deep.txt","content":"deep"}`),
					fake.Reply{Text: "Done."},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "write deep").ExitCode); err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(dir, "a/b/c/deep.txt")); err != nil {
					return &grader.Mismatch{Label: "a/b/c/deep.txt", Want: "created", Got: err.Error()}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "write-refuses-escape",
			Title: "refuses to write outside the workspace",
			Why: "A write escape is worse than a read escape: it changes the machine. " +
				"The check belongs on the resolved path, before anything is created.",
			Hints: hints.Get("03", "write-refuses-escape"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				outside := filepath.Join(filepath.Dir(dir), "escaped.txt")
				_ = os.Remove(outside)

				srv := fake.New(t,
					callTool("call_3", "write", `{"path":"../escaped.txt","content":"nope"}`),
					fake.Reply{Text: "I could not write there."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "write outside")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if _, err := os.Stat(outside); err == nil {
					return &grader.Mismatch{Label: "../escaped.txt", Want: "not created", Got: "file exists outside the workspace"}
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "escape") {
					return &grader.Mismatch{Label: "tool content", Want: "a refusal naming the escape", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "bash-runs-a-command",
			Title: "runs a command and returns its output",
			Why: "bash is the tool that turns an agent into something useful. The output " +
				"has to reach the model, and the working directory has to be the workspace.",
			Hints: hints.Get("03", "bash-runs-a-command"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"here.txt": "workspace marker"})
				srv := fake.New(t,
					callTool("call_4", "bash", `{"command":"ls && cat here.txt"}`),
					fake.Reply{Text: "I see the marker."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "list files")
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
				if !strings.Contains(toolMsg.Content, "workspace marker") {
					return &grader.Mismatch{Label: "tool content", Want: "the file contents from ls and cat", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "bash-reports-exit-code",
			Title: "reports a non-zero exit code without failing the run",
			Why: "A command that exits non-zero is information: the model asked for " +
				"something that did not work. Exiting removes its chance to react.",
			Hints: hints.Get("03", "bash-reports-exit-code"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callTool("call_5", "bash", `{"command":"echo before; exit 3"}`),
					fake.Reply{Text: "That failed."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "run it")
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
				if !strings.Contains(toolMsg.Content, "3") || !strings.Contains(toolMsg.Content, "before") {
					return &grader.Mismatch{
						Label: "tool content",
						Want:  "the exit code 3 and the output before it",
						Got:   toolMsg.Content,
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "bash-truncates-output",
			Title: "caps a command that produces too much output",
			Why: "One verbose command can consume the whole context window. The cap has " +
				"to be visible, or the model believes it saw the end of the output.",
			Hints: hints.Get("03", "bash-truncates-output"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callTool("call_6", "bash", `{"command":"yes aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa | head -n 5000"}`),
					fake.Reply{Text: "That was a lot."},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "be verbose").ExitCode); err != nil {
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
			ID:    "bash-timeout-is-a-result",
			Title: "turns a hung command into a result, not a hang",
			Why: "Without a bound, a command that never returns wedges the whole agent. " +
				"The model must receive the timeout as text so it can choose another path.",
			Hints: hints.Get("03", "bash-timeout-is-a-result"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					callTool("call_7", "bash", `{"command":"sleep 5"}`),
					fake.Reply{Text: "That took too long."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()),
					"-p", "sleep", "-timeout", "20s", "-tool-timeout", "300ms")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if res.Duration > 3*time.Second {
					return &grader.Mismatch{Label: "duration", Want: "bounded by -tool-timeout", Got: res.Duration.String()}
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := lastToolMessage(second)
				if err != nil {
					return err
				}
				if !strings.Contains(strings.ToLower(toolMsg.Content), "timed out") {
					return &grader.Mismatch{Label: "tool content", Want: "a timeout message", Got: toolMsg.Content}
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

func callTool(id, name, args string) fake.Reply {
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}

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
	return fake.RecordedMessage{}, fmt.Errorf("no tool message in the request")
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
