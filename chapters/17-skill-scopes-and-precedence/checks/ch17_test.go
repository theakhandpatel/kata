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

// Chapter 17 -- skill scopes and precedence.
//
// Two directories can define the same skill name. Which one wins has to be decided by
// the program, not by the order the directories happen to be read.
func TestChapter17(t *testing.T) {
	grader.Suite(t, "17", "skill scopes and precedence", []grader.Case{
		{
			ID:    "project-outranks-user",
			Title: "prefers the project copy of a duplicated skill",
			Why: "A repository's skills are about that repository. A user-wide skill with " +
				"the same name is the more general one, so it should lose.",
			Hints: hints.Get("17", "project-outranks-user"),
			Run: func(t *testing.T) error {
				dir, home := scopedWorkspace(t,
					map[string]string{"deploy": "PROJECT-BODY deploy to $0"},
					map[string]string{"deploy": "USER-BODY deploy"},
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					runScoped(t, dir, home, srv, "-p", "hello").ExitCode); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				if !strings.Contains(system, "deploy") {
					return &grader.Mismatch{Label: "system content", Want: "the deploy skill", Got: system}
				}
				if !strings.Contains(system, "project") {
					return &grader.Mismatch{Label: "system content", Want: "the scope name", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "losing-copy-is-not-expanded",
			Title: "expands the winning copy, not the shadowed one",
			Why: "Naming a skill is not enough. The body that runs has to be the one the " +
				"program chose, or the precedence only exists in the catalogue.",
			Hints: hints.Get("17", "losing-copy-is-not-expanded"),
			Run: func(t *testing.T) error {
				dir, home := scopedWorkspace(t,
					map[string]string{"deploy": "PROJECT-BODY deploy to $0"},
					map[string]string{"deploy": "USER-BODY deploy"},
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					runScoped(t, dir, home, srv, "-p", "/deploy staging").ExitCode); err != nil {
					return err
				}
				user := lastUserContent(srv.Last())
				if strings.Contains(user, "USER-BODY") {
					return &grader.Mismatch{Label: "user message", Want: "the project body", Got: user}
				}
				if !strings.Contains(user, "PROJECT-BODY") {
					return &grader.Mismatch{Label: "user message", Want: "the project body", Got: user}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "both-scopes-contribute",
			Title: "advertises skills from every scope",
			Why: "Precedence only applies to collisions. A project skill and a user skill " +
				"with different names are both useful.",
			Hints: hints.Get("17", "both-scopes-contribute"),
			Run: func(t *testing.T) error {
				dir, home := scopedWorkspace(t,
					map[string]string{"deploy": "PROJECT-BODY"},
					map[string]string{"review": "USER-BODY"},
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					runScoped(t, dir, home, srv, "-p", "hello").ExitCode); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				for _, want := range []string{"deploy", "review"} {
					if !strings.Contains(system, want) {
						return &grader.Mismatch{Label: "system content", Want: want, Got: system}
					}
				}
				if !strings.Contains(system, "project") || !strings.Contains(system, "user") {
					return &grader.Mismatch{Label: "system content", Want: "both scope names", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-scope-is-skipped",
			Title: "ignores a scope whose directory does not exist",
			Why: "A person who has never written a user skill has no user directory. " +
				"That is not an error, it is the normal case.",
			Hints: hints.Get("17", "missing-scope-is-skipped"),
			Run: func(t *testing.T) error {
				dir, home := scopedWorkspace(t,
					map[string]string{"deploy": "PROJECT-BODY"},
					nil,
				)
				srv := fake.New(t, fake.Reply{Text: "still works"})

				res := runScoped(t, dir, home, srv, "-p", "hello")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "still works\n", res.Stdout); err != nil {
					return err
				}
				if !strings.Contains(systemContent(srv.Last()), "deploy") {
					return &grader.Mismatch{Label: "system content", Want: "the project skill", Got: systemContent(srv.Last())}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-scopes-at-all",
			Title: "runs with no skills anywhere",
			Why: "A new project has no skills and a new machine has no user skills. That " +
				"combination is the first run of the course.",
			Hints: hints.Get("17", "no-scopes-at-all"),
			Run: func(t *testing.T) error {
				dir := t.TempDir()
				home := t.TempDir()
				srv := fake.New(t, fake.Reply{Text: "no skills here"})

				res := runScoped(t, dir, home, srv, "-p", "hello")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				if strings.Contains(system, "# Skills") {
					return &grader.Mismatch{Label: "system content", Want: "no skills section", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "malformed-file-in-one-scope",
			Title: "skips a broken file without losing the other scope",
			Why:   "One bad file must not take the catalogue with it, in either scope.",
			Hints: hints.Get("17", "malformed-file-in-one-scope"),
			Run: func(t *testing.T) error {
				dir, home := scopedWorkspace(t,
					map[string]string{"deploy": "PROJECT-BODY"},
					map[string]string{"review": "USER-BODY"},
				)
				// A file with no frontmatter in the project scope.
				writeFile(t, filepath.Join(dir, "skills", "broken", "SKILL.md"), "no header here\n")

				srv := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					runScoped(t, dir, home, srv, "-p", "hello").ExitCode); err != nil {
					return err
				}
				system := systemContent(srv.Last())
				for _, want := range []string{"deploy", "review"} {
					if !strings.Contains(system, want) {
						return &grader.Mismatch{Label: "system content", Want: want, Got: system}
					}
				}
				if strings.Contains(system, "broken") {
					return &grader.Mismatch{Label: "system content", Want: "no entry for the broken file", Got: system}
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

// scopedWorkspace builds a project directory and a fake home directory with skills.
func scopedWorkspace(t *testing.T, project, user map[string]string) (dir, home string) {
	t.Helper()
	dir = t.TempDir()
	home = t.TempDir()
	for name, body := range project {
		writeSkill(t, filepath.Join(dir, "skills", name), body)
	}
	for name, body := range user {
		writeSkill(t, filepath.Join(home, ".config", "kata", "skills", name), body)
	}
	return dir, home
}

func writeSkill(t *testing.T, dir, body string) {
	t.Helper()
	name := filepath.Base(dir)
	content := "---\nname: " + name + "\ndescription: The " + name + " skill from this scope.\n---\n" + body + "\n"
	writeFile(t, filepath.Join(dir, "SKILL.md"), content)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// runScoped runs the agent with HOME pointed at the fake home, so the user scope is
// deterministic and never reads the machine's real skills.
func runScoped(t *testing.T, dir, home string, srv *fake.Server, args ...string) blackbox.Result {
	t.Helper()
	e := env(srv.BaseURL())
	e["HOME"] = home
	e["XDG_CONFIG_HOME"] = filepath.Join(home, ".config")
	return blackbox.RunIn(t, dir, e, args...)
}

func systemContent(req fake.Recorded) string {
	for _, m := range req.Messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

func lastUserContent(req fake.Recorded) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return ""
}
