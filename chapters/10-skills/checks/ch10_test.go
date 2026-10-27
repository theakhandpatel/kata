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

// Chapter 09 -- the skill catalogue.
//
// A skill is a file. This chapter teaches finding those files and advertising them,
// and deliberately stops before anything is invoked. Invocation is chapter 10, so the
// two halves can be finished in two sittings.
func TestChapter10(t *testing.T) {
	grader.Suite(t, "10", "the skill catalogue", []grader.Case{
		{
			ID:    "advertises-skills",
			Title: "lists skill names and descriptions in the system message",
			Why: "A model cannot invoke what it has not been told about. The catalogue " +
				"carries each skill's name and what it is for, and nothing else.",
			Hints: hints.Get("10", "advertises-skills"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
					skill("review", "Review the current diff.", reviewBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hello").ExitCode); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				for _, want := range []string{"deploy", "Deploy the app to a region.", "review", "Review the current diff."} {
					if !strings.Contains(system, want) {
						return &grader.Mismatch{Label: "system content", Want: want, Got: system}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "body-not-advertised",
			Title: "advertises descriptions without sending bodies",
			Why: "The catalogue staying small is the whole reason it exists. A project " +
				"with fifty skills should cost fifty lines on every request, not fifty " +
				"documents.",
			Hints: hints.Get("10", "body-not-advertised"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hello").ExitCode); err != nil {
					return err
				}
				// A program with no skills satisfies the negative half for free, so
				// require the advertisement before asserting the absence.
				if !strings.Contains(systemContent(srv.Last()), "Deploy the app to a region.") {
					return &grader.Mismatch{Label: "system content", Want: "the skill description", Got: systemContent(srv.Last())}
				}
				if strings.Contains(allContent(srv.Last()), "BODY-DEPLOY") {
					return &grader.Mismatch{Label: "request content", Want: "no skill body", Got: "body present"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "malformed-skill-skipped",
			Title: "skips a SKILL.md it cannot parse",
			Why: "One broken file must not stop the agent from starting. A skill with no " +
				"name and no description cannot be advertised, so there is nothing useful " +
				"to do with it except leave it out.",
			Hints: hints.Get("10", "malformed-skill-skipped"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				// No frontmatter: this file must be ignored, not fatal.
				writeFile(t, filepath.Join(dir, "skills", "broken", "SKILL.md"),
					"BODY-BROKEN just markdown with no header\n")

				srv := fake.New(t, fake.Reply{Text: "still works"})
				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hello")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "still works\n", res.Stdout); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				if !strings.Contains(system, "deploy") {
					return &grader.Mismatch{Label: "system content", Want: "the valid skill", Got: system}
				}
				if strings.Contains(system, "broken") {
					return &grader.Mismatch{Label: "system content", Want: "no entry for the broken file", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-skills-dir",
			Title: "runs normally when there is no skills directory",
			Why: "Most projects have no skills. A catalogue that treats a missing " +
				"directory as an error makes the agent refuse to work in an empty folder.",
			Hints: hints.Get("10", "missing-skills-dir"),
			Run: func(t *testing.T) error {
				dir := workspace(t)
				srv := fake.New(t, fake.Reply{Text: "no skills here"})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "hello")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "no skills here\n", res.Stdout); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				if !strings.Contains(system, "coding agent") {
					return &grader.Mismatch{Label: "system content", Want: "the base instruction", Got: system}
				}
				if strings.Contains(system, "# Skills") {
					return &grader.Mismatch{Label: "system content", Want: "no skills section", Got: system}
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

const (
	deployBody = "BODY-DEPLOY: Deploy to $0 in $1. Full request was: $ARGUMENTS"
	reviewBody = "BODY-REVIEW: Read the diff and list risks."
)

func skill(name, description, body string) struct{ Name, Description, Body string } {
	return struct{ Name, Description, Body string }{name, description, body}
}

func skillsWorkspace(t *testing.T, skills ...struct{ Name, Description, Body string }) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range skills {
		full := filepath.Join(dir, "skills", s.Name, "SKILL.md")
		content := "---\nname: " + s.Name + "\ndescription: " + s.Description + "\n---\n" + s.Body + "\n"
		writeFile(t, full, content)
	}
	return dir
}

func workspace(t *testing.T) string { return t.TempDir() }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func systemContent(req fake.Recorded) string {
	for _, m := range req.Messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

func allContent(req fake.Recorded) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}
