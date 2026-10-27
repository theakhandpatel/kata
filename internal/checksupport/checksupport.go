// Package checksupport holds the helpers every chapter's check file needs.
//
// They were copy-pasted into twenty-one files before this package existed. The copies had
// already drifted: three versions of `workspace`, two of `env`, one that took a directory
// and one that did not. A chapter's checks should be about the chapter, not about
// re-declaring how to start a fake server.
//
// Chapter checks import this as `support`:
//
//	support.Run(t, dir, srv, "-p", "read it")
package checksupport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
)

// Env points the agent at a scripted server with a test key.
func Env(baseURL string) map[string]string {
	return map[string]string{
		"OPENAI_BASE_URL": baseURL,
		"OPENAI_API_KEY":  "test-key",
	}
}

// Run runs the agent in dir against srv.
func Run(t *testing.T, dir string, srv *fake.Server, args ...string) blackbox.Result {
	t.Helper()
	return blackbox.RunIn(t, dir, Env(srv.BaseURL()), args...)
}

// RunEnv runs the agent with extra environment variables layered over the usual ones.
func RunEnv(t *testing.T, dir string, srv *fake.Server, extra map[string]string, args ...string) blackbox.Result {
	t.Helper()
	e := Env(srv.BaseURL())
	for k, v := range extra {
		e[k] = v
	}
	return blackbox.RunIn(t, dir, e, args...)
}

// RunClean runs with HOME and XDG_CONFIG_HOME pointing at empty temporary directories, so
// the machine's real configuration cannot change the result.
func RunClean(t *testing.T, dir string, args ...string) blackbox.Result {
	t.Helper()
	home := t.TempDir()
	return blackbox.RunIn(t, dir, map[string]string{
		"OPENAI_BASE_URL": "",
		"OPENAI_API_KEY":  "",
		"HARNESS_MODEL":   "",
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
	}, args...)
}

// Dir makes an empty workspace.
func Dir(t *testing.T) string { return t.TempDir() }

// DirWith makes a workspace containing the given files. A nil map makes an empty one, which
// is why callers that used to pass nil to `workspace` keep working unchanged.
func DirWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		WriteFile(t, filepath.Join(dir, name), body)
	}
	return dir
}

// WriteFile writes a file, creating parent directories.
func WriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Call scripts one assistant turn that asks for a tool.
func Call(id, name, args string) fake.Reply {
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}

// CallSplit scripts one call whose arguments arrive in several fragments, with the id only
// on the first. Real providers stream this way, and a parser keyed on the id loses the rest.
func CallSplit(id, name, args string, parts int) fake.Reply {
	frames := []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
	}
	size := (len(args) + parts - 1) / parts
	for i := 0; i < len(args); i += size {
		end := i + size
		if end > len(args) {
			end = len(args)
		}
		frames = append(frames, fake.Frame{ToolCall: &fake.CallFrame{Index: 0, Args: args[i:end]}})
	}
	frames = append(frames, fake.Frame{Finish: "tool_calls"})
	return fake.Reply{Frames: frames}
}

// Request returns the nth request the server saw, or a mismatch naming how many arrived.
func Request(srv *fake.Server, n int) (fake.Recorded, error) {
	reqs := srv.Requests()
	if len(reqs) <= n {
		return fake.Recorded{}, &grader.Mismatch{
			Label: "requests",
			Want:  "at least " + Itoa(n+1),
			Got:   Itoa(len(reqs)),
		}
	}
	return reqs[n], nil
}

// ToolResult finds the tool message answering a call id.
func ToolResult(req fake.Recorded, id string) (fake.RecordedMessage, error) {
	for _, m := range req.Messages {
		if m.Role == "tool" && m.ToolCallID == id {
			return m, nil
		}
	}
	return fake.RecordedMessage{}, &grader.Mismatch{
		Label: "tool result " + id,
		Want:  "a message with this tool_call_id",
		Got:   "none",
	}
}

// LastToolResult returns the most recent tool message in a request, whatever its call id.
//
// Some chapters make one call and do not care which id answered it. Naming that intent here
// is better than a second helper in the chapter.
func LastToolResult(req fake.Recorded) (fake.RecordedMessage, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "tool" {
			return req.Messages[i], nil
		}
	}
	return fake.RecordedMessage{}, &grader.Mismatch{
		Label: "tool message",
		Want:  "at least one tool result in the request",
		Got:   "none",
	}
}

// ToolNames lists the tool names a request advertised.
func ToolNames(tools []map[string]any) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := fn["name"].(string); ok {
			out = append(out, name)
		}
	}
	return out
}

// HasTool reports whether a request advertised a tool by name.
func HasTool(tools []map[string]any, name string) bool {
	for _, n := range ToolNames(tools) {
		if n == name {
			return true
		}
	}
	return false
}

// SystemContent returns the system message, or "" when there is none.
func SystemContent(req fake.Recorded) string {
	for _, m := range req.Messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

// LastUser returns the most recent user message, or "".
func LastUser(req fake.Recorded) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return ""
}

// AllContent joins every message's content, for assertions that do not care which message
// carried a string.
func AllContent(req fake.Recorded) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// Events parses the JSON event lines from stderr. Lines that are not JSON are skipped, so a
// program that also logs does not break the parser.
func Events(t *testing.T, stderr string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("event line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// EventsOfType filters parsed events.
func EventsOfType(events []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

// LastEventOfType returns the last matching event, or nil.
func LastEventOfType(events []map[string]any, typ string) map[string]any {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["type"] == typ {
			return events[i]
		}
	}
	return nil
}

// SessionIDFromStore reads the single session id out of `-sessions` output.
func SessionIDFromStore(t *testing.T, dir string) string {
	t.Helper()
	res := blackbox.RunIn(t, dir, map[string]string{"OPENAI_BASE_URL": "http://127.0.0.1:1/v1"}, "-sessions")
	lines := strings.Split(res.Stdout, "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// Itoa is strconv.Itoa without the import, for messages built in a mismatch.
func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// YesNo renders a bool for a mismatch message.
func YesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// FindTool returns the advertised definition of a tool by name.
func FindTool(tools []map[string]any, name string) (map[string]any, bool) {
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if ok && fn["name"] == name {
			return t, true
		}
	}
	return nil, false
}
