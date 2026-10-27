//go:build kata_checks

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

// Chapter 20 -- what a conversation costs.
//
// Usage is recorded beside the reply that produced it, and a report answers the question
// a person actually has: what did this cost me, and which conversation spent it.
func TestChapter20(t *testing.T) {
	grader.Suite(t, "20", "what a conversation costs", []grader.Case{
		{
			ID:    "usage-is-recorded",
			Title: "records the token counts the provider reported",
			Why: "The provider sends the counts once, on the last message of the stream. If " +
				"they are not written down then, they are gone.",
			Hints: hints.Get("20", "usage-is-recorded"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{
					Text:  "the answer",
					Usage: &fake.Usage{PromptTokens: 120, CompletionTokens: 30, TotalTokens: 150},
				})

				if err := grader.Eqf("exit code", 0, runAgent(t, dir, srv, "-p", "a question").ExitCode); err != nil {
					return err
				}

				out := runAgent(t, dir, fake.New(t), "-usage").Stdout
				if !strings.Contains(out, "120") {
					return &grader.Mismatch{Label: "usage output", Want: "the prompt token count", Got: out}
				}
				if !strings.Contains(out, "30") {
					return &grader.Mismatch{Label: "usage output", Want: "the completion token count", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "usage-per-session",
			Title: "totals usage for one session on request",
			Why: "The interesting question is usually about one conversation, not the sum of " +
				"everything ever run.",
			Hints: hints.Get("20", "usage-per-session"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{
					Text:  "answer",
					Usage: &fake.Usage{PromptTokens: 200, CompletionTokens: 50, TotalTokens: 250},
				})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, srv, "-p", "only conversation").ExitCode); err != nil {
					return err
				}
				id := support.SessionIDFromStore(t, dir)
				if id == "" {
					return &grader.Mismatch{Label: "session", Want: "one stored session", Got: "none"}
				}

				out := runAgent(t, dir, fake.New(t), "-usage", "-resume", id).Stdout
				if !strings.Contains(out, "200") || !strings.Contains(out, "50") {
					return &grader.Mismatch{Label: "usage output", Want: "this session's counts", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "usage-across-sessions",
			Title: "sums every session and lists each one",
			Why: "A total with no breakdown cannot tell you which conversation to stop " +
				"having.",
			Hints: hints.Get("20", "usage-across-sessions"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)

				a := fake.New(t, fake.Reply{
					Text:  "a",
					Usage: &fake.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110},
				})
				if err := grader.Eqf("exit code", 0, runAgent(t, dir, a, "-p", "first topic").ExitCode); err != nil {
					return err
				}
				b := fake.New(t, fake.Reply{
					Text:  "b",
					Usage: &fake.Usage{PromptTokens: 400, CompletionTokens: 40, TotalTokens: 440},
				})
				if err := grader.Eqf("exit code", 0, runAgent(t, dir, b, "-p", "second topic").ExitCode); err != nil {
					return err
				}

				out := runAgent(t, dir, fake.New(t), "-usage").Stdout
				if !strings.Contains(out, "first topic") || !strings.Contains(out, "second topic") {
					return &grader.Mismatch{Label: "usage output", Want: "both session titles", Got: out}
				}
				if !strings.Contains(out, "500") {
					return &grader.Mismatch{Label: "usage output", Want: "the prompt total 500", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "cost-is-computed",
			Title: "turns tokens into money",
			Why: "Token counts are the input. The number a person cares about is what they " +
				"cost, and that needs a rate and arithmetic that does not drift.",
			Hints: hints.Get("20", "cost-is-computed"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{
					Text:  "answer",
					Usage: &fake.Usage{PromptTokens: 1000, CompletionTokens: 1000, TotalTokens: 2000},
				})
				if err := grader.Eqf("exit code", 0, runAgent(t, dir, srv, "-p", "costly").ExitCode); err != nil {
					return err
				}

				out := runAgent(t, dir, fake.New(t), "-usage").Stdout
				// Default rates: 0.15 per 1000 prompt, 0.60 per 1000 completion.
				if !strings.Contains(out, "$0.000") && !strings.Contains(out, "$") {
					return &grader.Mismatch{Label: "usage output", Want: "a money amount", Got: out}
				}
				if !strings.Contains(out, "0.000750") {
					return &grader.Mismatch{Label: "usage output", Want: "the computed cost 0.000750", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "usage-survives-a-resume",
			Title: "keeps the cost of an earlier run after resuming",
			Why: "A report that only knows about the current process is useless. The numbers " +
				"have to live with the conversation.",
			Hints: hints.Get("20", "usage-survives-a-resume"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				first := fake.New(t, fake.Reply{
					Text:  "first",
					Usage: &fake.Usage{PromptTokens: 300, CompletionTokens: 30, TotalTokens: 330},
				})
				if err := grader.Eqf("exit code", 0, runAgent(t, dir, first, "-p", "remember this").ExitCode); err != nil {
					return err
				}
				id := support.SessionIDFromStore(t, dir)

				second := fake.New(t, fake.Reply{
					Text:  "second",
					Usage: &fake.Usage{PromptTokens: 700, CompletionTokens: 70, TotalTokens: 770},
				})
				if err := grader.Eqf("exit code", 0,
					runAgent(t, dir, second, "-p", "and this", "-resume", id).ExitCode); err != nil {
					return err
				}

				out := runAgent(t, dir, fake.New(t), "-usage", "-resume", id).Stdout
				if !strings.Contains(out, "1000") {
					return &grader.Mismatch{Label: "usage output", Want: "both runs totalled at 1000 prompt tokens", Got: out}
				}
				return nil
			},
		},
		{
			ID:    "usage-needs-no-provider",
			Title: "reports usage without calling a model",
			Why:   "Asking what you spent should not cost anything.",
			Hints: hints.Get("20", "usage-needs-no-provider"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{
					Text:  "x",
					Usage: &fake.Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11},
				})
				if err := grader.Eqf("exit code", 0, runAgent(t, dir, srv, "-p", "hi").ExitCode); err != nil {
					return err
				}

				// A dead endpoint must not matter.
				e := map[string]string{"OPENAI_BASE_URL": "http://127.0.0.1:1/v1"}
				res := blackbox.RunIn(t, dir, e, "-usage")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if strings.TrimSpace(res.Stdout) == "" {
					return &grader.Mismatch{Label: "stdout", Want: "a report", Got: "empty"}
				}
				return nil
			},
		},
		{
			ID:    "empty-store-reports-nothing",
			Title: "says so when nothing has been recorded",
			Why: "Zero rows is not an error, and a report claiming zero dollars spent across " +
				"zero conversations is worse than a sentence.",
			Hints: hints.Get("20", "empty-store-reports-nothing"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				res := runAgent(t, dir, fake.New(t), "-usage")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if !strings.Contains(strings.ToLower(res.Stdout), "no usage") {
					return &grader.Mismatch{Label: "stdout", Want: "a message about having no usage", Got: res.Stdout}
				}
				return nil
			},
		},
	})
}

// --- helpers ---

func runAgent(t *testing.T, dir string, srv *fake.Server, args ...string) blackbox.Result {
	t.Helper()
	return blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), args...)
}
