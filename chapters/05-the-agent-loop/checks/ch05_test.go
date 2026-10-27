//go:build miyagi_checks

package checks

import (
	"fmt"
	"strings"
	"testing"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 05 -- the agent loop.
//
// The program must keep going while the model asks for tools, stop when it does
// not, and never run forever. The event stream is graded as a sequence, because
// "it worked" is weaker than "it happened in this order".
// defaultStepCap is the number the program uses when -max-steps is absent. It is written
// here rather than imported because the checks are black box: they do not link against the
// agent package, so this number is part of the contract, not an implementation detail.
const defaultStepCap = 20

func TestChapter05(t *testing.T) {
	grader.Suite(t, "05", "the agent loop", []grader.Case{
		{
			ID:    "loops-until-done",
			Title: "keeps going until the model stops asking for tools",
			Why: "One hop handles one tool. A task that needs two almost always needs " +
				"three, and a program that stops after the second result gives up " +
				"mid-task.",
			Hints: hints.Get("05", "loops-until-done"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
				srv := fake.New(t,
					support.Call("call_1", "read", `{"path":"a.txt"}`),
					support.Call("call_2", "read", `{"path":"b.txt"}`),
					fake.Reply{Text: "alpha and beta"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read both")
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
			Hints: hints.Get("05", "preserves-result-order"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
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
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read both").ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
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
			Hints: hints.Get("05", "step-cap-stops"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				replies := make([]fake.Reply, 0, 6)
				for i := 0; i < 6; i++ {
					replies = append(replies, support.Call(fmt.Sprintf("call_%d", i), "read", `{"path":"a.txt"}`))
				}
				srv := fake.New(t, replies...)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "loop", "-max-steps", "3")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				if !support.SaysAny(res.Stderr, "step limit", "max steps", "too many turns", "turn limit") {
					return &grader.Mismatch{Label: "stderr", Want: "a message saying the run hit its step limit", Got: res.Stderr}
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
			Hints: hints.Get("05", "events-sequence"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t,
					support.Call("call_1", "read", `{"path":"a.txt"}`),
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read it", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				events := support.Events(t, res.Stderr)
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
			Hints: hints.Get("05", "events-text-deltas"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "The quick brown fox."})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				events := support.Events(t, res.Stderr)
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
			ID:    "default-cap-exists",
			Title: "stops on its own when no limit is given",
			Why: "The flag is the ceiling a learner sets. The default is the ceiling they get " +
				"when they set nothing, which is the case that protects a runaway run in the " +
				"real world.",
			Hints: hints.Get("05", "default-cap-exists"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				// A model that asks for a tool forever, and no -max-steps on the command line.
				replies := make([]fake.Reply, 0, defaultStepCap+2)
				for i := 0; i < defaultStepCap+2; i++ {
					replies = append(replies, support.Call(fmt.Sprintf("call_%d", i), "no_such_tool", `{}`))
				}
				srv := fake.New(t, replies...)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "forever")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if n := len(srv.Requests()); n > defaultStepCap {
					return &grader.Mismatch{
						Label: "requests",
						Want:  fmt.Sprintf("at most %d, the default cap", defaultStepCap),
						Got:   support.Itoa(n),
					}
				}
				if !support.SaysAny(res.Stderr, "step limit", "max steps", "too many turns", "turn limit") {
					return &grader.Mismatch{Label: "stderr", Want: "a message saying the run hit its step limit", Got: res.Stderr}
				}
				return nil
			},
		},
		{
			ID:    "single-turn-no-tools",
			Title: "makes one turn when no tool is needed",
			Why: "The loop must recognise a reply without tool calls as the end. A loop " +
				"that always asks again doubles the cost and can never terminate on a " +
				"plain answer.",
			Hints: hints.Get("05", "single-turn-no-tools"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "Just text."})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				events := support.Events(t, res.Stderr)
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
			Hints: hints.Get("05", "continues-after-tool-error"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"real.txt": "here"})
				srv := fake.New(t,
					support.Call("call_1", "read", `{"path":"missing.txt"}`),
					support.Call("call_2", "read", `{"path":"real.txt"}`),
					fake.Reply{Text: "Found it on the second try."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "find it", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				events := support.Events(t, res.Stderr)
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
			Hints: hints.Get("05", "turn-count-matches-requests"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t,
					support.Call("call_1", "read", `{"path":"a.txt"}`),
					support.Call("call_2", "read", `{"path":"a.txt"}`),
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				events := support.Events(t, res.Stderr)
				end := support.LastEventOfType(events, "agent_end")
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

// parseEvents keeps only the JSON lines on stderr. The program may also write
// human diagnostics there, and a parser that assumes everything is JSON is a
// parser that breaks the first time someone adds a log line.

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

func hasErrorResult(events []map[string]any) bool {
	for _, e := range events {
		if e["type"] == "tool_result" && e["isError"] == true {
			return true
		}
	}
	return false
}
