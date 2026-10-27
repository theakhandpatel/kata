//go:build miyagi_checks

package checks

import (
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 06 -- prompt assembly.
//
// The program must send a system message built from a stable base and the
// project's own AGENTS.md files. Everything is graded from the request trace, so
// the internal assembly is yours.
func TestChapter06(t *testing.T) {
	grader.Suite(t, "06", "prompt assembly", []grader.Case{
		{
			ID:    "system-message-first",
			Title: "sends a system message before the user message",
			Why: "The system message is the first thing the model reads and the first " +
				"thing a provider caches. It has to exist, it has to come first, and " +
				"it has to say what the agent is.",
			Hints: hints.Get("06", "system-message-first"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				msgs := srv.Last().Messages
				if len(msgs) < 2 {
					return &grader.Mismatch{Label: "messages", Want: "a system message then a user message", Got: roles(msgs)}
				}
				if err := grader.Eqf("messages[0].role", "system", msgs[0].Role); err != nil {
					return err
				}
				if !strings.Contains(msgs[0].Content, "coding agent") {
					return &grader.Mismatch{Label: "system content", Want: "the base instruction", Got: msgs[0].Content}
				}
				if err := grader.Eqf("messages[1].role", "user", msgs[1].Role); err != nil {
					return err
				}
				if err := grader.Eqf("messages[1].content", "hi", msgs[1].Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "agents-md-included",
			Title: "includes the workspace AGENTS.md and names its path",
			Why: "Project instructions only work if the model sees them. Naming the " +
				"source lets the model reason about where a rule came from when two " +
				"files disagree.",
			Hints: hints.Get("06", "agents-md-included"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{
					"AGENTS.md": "# Rules\n\nUse tabs, not spaces.\n",
				})
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}

				system := support.SystemContent(srv.Last())
				if !strings.Contains(system, "Use tabs, not spaces.") {
					return &grader.Mismatch{Label: "system content", Want: "the AGENTS.md body", Got: system}
				}
				if !strings.Contains(system, "AGENTS.md") {
					return &grader.Mismatch{Label: "system content", Want: "the source path", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "agents-md-from-parent",
			Title: "finds an AGENTS.md above the working directory",
			Why: "A monorepo keeps one set of rules at the root. Running the agent in a " +
				"package has to inherit those rules, or they only apply from one " +
				"directory.",
			Hints: hints.Get("06", "agents-md-from-parent"),
			Run: func(t *testing.T) error {
				root := support.DirWith(t, map[string]string{
					"AGENTS.md":    "root rule: run the tests\n",
					"pkg/keep.txt": "keep",
				})
				dir := filepath.Join(root, "pkg")
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				system := support.SystemContent(srv.Last())
				if !strings.Contains(system, "root rule: run the tests") {
					return &grader.Mismatch{Label: "system content", Want: "the parent AGENTS.md", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "agents-md-order",
			Title: "puts the nearest AGENTS.md last",
			Why: "When two files give different rules, the closer one should win. Order " +
				"is how you express that without a merge algorithm.",
			Hints: hints.Get("06", "agents-md-order"),
			Run: func(t *testing.T) error {
				root := support.DirWith(t, map[string]string{
					"AGENTS.md":     "outer rule\n",
					"pkg/AGENTS.md": "inner rule\n",
				})
				dir := filepath.Join(root, "pkg")
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				system := support.SystemContent(srv.Last())
				outer := strings.Index(system, "outer rule")
				inner := strings.Index(system, "inner rule")
				if outer < 0 || inner < 0 {
					return &grader.Mismatch{Label: "system content", Want: "both files", Got: system}
				}
				if outer > inner {
					return &grader.Mismatch{Label: "order", Want: "outer before inner", Got: "inner came first"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-agents-md",
			Title: "sends the base prompt when there is no AGENTS.md",
			Why: "Discovery that assumes a file exists fails in every fresh directory. " +
				"The agent has to work in a project with no instructions at all.",
			Hints: hints.Get("06", "no-agents-md"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				system := support.SystemContent(srv.Last())
				if !strings.Contains(system, "coding agent") {
					return &grader.Mismatch{Label: "system content", Want: "the base instruction", Got: system}
				}
				if strings.Contains(system, "Project instructions") {
					return &grader.Mismatch{Label: "system content", Want: "no project section", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "empty-agents-md",
			Title: "skips an empty AGENTS.md instead of emitting an empty section",
			Why: "An empty heading reads as 'instructions were removed'. Skipping the " +
				"file keeps the prompt honest and the prefix short.",
			Hints: hints.Get("06", "empty-agents-md"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"AGENTS.md": "\n\n   \n"})
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				system := support.SystemContent(srv.Last())
				// A negative assertion alone is satisfied by a program that sends no
				// system message at all. Require the positive signal first.
				if !strings.Contains(system, "coding agent") {
					return &grader.Mismatch{Label: "system content", Want: "the base instruction", Got: system}
				}
				if strings.Contains(system, "Project instructions") {
					return &grader.Mismatch{Label: "system content", Want: "no section for an empty file", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "system-prompt-stable",
			Title: "sends the same system message on every run",
			Why: "Providers cache the request prefix. A system message containing a " +
				"timestamp, a random id, or a map iteration order loses that cache on " +
				"every call and costs real money.",
			Hints: hints.Get("06", "system-prompt-stable"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{
					"AGENTS.md": "rule one\nrule two\n",
					"b.txt":     "b",
					"a.txt":     "a",
				})

				first := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(first.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}

				second := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(second.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}

				a, b := support.SystemContent(first.Last()), support.SystemContent(second.Last())
				if !strings.Contains(a, "rule one") {
					return &grader.Mismatch{Label: "system content", Want: "the AGENTS.md body", Got: a}
				}
				if err := grader.Eqf("system content", a, b); err != nil {
					return err
				}
				return nil
			},
		},
		{
			ID:    "user-message-preserved",
			Title: "sends the user's prompt unchanged after the system message",
			Why: "Assembly must not rewrite what the user typed. A prompt that arrives " +
				"at the model with extra wrapping is a prompt the user cannot debug.",
			Hints: hints.Get("06", "user-message-preserved"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"AGENTS.md": "be brief\n"})
				srv := fake.New(t, fake.Reply{Text: "ok"})
				const prompt = `find the "todo" marker in this repo`

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", prompt).ExitCode); err != nil {
					return err
				}

				msgs := srv.Last().Messages
				if len(msgs) < 2 {
					return &grader.Mismatch{Label: "messages", Want: "system then user", Got: roles(msgs)}
				}
				if err := grader.Eqf("messages[1].content", prompt, msgs[1].Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

func roles(msgs []fake.RecordedMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role
	}
	return out
}
