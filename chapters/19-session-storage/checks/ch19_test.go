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

// Chapter 19 -- session storage.
//
// Many conversations in one database: create, list, resume by id, and continue the most
// recent. The checks run the binary several times against the same store.
func TestChapter19(t *testing.T) {
	grader.Suite(t, "19", "session storage", []grader.Case{
		{
			ID:    "creates-a-session",
			Title: "creates a session on the first run",
			Why:   "A conversation needs an identity before it can be listed or resumed.",
			Hints: hints.Get("19", "creates-a-session"),
			Run: func(t *testing.T) error {
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "first answer"})

				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, "-p", "remember the branch is release").ExitCode); err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(dir, ".agent", "sessions.db")); err != nil {
					return &grader.Mismatch{Label: "database", Want: "created", Got: err.Error()}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "lists-sessions",
			Title: "lists what is stored, newest first",
			Why: "A directory of files named by hand is a puzzle. A list with a title and a " +
				"time is how a person finds the conversation they want.",
			Hints: hints.Get("19", "lists-sessions"),
			Run: func(t *testing.T) error {
				dir := workspace(t)

				first := fake.New(t, fake.Reply{Text: "one"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, first, "-p", "the first conversation").ExitCode); err != nil {
					return err
				}
				second := fake.New(t, fake.Reply{Text: "two"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, second, "-p", "a different topic entirely").ExitCode); err != nil {
					return err
				}

				out := runAgent(t, dir, fake.New(t), "-sessions").Stdout
				if !strings.Contains(out, "the first conversation") {
					return &grader.Mismatch{Label: "stdout", Want: "the first session title", Got: out}
				}
				if !strings.Contains(out, "a different topic") {
					return &grader.Mismatch{Label: "stdout", Want: "the second session title", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "title-from-first-prompt",
			Title: "names a session after its first prompt",
			Why: "A list of timestamps is unreadable. The first question is what a person " +
				"recognises their own conversation by.",
			Hints: hints.Get("19", "title-from-first-prompt"),
			Run: func(t *testing.T) error {
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				const prompt = "why does the deploy fail on fridays"

				if err := grader.Eqf("exit code", 0, runAgent(t, dir, srv, "-p", prompt).ExitCode); err != nil {
					return err
				}
				out := runAgent(t, dir, fake.New(t), "-sessions").Stdout
				if !strings.Contains(out, "why does the deploy fail") {
					return &grader.Mismatch{Label: "stdout", Want: "the prompt as a title", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "resumes-by-id",
			Title: "reopens a session by id and remembers it",
			Why: "The point of storage is returning to an old conversation and continuing " +
				"it, not starting again.",
			Hints: hints.Get("19", "resumes-by-id"),
			Run: func(t *testing.T) error {
				dir := workspace(t)

				first := fake.New(t, fake.Reply{Text: "stored answer"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, first, "-p", "remember the marker word").ExitCode); err != nil {
					return err
				}
				id := sessionIDFrom(t, dir)
				if id == "" {
					return &grader.Mismatch{Label: "session id", Want: "one session in the store", Got: "none"}
				}

				second := fake.New(t, fake.Reply{Text: "second answer"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, second, "-p", "what was the marker", "-resume", id).ExitCode); err != nil {
					return err
				}

				msgs := second.Last().Messages
				found := false
				for _, m := range msgs {
					if strings.Contains(m.Content, "remember the marker word") {
						found = true
					}
				}
				if !found {
					return &grader.Mismatch{Label: "resumed request", Want: "the earlier prompt", Got: "missing"}
				}
				return nil
			},
		},
		{
			ID:    "continue-uses-latest",
			Title: "continues the most recent session without being told which",
			Why: "Typing an id every time is the thing that makes session storage feel like " +
				"a chore. Continue is the flag that makes it useful.",
			Hints: hints.Get("19", "continue-uses-latest"),
			Run: func(t *testing.T) error {
				dir := workspace(t)

				older := fake.New(t, fake.Reply{Text: "older answer"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, older, "-p", "the older conversation").ExitCode); err != nil {
					return err
				}
				newer := fake.New(t, fake.Reply{Text: "newer answer"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, newer, "-p", "the newer conversation").ExitCode); err != nil {
					return err
				}

				last := fake.New(t, fake.Reply{Text: "continued"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, last, "-p", "and then?", "-continue").ExitCode); err != nil {
					return err
				}
				msgs := last.Last().Messages
				foundNewer, foundOlder := false, false
				for _, m := range msgs {
					if strings.Contains(m.Content, "the newer conversation") {
						foundNewer = true
					}
					if strings.Contains(m.Content, "the older conversation") {
						foundOlder = true
					}
				}
				if !foundNewer {
					return &grader.Mismatch{Label: "continued request", Want: "the most recent session", Got: "missing its prompt"}
				}
				if foundOlder {
					return &grader.Mismatch{Label: "continued request", Want: "only the most recent session", Got: "the older one leaked in"}
				}
				return nil
			},
		},
		{
			ID:    "tool-calls-round-trip",
			Title: "keeps tool calls and results across a resume",
			Why: "A stored conversation with a tool result whose call is missing is " +
				"rejected by the provider, so both halves have to survive the database.",
			Hints: hints.Get("19", "tool-calls-round-trip"),
			Run: func(t *testing.T) error {
				dir := workspaceFiles(t, map[string]string{"a.txt": "alpha"})

				first := fake.New(t,
					callTool("call_1", "read", `{"path":"a.txt"}`),
					fake.Reply{Text: "the file says alpha"},
				)
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, first, "-p", "read a.txt").ExitCode); err != nil {
					return err
				}
				id := sessionIDFrom(t, dir)
				if id == "" {
					return &grader.Mismatch{Label: "session id", Want: "present", Got: "none"}
				}

				second := fake.New(t, fake.Reply{Text: "still alpha"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, second, "-p", "and now?", "-resume", id).ExitCode); err != nil {
					return err
				}

				var sawCall, sawResult bool
				for _, m := range second.Last().Messages {
					if len(m.ToolCalls) > 0 && m.ToolCalls[0].ID == "call_1" {
						sawCall = true
					}
					if m.Role == "tool" && m.ToolCallID == "call_1" {
						sawResult = true
					}
				}
				if !sawCall || !sawResult {
					return &grader.Mismatch{
						Label: "resumed conversation",
						Want:  "the tool call and its result",
						Got:   "call=" + boolStr(sawCall) + " result=" + boolStr(sawResult),
					}
				}
				return nil
			},
		},
		{
			ID:    "two-sessions-do-not-mix",
			Title: "keeps two conversations separate",
			Why: "The failure this catches is a store with no session column, where every " +
				"conversation is one long thread and resuming either brings back both.",
			Hints: hints.Get("19", "two-sessions-do-not-mix"),
			Run: func(t *testing.T) error {
				dir := workspace(t)

				a := fake.New(t, fake.Reply{Text: "a"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, a, "-p", "topic alpha").ExitCode); err != nil {
					return err
				}
				firstID := sessionIDFrom(t, dir)

				b := fake.New(t, fake.Reply{Text: "b"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, b, "-p", "topic beta").ExitCode); err != nil {
					return err
				}

				resumeFirst := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, resumeFirst, "-p", "anyone there", "-resume", firstID).ExitCode); err != nil {
					return err
				}
				for _, m := range resumeFirst.Last().Messages {
					if strings.Contains(m.Content, "topic beta") {
						return &grader.Mismatch{Label: "resumed request", Want: "only topic alpha", Got: "topic beta leaked in"}
					}
				}
				return nil
			},
		},
		{
			ID:    "unknown-id-is-reported",
			Title: "says so when asked to resume a session that does not exist",
			Why: "Silently starting a new conversation under a mistyped id loses the thread " +
				"the person thought they were rejoining.",
			Hints: hints.Get("19", "unknown-id-is-reported"),
			Run: func(t *testing.T) error {
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "should not be reached"})

				res := runAgent(t, dir, srv, "-p", "hi", "-resume", "no-such-session")
				if res.ExitCode == 0 {
					return &grader.Mismatch{Label: "exit code", Want: "non-zero", Got: "0"}
				}
				if !strings.Contains(strings.ToLower(res.Stderr), "no such session") {
					return &grader.Mismatch{Label: "stderr", Want: "a message naming the missing session", Got: res.Stderr}
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

func runAgent(t *testing.T, dir string, srv *fake.Server, args ...string) blackbox.Result {
	t.Helper()
	return blackbox.RunIn(t, dir, env(srv.BaseURL()), args...)
}

func workspace(t *testing.T) string { return t.TempDir() }

func workspaceFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func callTool(id, name, args string) fake.Reply {
	return fake.Reply{Frames: []fake.Frame{
		{ToolCall: &fake.CallFrame{Index: 0, ID: id, Name: name}},
		{ToolCall: &fake.CallFrame{Index: 0, Args: args}},
		{Finish: "tool_calls"},
	}}
}

// sessionIDFrom reads the single session id out of the store's list output.
func sessionIDFrom(t *testing.T, dir string) string {
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

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
