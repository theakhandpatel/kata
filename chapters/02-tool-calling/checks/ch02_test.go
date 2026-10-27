//go:build miyagi_checks

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

// Chapter 02 -- tool calling: the protocol.
//
// The model can now ask for a tool, and the program can answer the request without
// crashing. This chapter is only about the conversation: what a call looks like on the
// wire, how its pieces are grouped, and what happens when the model asks for something
// that does not exist. Chapter 03 puts a real tool behind it.
func TestChapter02(t *testing.T) {
	grader.Suite(t, "02", "tool calling", []grader.Case{
		{
			ID:    "advertises-tools",
			Title: "sends a tools field even when it is empty",
			Why: "The request shape is the protocol. A server reads the list of tools to " +
				"decide what the model may ask for. Getting the field name or position " +
				"wrong means the model never sees any tool at all.",
			Hints: hints.Get("02", "advertises-tools"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				raw := srv.Last().Raw
				if _, present := raw["tools"]; present {
					// An empty list is fine; a wrong type is not.
					if _, ok := raw["tools"].([]any); !ok {
						return &grader.Mismatch{Label: "tools", Want: "an array", Got: raw["tools"]}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "two-calls-in-one-message",
			Title: "keeps two calls apart when they arrive together",
			Why: "One assistant message can ask for several tools. The fragments of the " +
				"second call carry index 1 and no id, so a parser that keys everything on index " +
				"zero merges the two arguments into one and loses both.",
			Hints: hints.Get("02", "two-calls-in-one-message"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_a", Name: "no_such_tool", Args: `{"path":`}},
						{ToolCall: &fake.CallFrame{Index: 1, ID: "call_b", Name: "no_such_tool", Args: `{"path":`}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `"a.txt"}`}},
						{ToolCall: &fake.CallFrame{Index: 1, Args: `"b.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read both")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				// Both calls must reach the model, each with its own arguments intact.
				for _, want := range []struct{ id, args string }{
					{"call_a", `{"path":"a.txt"}`},
					{"call_b", `{"path":"b.txt"}`},
				} {
					found := false
					for _, m := range second.Messages {
						if len(m.ToolCalls) == 0 {
							continue
						}
						for _, tc := range m.ToolCalls {
							if tc.ID == want.id && tc.Function.Arguments == want.args {
								found = true
							}
						}
					}
					if !found {
						return &grader.Mismatch{
							Label: "call " + want.id,
							Want:  "arguments " + want.args + " in the follow-up request",
							Got:   "missing or merged",
						}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "unknown-tool-is-a-result",
			Title: "answers a call for a tool it does not have",
			Why: "The model can name a tool that does not exist, and it will. If the " +
				"program exits, the conversation is over and the model never learns what " +
				"went wrong. The answer has to be a result it can read.",
			Hints: hints.Get("02", "unknown-tool-is-a-result"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					support.Call("call_1", "no_such_tool", `{"path":"a.txt"}`),
					fake.Reply{Text: "I could not do that."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "do it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "I could not do that.\n", res.Stdout); err != nil {
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
				if !strings.Contains(toolMsg.Content, "no such tool") {
					return &grader.Mismatch{Label: "tool result", Want: "a message naming the unknown tool", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "invalid-arguments",
			Title: "answers a call whose arguments are not JSON",
			Why: "The arguments are text the model wrote, so they can be malformed. " +
				"Passing them straight to a decoder crashes the program on a mistake the " +
				"model can fix if you tell it.",
			Hints: hints.Get("02", "invalid-arguments"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_1", Name: "read", Args: `{"path":`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "That call was malformed."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "do it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
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
				if !strings.Contains(toolMsg.Content, "not valid JSON") {
					return &grader.Mismatch{Label: "tool result", Want: "a message about invalid JSON", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "folds-arguments",
			Title: "reassembles arguments split across several messages",
			Why: "Argument text arrives in pieces. The first piece names the call; the " +
				"rest carry only the group number. A parser that keys on the id loses " +
				"everything after the first piece.",
			Hints: hints.Get("02", "folds-arguments"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_9", Name: "no_such_tool", Args: ""}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"pa`}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `th":"no`}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `tes.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "done"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "read notes")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(second, "call_9")
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "no such tool") {
					return &grader.Mismatch{Label: "tool result", Want: "a result for call_9", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "result-carries-call-id",
			Title: "pairs every result with the call it answers",
			Why: "A result without its call id is a conversation a provider rejects. The " +
				"id is the only thing tying the two halves together.",
			Hints: hints.Get("02", "result-carries-call-id"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					support.Call("call_abc", "no_such_tool", `{}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				found := false
				for _, m := range second.Messages {
					if m.Role == "tool" && m.ToolCallID == "call_abc" {
						found = true
					}
				}
				if !found {
					return &grader.Mismatch{Label: "tool message", Want: "tool_call_id call_abc", Got: "none"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "tool-list-comes-from-the-registry",
			Title: "builds the tool list from what is registered",
			Why: "A hardcoded schema advertises tools nothing can run, and the model " +
				"wastes a turn on them. The list has to come from the registry, so it " +
				"matches what exists.",
			Hints: hints.Get("02", "tool-list-comes-from-the-registry"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}
				// Whatever is advertised, a call to it must resolve.
				for _, name := range support.ToolNames(srv.Last().Tools) {
					reply := fake.New(t,
						support.Call("call_x", name, `{}`),
						fake.Reply{Text: "done"},
					)
					if err := grader.Eqf("exit code", 0,
						blackbox.RunIn(t, dir, support.Env(reply.BaseURL()), "-p", "x").ExitCode); err != nil {
						return err
					}
					second, err := support.Request(reply, 1)
					if err != nil {
						return err
					}
					toolMsg, err := support.ToolResult(second, "call_x")
					if err != nil {
						return err
					}
					if strings.Contains(toolMsg.Content, "no such tool") {
						return &grader.Mismatch{
							Label: "advertised tool " + name,
							Want:  "a tool that exists in the registry",
							Got:   toolMsg.Content,
						}
					}
				}
				return nil
			},
		},
	})
}

// --- helpers ---
