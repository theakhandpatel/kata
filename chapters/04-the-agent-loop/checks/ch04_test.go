//go:build kata_checks

package checks

import (
	"encoding/json"
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

// Chapter 04 -- the agent loop.
//
// The program must keep going while the model asks for tools, stop when it does
// not, and never run forever. The event stream is graded as a sequence, because
// "it worked" is weaker than "it happened in this order".
func TestChapter04(t *testing.T) {
	grader.Suite(t, "04", "the agent loop", []grader.Case{
		{
			ID:    "loops-until-done",
			Title: "keeps going until the model stops asking for tools",
			Why: "One hop handles one tool. A task that needs two almost always needs " +
				"three, and a program that stops after the second result gives up " +
				"mid-task.",
			Hints: hints.Get("04", "loops-until-done"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"a.txt"}`),
					callTool("call_2", "read", `{"path":"b.txt"}`),
					fake.Reply{Text: "alpha and beta"},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read both")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "alpha and beta\n", res.Stdout); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "preserves-result-order",
			Title: "returns results in the order the model asked for them",
			Why: "Two calls in one message are one batch. Results must come back in " +
				"source order and carry their own ids, or the conversation stops " +
				"matching the calls.",
			Hints: hints.Get("04", "preserves-result-order"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_a", Name: "read"}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"path":"a.txt"}`}},
						{ToolCall: &fake.CallFrame{Index: 1, ID: "call_b", Name: "read"}},
						{ToolCall: &fake.CallFrame{Index: 1, Args: `{"path":"b.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read both").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				var got []string
				for _, m := range second.Messages {
					if m.Role == "tool" {
						got = append(got, m.ToolCallID+"="+m.Content)
					}
				}
				want := []string{"call_a=alpha", "call_b=beta"}
				if err := grader.Eqf("tool results", want, got); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "step-cap-stops",
			Title: "stops at the step limit instead of running forever",
			Why: "A model can keep asking for tools indefinitely. The cap converts an " +
				"endless run into a diagnosable failure, and the exit code has to say " +
				"the task did not finish.",
			Hints: hints.Get("04", "step-cap-stops"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha"})
				replies := make([]fake.Reply, 0, 6)
				for i := 0; i < 6; i++ {
					replies = append(replies, callTool(fmt.Sprintf("call_%d", i), "read", `{"path":"a.txt"}`))
				}
				srv := fake.New(t, replies...)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "loop", "-max-steps", "3")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				if !strings.Contains(strings.ToLower(res.Stderr), "step limit") {
					return &grader.Mismatch{Label: "stderr", Want: "a message naming the step limit", Got: res.Stderr}
				}
				return nil
			},
		},
		{
			ID:    "events-sequence",
			Title: "emits a well-formed event sequence",
			Why: "The event stream is what a UI and a log consume. Every tool call must be " +
				"answered by exactly one result with the same id, and the run must end " +
				"with a single agent_end.",
			Hints: hints.Get("04", "events-sequence"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"a.txt"}`),
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read it", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				events := parseEvents(t, res.Stderr)
				if len(events) == 0 {
					return &grader.Mismatch{Label: "stderr", Want: "JSON events", Got: res.Stderr}
				}
				if err := grader.Eqf("agent_end count", 1, countType(events, "agent_end")); err != nil {
					return err
				}
				if err := grader.Eqf("last event", "agent_end", events[len(events)-1]["type"]); err != nil {
					return err
				}

				calls := idsFor(events, "tool_call")
				results := idsFor(events, "tool_result")
				if err := grader.Eqf("tool_call ids", []string{"call_1"}, calls); err != nil {
					return err
				}
				if err := grader.Eqf("tool_result ids", calls, results); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "events-text-deltas",
			Title: "reports streamed text as it arrives",
			Why: "Collecting a reply into one message hides the stream. A terminal or a " +
				"log needs the deltas, and their sum must equal the final answer.",
			Hints: hints.Get("04", "events-text-deltas"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t, fake.Reply{Text: "The quick brown fox."})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				events := parseEvents(t, res.Stderr)
				var joined strings.Builder
				n := 0
				for _, e := range events {
					if e["type"] == "text" {
						if s, ok := e["text"].(string); ok {
							joined.WriteString(s)
							n++
						}
					}
				}
				if err := grader.Eqf("text events joined", "The quick brown fox.", joined.String()); err != nil {
					return err
				}
				if n < 2 {
					return &grader.Mismatch{Label: "text events", Want: "at least 2 deltas", Got: n}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "single-turn-no-tools",
			Title: "makes one turn when no tool is needed",
			Why: "The loop must recognise a reply without tool calls as the end. A loop " +
				"that always asks again doubles the cost and can never terminate on a " +
				"plain answer.",
			Hints: hints.Get("04", "single-turn-no-tools"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t, fake.Reply{Text: "Just text."})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				events := parseEvents(t, res.Stderr)
				if err := grader.Eqf("turn_start count", 1, countType(events, "turn_start")); err != nil {
					return err
				}
				if err := grader.Eqf("turn_end count", 1, countType(events, "turn_end")); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "continues-after-tool-error",
			Title: "keeps going after a tool fails",
			Why: "A missing file is information. Stopping the loop on the first failed " +
				"tool removes the model's chance to try another path, which is most of " +
				"what makes an agent useful.",
			Hints: hints.Get("04", "continues-after-tool-error"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"real.txt": "here"})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"missing.txt"}`),
					callTool("call_2", "read", `{"path":"real.txt"}`),
					fake.Reply{Text: "Found it on the second try."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "find it", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				events := parseEvents(t, res.Stderr)
				if err := grader.Eqf("turn_start count", 3, countType(events, "turn_start")); err != nil {
					return err
				}
				if !hasErrorResult(events) {
					return &grader.Mismatch{Label: "events", Want: "a tool_result with isError true", Got: "none"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "turn-count-matches-requests",
			Title: "reports the same turn count the server saw",
			Why: "The event stream is a claim about what happened. If it disagrees with " +
				"the request log, one of the two is lying to whoever reads it.",
			Hints: hints.Get("04", "turn-count-matches-requests"),
			Run: func(t *testing.T) error {
				dir := workspace(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t,
					callTool("call_1", "read", `{"path":"a.txt"}`),
					callTool("call_2", "read", `{"path":"a.txt"}`),
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				events := parseEvents(t, res.Stderr)
				end := lastOfType(events, "agent_end")
				if end == nil {
					return &grader.Mismatch{Label: "events", Want: "an agent_end", Got: "none"}
				}
				turns, _ := end["turns"].(float64)
				if err := grader.Eqf("agent_end turns", len(srv.Requests()), int(turns)); err != nil {
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

// parseEvents keeps only the JSON lines on stderr. The program may also write
// human diagnostics there, and a parser that assumes everything is JSON is a
// parser that breaks the first time someone adds a log line.
func parseEvents(t *testing.T, stderr string) []map[string]any {
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

func countType(events []map[string]any, typ string) int {
	n := 0
	for _, e := range events {
		if e["type"] == typ {
			n++
		}
	}
	return n
}

func idsFor(events []map[string]any, typ string) []string {
	var out []string
	for _, e := range events {
		if e["type"] != typ {
			continue
		}
		if id, ok := e["id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

func lastOfType(events []map[string]any, typ string) map[string]any {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["type"] == typ {
			return events[i]
		}
	}
	return nil
}

func hasErrorResult(events []map[string]any) bool {
	for _, e := range events {
		if e["type"] == "tool_result" && e["isError"] == true {
			return true
		}
	}
	return false
}
