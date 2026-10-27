//go:build kata_checks

package checks

import (
	"strings"
	"testing"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
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
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hi")
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
			ID:    "unknown-tool-is-a-result",
			Title: "answers a call for a tool it does not have",
			Why: "The model can name a tool that does not exist, and it will. If the " +
				"program exits, the conversation is over and the model never learns what " +
				"went wrong. The answer has to be a result it can read.",
			Hints: hints.Get("02", "unknown-tool-is-a-result"),
			Run: func(t *testing.T) error {
				dir := workspace(t)
				srv := fake.New(t,
					callTool("call_1", "no_such_tool", `{"path":"a.txt"}`),
					fake.Reply{Text: "I could not do that."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "do it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "I could not do that.\n", res.Stdout); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(second, "call_1")
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
				dir := workspace(t)
				srv := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_1", Name: "read", Args: `{"path":`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "That call was malformed."},
				)

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "do it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(second, "call_1")
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
				dir := workspace(t)
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

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "read notes")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				toolMsg, err := toolByID(second, "call_9")
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
				dir := workspace(t)
				srv := fake.New(t,
					callTool("call_abc", "no_such_tool", `{}`),
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}
				second, err := requestN(srv, 1)
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
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}
				// Whatever is advertised, a call to it must resolve.
				for _, name := range toolNames(srv.Last().Tools) {
					reply := fake.New(t,
						callTool("call_x", name, `{}`),
						fake.Reply{Text: "done"},
					)
					if err := grader.Eqf("exit code", 0,
						blackbox.RunIn(t, dir, env(reply.BaseURL()), "-p", "x").ExitCode); err != nil {
						return err
					}
					second, err := requestN(reply, 1)
					if err != nil {
						return err
					}
					toolMsg, err := toolByID(second, "call_x")
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

func workspace(t *testing.T) string { return t.TempDir() }

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
