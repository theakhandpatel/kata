//go:build miyagi_checks

package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 07 -- sessions.
//
// A run stops being a single process. The program must write conversation to a
// file, read it back on the next run, and never double-count what it already
// recorded.
func TestChapter07(t *testing.T) {
	grader.Suite(t, "07", "sessions", []grader.Case{
		{
			ID:    "creates-session-file",
			Title: "creates the session file it was given",
			Why: "A session path is a promise that the conversation survives the " +
				"process. If the file is not created, nothing else in this chapter can " +
				"work.",
			Hints: hints.Get("07", "creates-session-file"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "first answer"})
				path := filepath.Join(dir, "s.jsonl")

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "first question", "-session", path)
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				info, err := os.Stat(path)
				if err != nil {
					return &grader.Mismatch{Label: "session file", Want: "created", Got: err.Error()}
				}
				if info.Size() == 0 {
					return &grader.Mismatch{Label: "session file", Want: "at least one record", Got: "empty"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "session-is-jsonl",
			Title: "writes one JSON object per line, header first",
			Why: "An append-only JSONL file survives a crash mid-write, can be read with " +
				"tail, and does not need a parser to inspect. The header carries the " +
				"version so an old file is rejected rather than misread.",
			Hints: hints.Get("07", "session-is-jsonl"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				path := filepath.Join(dir, "s.jsonl")

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "q", "-session", path).ExitCode); err != nil {
					return err
				}

				lines := readLines(t, path)
				if len(lines) < 2 {
					return &grader.Mismatch{Label: "lines", Want: "a header and at least one message", Got: len(lines)}
				}
				for i, line := range lines {
					var m map[string]any
					if err := json.Unmarshal([]byte(line), &m); err != nil {
						return &grader.Mismatch{Label: "line " + support.Itoa(i+1), Want: "valid JSON", Got: line}
					}
					if _, ok := m["type"]; !ok {
						return &grader.Mismatch{Label: "line " + support.Itoa(i+1), Want: "a type field", Got: line}
					}
				}
				var header map[string]any
				_ = json.Unmarshal([]byte(lines[0]), &header)
				if err := grader.Eqf("header type", "header", header["type"]); err != nil {
					return err
				}
				if _, ok := header["version"]; !ok {
					return &grader.Mismatch{Label: "header", Want: "a version field", Got: lines[0]}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "resumes-history",
			Title: "sends the recorded conversation on the next run",
			Why: "Resuming is the whole feature. The second run must see what the first " +
				"one said, in order, before the new question.",
			Hints: hints.Get("07", "resumes-history"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				path := filepath.Join(dir, "s.jsonl")

				first := fake.New(t, fake.Reply{Text: "first answer"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "first question", "-session", path).ExitCode); err != nil {
					return err
				}

				second := fake.New(t, fake.Reply{Text: "second answer"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "second question", "-session", path).ExitCode); err != nil {
					return err
				}

				msgs := second.Last().Messages
				if len(msgs) < 4 {
					return &grader.Mismatch{Label: "messages", Want: "system, two prior turns and the new question", Got: roleContents(msgs)}
				}
				if err := grader.Eqf("messages[0].role", "system", msgs[0].Role); err != nil {
					return err
				}
				got := roleContents(msgs[1:4])
				want := []string{
					"user|first question",
					"assistant|first answer",
					"user|second question",
				}
				if err := grader.Eqf("resumed messages", want, got); err != nil {
					return err
				}
				second.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "appends-not-overwrites",
			Title: "appends to the session instead of replacing it",
			Why: "A session that rewrites itself loses everything before the last crash. " +
				"Append-only also means an older record cannot change after the fact.",
			Hints: hints.Get("07", "appends-not-overwrites"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				path := filepath.Join(dir, "s.jsonl")

				first := fake.New(t, fake.Reply{Text: "one"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "q1", "-session", path).ExitCode); err != nil {
					return err
				}
				before := len(readLines(t, path))

				second := fake.New(t, fake.Reply{Text: "two"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "q2", "-session", path).ExitCode); err != nil {
					return err
				}
				after := len(readLines(t, path))

				if after <= before {
					return &grader.Mismatch{Label: "line count", Want: "more than " + support.Itoa(before), Got: support.Itoa(after)}
				}
				body := strings.Join(readLines(t, path), "\n")
				if n := strings.Count(body, "q1"); n != 1 {
					return &grader.Mismatch{Label: "occurrences of the first question", Want: "1", Got: support.Itoa(n)}
				}
				second.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-duplicate-turns",
			Title: "does not duplicate what the session already holds",
			Why: "The classic resume bug: load the history, then append it again. The " +
				"conversation doubles, the context window halves, and every later turn " +
				"repeats.",
			Hints: hints.Get("07", "no-duplicate-turns"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				path := filepath.Join(dir, "s.jsonl")

				first := fake.New(t, fake.Reply{Text: "unique-marker-answer"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "q1", "-session", path).ExitCode); err != nil {
					return err
				}

				second := fake.New(t, fake.Reply{Text: "two"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "q2", "-session", path).ExitCode); err != nil {
					return err
				}

				n := 0
				for _, m := range second.Last().Messages {
					if strings.Contains(m.Content, "unique-marker-answer") {
						n++
					}
				}
				if n != 1 {
					return &grader.Mismatch{Label: "occurrences of the first answer", Want: "1", Got: support.Itoa(n)}
				}
				second.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "system-not-duplicated",
			Title: "rebuilds one system message, never stores it",
			Why: "If the system message goes into the session and is also rebuilt on " +
				"resume, the model sees two. Storing conversation is enough; the prompt " +
				"is policy and belongs to the current run.",
			Hints: hints.Get("07", "system-not-duplicated"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"AGENTS.md": "project rule\n"})
				path := filepath.Join(dir, "s.jsonl")

				first := fake.New(t, fake.Reply{Text: "one"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "q1", "-session", path).ExitCode); err != nil {
					return err
				}
				second := fake.New(t, fake.Reply{Text: "two"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "q2", "-session", path).ExitCode); err != nil {
					return err
				}

				msgs := second.Last().Messages
				systems := 0
				for i, m := range msgs {
					if m.Role == "system" {
						systems++
						if i != 0 {
							return &grader.Mismatch{Label: "system position", Want: "index 0", Got: support.Itoa(i)}
						}
					}
				}
				if systems != 1 {
					return &grader.Mismatch{Label: "system messages", Want: "1", Got: support.Itoa(systems)}
				}
				if !strings.Contains(msgs[0].Content, "project rule") {
					return &grader.Mismatch{Label: "system content", Want: "the AGENTS.md body", Got: msgs[0].Content}
				}
				second.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "tool-history-persisted",
			Title: "keeps tool calls and results across runs",
			Why: "A resumed conversation with a tool result whose call is missing is " +
				"rejected by the provider. Both halves have to be recorded.",
			Hints: hints.Get("07", "tool-history-persisted"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				path := filepath.Join(dir, "s.jsonl")

				first := fake.New(t,
					fake.Reply{Frames: []fake.Frame{
						{ToolCall: &fake.CallFrame{Index: 0, ID: "call_1", Name: "read"}},
						{ToolCall: &fake.CallFrame{Index: 0, Args: `{"path":"a.txt"}`}},
						{Finish: "tool_calls"},
					}},
					fake.Reply{Text: "the file says alpha"},
				)
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "read a.txt", "-session", path).ExitCode); err != nil {
					return err
				}

				second := fake.New(t, fake.Reply{Text: "still alpha"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "and now?", "-session", path).ExitCode); err != nil {
					return err
				}

				var sawCall, sawResult bool
				for _, m := range second.Last().Messages {
					if len(m.ToolCalls) > 0 && m.ToolCalls[0].ID == "call_1" {
						sawCall = true
					}
					if m.Role == "tool" && m.ToolCallID == "call_1" && strings.Contains(m.Content, "alpha") {
						sawResult = true
					}
				}
				if !sawCall {
					return &grader.Mismatch{Label: "resumed request", Want: "the assistant tool call", Got: "missing"}
				}
				if !sawResult {
					return &grader.Mismatch{Label: "resumed request", Want: "the tool result", Got: "missing"}
				}
				second.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-session-is-created",
			Title: "starts a new session when the file does not exist",
			Why: "The first run has no file to resume. Treating a missing file as an " +
				"error makes the flag useless until someone creates it by hand.",
			Hints: hints.Get("07", "missing-session-is-created"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				path := filepath.Join(dir, "nested", "dir", "new.jsonl")

				first := fake.New(t, fake.Reply{Text: "one"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "q1", "-session", path).ExitCode); err != nil {
					return err
				}
				if _, err := os.Stat(path); err != nil {
					return &grader.Mismatch{Label: "session file", Want: "created with its parent directory", Got: err.Error()}
				}

				second := fake.New(t, fake.Reply{Text: "two"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "q2", "-session", path).ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("resumed messages", 4, len(second.Last().Messages)); err != nil {
					return err
				}
				second.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// roleContents renders messages as "role|content" for compact comparison.
func roleContents(msgs []fake.RecordedMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role + "|" + m.Content
	}
	return out
}
