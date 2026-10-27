//go:build kata_checks

package checks

import (
	"fmt"
	"strings"
	"testing"

	"kata/internal/blackbox"
	support "kata/internal/checksupport"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 12 -- subagents.
//
// A child run has its own context, its own tools and its own step cap, and the
// parent gets one result for the call. Every case reads the request trace, because
// the difference between parent and child is visible only there.
func TestChapter12(t *testing.T) {
	grader.Suite(t, "12", "subagents", []grader.Case{
		{
			ID:    "task-tool-advertised",
			Title: "advertises a task tool to the parent",
			Why: "Delegation has to be a tool, or the model has no way to ask for it. " +
				"The schema also has to say that the child cannot see the conversation, " +
				"because that changes how a prompt is written.",
			Hints: hints.Get("12", "task-tool-advertised"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				if !support.HasTool(srv.Last().Tools, "task") {
					return &grader.Mismatch{Label: "tools", Want: "a task tool", Got: support.ToolNames(srv.Last().Tools)}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "child-gets-fresh-context",
			Title: "starts the child from the task prompt alone",
			Why: "A child that inherits the parent's history doubles the context cost and " +
				"answers a conversation it was not asked about. Isolation is the whole " +
				"reason to delegate.",
			Hints: hints.Get("12", "child-gets-fresh-context"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					callTask("call_1", "count the TODO markers in this repo"),
					fake.Reply{Text: "there are 3"},
					fake.Reply{Text: "the subagent found 3"},
				)

				const parentPrompt = "PARENT-SECRET keep this out of the child"
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", parentPrompt).ExitCode); err != nil {
					return err
				}

				child, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				if err := grader.Eqf("child message count", 2, len(child.Messages)); err != nil {
					return err
				}
				if err := grader.Eqf("child messages[0].role", "system", child.Messages[0].Role); err != nil {
					return err
				}
				if err := grader.Eqf("child messages[1].role", "user", child.Messages[1].Role); err != nil {
					return err
				}
				if !strings.Contains(child.Messages[1].Content, "count the TODO markers") {
					return &grader.Mismatch{Label: "child prompt", Want: "the task text", Got: child.Messages[1].Content}
				}
				for _, m := range child.Messages {
					if strings.Contains(m.Content, "PARENT-SECRET") {
						return &grader.Mismatch{Label: "child context", Want: "no parent history", Got: m.Content}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "child-cannot-recurse",
			Title: "does not offer the task tool to a child",
			Why: "Recursion is cheap to allow and expensive to debug: two levels deep, " +
				"the request count is a tree. Leaving the tool out of the child's " +
				"registry makes it impossible rather than discouraged.",
			Hints: hints.Get("12", "child-cannot-recurse"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					callTask("call_1", "do a thing"),
					fake.Reply{Text: "done"},
					fake.Reply{Text: "the child did it"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "delegate").ExitCode); err != nil {
					return err
				}
				child, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				// Confirm this really is the child: a parent's second request also
				// lacks a task tool when the tool does not exist at all.
				if err := grader.Eqf("child message count", 2, len(child.Messages)); err != nil {
					return err
				}
				if !strings.Contains(child.Messages[1].Content, "do a thing") {
					return &grader.Mismatch{Label: "child prompt", Want: "the task text", Got: child.Messages[1].Content}
				}
				if support.HasTool(child.Tools, "task") {
					return &grader.Mismatch{Label: "child tools", Want: "no task tool", Got: support.ToolNames(child.Tools)}
				}
				// The child still needs tools, or it cannot do the work.
				if len(child.Tools) == 0 {
					return &grader.Mismatch{Label: "child tools", Want: "read, write and bash", Got: "none"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "result-returned-to-parent",
			Title: "puts the child's answer in the parent's conversation",
			Why: "A delegation that returns nothing is a very expensive way to lose " +
				"information. The answer has to arrive as the result of the call that " +
				"asked for it.",
			Hints: hints.Get("12", "result-returned-to-parent"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					callTask("call_1", "summarise the readme"),
					fake.Reply{Text: "CHILD-ANSWER: the readme explains the build"},
					fake.Reply{Text: "The summary is ready."},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "summarise it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "The summary is ready.\n", res.Stdout); err != nil {
					return err
				}
				back, err := support.Request(srv, 2)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(back, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "CHILD-ANSWER") {
					return &grader.Mismatch{Label: "task result", Want: "the child's answer", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "exactly-one-result",
			Title: "returns exactly one result for one call",
			Why: "Two results for one call is invalid, and it is what happens when a " +
				"failure path also appends a success. The parent must see one answer per " +
				"call, whatever the child did.",
			Hints: hints.Get("12", "exactly-one-result"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					callTask("call_1", "do the thing"),
					fake.Reply{Text: "done"},
					fake.Reply{Text: "all done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "delegate").ExitCode); err != nil {
					return err
				}
				back, err := support.Request(srv, 2)
				if err != nil {
					return err
				}
				// A harness with no task tool also produces exactly one result: an
				// error saying the tool is unknown. Require the tool first.
				if !support.HasTool(back.Tools, "task") {
					return &grader.Mismatch{Label: "tools", Want: "a task tool", Got: support.ToolNames(back.Tools)}
				}
				n := 0
				for _, m := range back.Messages {
					if m.Role == "tool" && m.ToolCallID == "call_1" {
						n++
						if strings.Contains(m.Content, "no such tool") {
							return &grader.Mismatch{Label: "task result", Want: "the child's answer", Got: m.Content}
						}
					}
				}
				if n != 1 {
					return &grader.Mismatch{Label: "results for call_1", Want: "1", Got: support.Itoa(n)}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "child-uses-tools",
			Title: "lets the child run tools of its own",
			Why: "A subagent that cannot read a file cannot summarise a repository. The " +
				"restriction is on delegation, not on the ability to do work.",
			Hints: hints.Get("12", "child-uses-tools"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"notes.txt": "alpha"})
				srv := fake.New(t,
					callTask("call_1", "read notes.txt and report"),
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "child_1", Name: "read"}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"path":"notes.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "the file says alpha"},
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "delegate").ExitCode); err != nil {
					return err
				}
				childSecond, err := support.Request(srv, 2)
				if err != nil {
					return err
				}
				found := false
				for _, m := range childSecond.Messages {
					if m.Role == "tool" && strings.Contains(m.Content, "alpha") {
						found = true
					}
				}
				if !found {
					return &grader.Mismatch{Label: "child conversation", Want: "the tool result", Got: "missing"}
				}
				parent, err := support.Request(srv, 3)
				if err != nil {
					return err
				}
				toolMsg, err := support.ToolResult(parent, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(toolMsg.Content, "the file says alpha") {
					return &grader.Mismatch{Label: "task result", Want: "the child's final answer", Got: toolMsg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "child-failure-is-a-result",
			Title: "turns a failed child into an error result",
			Why: "The parent is waiting on one call. A failure has to resolve that call " +
				"one way or another, or the conversation is invalid and the loop stalls.",
			Hints: hints.Get("12", "child-failure-is-a-result"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				replies := []fake.Reply{callTask("call_1", "loop forever")}
				// The child keeps calling tools until its own step cap fires.
				for i := 0; i < 12; i++ {
					replies = append(replies, fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: fmt.Sprintf("child_%d", i), Name: "read"}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"path":"a.txt"}`}},
						{Finish: "tool_calls"},
					}})
				}
				replies = append(replies, fake.Reply{Text: "the child could not finish"})
				srv := fake.New(t, replies...)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "delegate")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				back := srv.Last()
				toolMsg, err := support.ToolResult(back, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(strings.ToLower(toolMsg.Content), "step limit") {
					return &grader.Mismatch{Label: "task result", Want: "a step-limit failure", Got: toolMsg.Content}
				}
				return nil
			},
		},
		{
			ID:    "parent-continues-after-task",
			Title: "keeps the parent's run alive after the child returns",
			Why: "Delegation happens inside the loop. A parent that stops when the child " +
				"answers has thrown away the rest of the task.",
			Hints: hints.Get("12", "parent-continues-after-task"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					callTask("call_1", "do a small job"),
					fake.Reply{Text: "small job done"},
					callTask("call_2", "do another small job"),
					fake.Reply{Text: "another job done"},
					fake.Reply{Text: "both jobs are done"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "two jobs")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "both jobs are done\n", res.Stdout); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 5, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

// callTask scripts one parent turn that delegates a job.
func callTask(id, prompt string) fake.Reply {
	args := fmt.Sprintf(`{"description":"job","prompt":%q}`, prompt)
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: "task"}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}
