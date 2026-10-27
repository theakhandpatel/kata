//go:build kata_checks

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 15 -- bounding a tool result.
//
// A large result is not cut, it is moved: the model gets a summary and a handle, and a
// second tool fetches a slice. The checks look at what reached the model, and at the
// file on disk.
func TestChapter15(t *testing.T) {
	grader.Suite(t, "15", "bounding a tool result", []grader.Case{
		{
			ID:    "small-result-passes-through",
			Title: "sends a small result unchanged",
			Why: "Moving a short result to disk costs a round trip and gains nothing. The " +
				"limit has to be a ceiling, not a rule that applies to everything.",
			Hints: hints.Get("15", "small-result-passes-through"),
			Run: func(t *testing.T) error {
				dir := workspaceFiles(t, map[string]string{"small.txt": "short contents"})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"small.txt"}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read it").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				if err := grader.Eqf("tool content", "short contents", msg.Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "large-result-moved",
			Title: "replaces a large result with a summary and a handle",
			Why: "Cutting a large result keeps the budget and loses the information. The " +
				"model needs to know that more exists and how to ask for it.",
			Hints: hints.Get("15", "large-result-moved"),
			Run: func(t *testing.T) error {
				big := strings.Repeat("abcdefghij", 5000) // 50 KB
				dir := workspaceFiles(t, map[string]string{"big.txt": big})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()),
						"-p", "read it", "-spill-bytes", "8000").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				if len(msg.Content) > 12_000 {
					return &grader.Mismatch{Label: "tool content length", Want: "a summary well under the original", Got: len(msg.Content)}
				}
				if !strings.Contains(msg.Content, "moved to disk") {
					return &grader.Mismatch{Label: "tool content", Want: "a notice that the content was moved", Got: msg.Content[:min(200, len(msg.Content))]}
				}
				if !strings.Contains(msg.Content, "handle:") {
					return &grader.Mismatch{Label: "tool content", Want: "a handle", Got: "none"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "result-written-to-disk",
			Title: "keeps the whole result on disk",
			Why: "A summary that cannot be followed up is a deletion with extra words. The " +
				"bytes have to be somewhere.",
			Hints: hints.Get("15", "result-written-to-disk"),
			Run: func(t *testing.T) error {
				big := strings.Repeat("0123456789", 5000)
				dir := workspaceFiles(t, map[string]string{"big.txt": big})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()),
						"-p", "read it", "-spill-bytes", "8000").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				handle := handleFrom(msg.Content)
				if handle == "" {
					return &grader.Mismatch{Label: "handle", Want: "a hex handle in the notice", Got: "none"}
				}
				path := filepath.Join(dir, ".agent-results", handle+".txt")
				onDisk, err := os.ReadFile(path)
				if err != nil {
					return &grader.Mismatch{Label: "stored result", Want: path, Got: err.Error()}
				}
				// The read tool caps its own output first, so what spills is the capped
				// content. The property being checked is that the bytes exist on disk,
				// not that they are the size of the original file.
				if len(onDisk) < 8_000 {
					return &grader.Mismatch{Label: "stored size", Want: "at least the spill limit", Got: len(onDisk)}
				}
				if !strings.HasPrefix(string(onDisk), "0123456789") {
					return &grader.Mismatch{Label: "stored content", Want: "the beginning of the result", Got: string(onDisk[:20])}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "read-result-fetches-a-slice",
			Title: "fetches a slice of a stored result",
			Why: "The handle is only useful if a second call returns the bytes. That call " +
				"is a tool like any other, and it has to respect its own limits.",
			Hints: hints.Get("15", "read-result-fetches-a-slice"),
			Run: func(t *testing.T) error {
				big := strings.Repeat("0123456789", 5000)
				dir := workspaceFiles(t, map[string]string{"big.txt": big})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()),
						"-p", "read it", "-spill-bytes", "8000").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				handle := handleFrom(msg.Content)
				if handle == "" {
					return &grader.Mismatch{Label: "handle", Want: "a handle in the notice", Got: "none"}
				}
				if !hasTool(srv.Last().Tools, "read_result") {
					return &grader.Mismatch{Label: "tools", Want: "read_result advertised", Got: toolNames(srv.Last().Tools)}
				}

				// A second run fetches the same slice the summary pointed at.
				srv2 := fake.New(t,
					callTool("call_2", "read_result", `{"handle":"`+handle+`","offset":0,"length":100}`),
					fake.Reply{Text: "done"},
				)
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv2.BaseURL()), "-p", "fetch", "-spill-bytes", "8000").ExitCode); err != nil {
					return err
				}
				after, err := requestN(srv2, 1)
				if err != nil {
					return err
				}
				slice, err := toolByID(after, "call_2")
				if err != nil {
					return err
				}
				if !strings.Contains(slice.Content, "bytes 0-100") {
					return &grader.Mismatch{Label: "result", Want: "a note naming the range", Got: slice.Content}
				}
				if !strings.Contains(slice.Content, strings.Repeat("0123456789", 10)) {
					return &grader.Mismatch{Label: "result", Want: "the first 100 bytes", Got: slice.Content[:min(120, len(slice.Content))]}
				}
				srv.AssertConsumed(t)
				srv2.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "unknown-handle-is-a-result",
			Title: "answers an unknown handle instead of failing",
			Why: "The handle is model input and models mistype. A missing file is a result " +
				"the model can read, not a crash.",
			Hints: hints.Get("15", "unknown-handle-is-a-result"),
			Run: func(t *testing.T) error {
				dir := workspaceFiles(t, nil)
				srv := fake.New(t,
					callTool("call_1", "read_result", `{"handle":"deadbeef","offset":0}`),
					fake.Reply{Text: "I could not find that result."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "fetch it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(msg.Content, "no result stored") {
					return &grader.Mismatch{Label: "tool content", Want: "a message about the missing handle", Got: msg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "offset-past-end-is-a-result",
			Title: "answers an offset past the end",
			Why: "A model that guesses an offset will overshoot. It needs to learn the " +
				"size, not to bring the program down.",
			Hints: hints.Get("15", "offset-past-end-is-a-result"),
			Run: func(t *testing.T) error {
				big := strings.Repeat("x", 50_000)
				dir := workspaceFiles(t, map[string]string{"big.txt": big})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"big.txt"}`),
					fake.Reply{Text: "done"},
				)
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()),
						"-p", "read", "-spill-bytes", "8000").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				handle := handleFrom(msg.Content)
				if handle == "" {
					return &grader.Mismatch{Label: "handle", Want: "present", Got: "none"}
				}

				srv2 := fake.New(t,
					callTool("call_2", "read_result", `{"handle":"`+handle+`","offset":999999}`),
					fake.Reply{Text: "that offset is past the end"},
				)
				res := blackbox.RunIn(t, dir, env(srv2.BaseURL()), "-p", "fetch", "-spill-bytes", "8000")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				after, err := requestN(srv2, 1)
				if err != nil {
					return err
				}
				slice, err := toolByID(after, "call_2")
				if err != nil {
					return err
				}
				if !strings.Contains(slice.Content, "past the end") {
					return &grader.Mismatch{Label: "result", Want: "a message naming the overshoot", Got: slice.Content}
				}
				srv.AssertConsumed(t)
				srv2.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "handle-is-not-a-path",
			Title: "refuses a handle that is a path",
			Why: "The handle arrives from the model, so it is untrusted. A handle of " +
				"../../etc/passwd would read a file that has nothing to do with results.",
			Hints: hints.Get("15", "handle-is-not-a-path"),
			Run: func(t *testing.T) error {
				dir := workspaceFiles(t, nil)
				srv := fake.New(t,
					callTool("call_1", "read_result", `{"handle":"../../../etc/passwd"}`),
					fake.Reply{Text: "no"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "fetch").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				msg, err := toolByID(second, "call_1")
				if err != nil {
					return err
				}
				if strings.Contains(msg.Content, "root:") {
					return &grader.Mismatch{Label: "tool content", Want: "a refusal", Got: "file contents leaked"}
				}
				if !strings.Contains(strings.ToLower(msg.Content), "invalid handle") {
					return &grader.Mismatch{Label: "tool content", Want: "an invalid handle message", Got: msg.Content}
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

func workspaceFiles(t *testing.T, files map[string]string) string {
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
		return fake.Recorded{}, &grader.Mismatch{Label: "requests", Want: "at least one more", Got: itoa(len(reqs))}
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

func hasTool(tools []map[string]any, name string) bool {
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]any); ok && fn["name"] == name {
			return true
		}
	}
	return false
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

// handleFrom pulls the hex handle out of the notice the tool wrote.
func handleFrom(content string) string {
	const marker = "handle: "
	i := strings.Index(content, marker)
	if i < 0 {
		return ""
	}
	rest := content[i+len(marker):]
	end := strings.IndexAny(rest, "\n ")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
