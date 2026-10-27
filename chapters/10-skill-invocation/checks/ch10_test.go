//go:build kata_checks

package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

const (
	deployBody = "BODY-DEPLOY: Deploy to $0 in $1. Full request was: $ARGUMENTS"
	reviewBody = "BODY-REVIEW: Read the diff and list risks."
)

// Chapter 10 -- invoking a skill.
//
// The catalogue from chapter 09 is already in the system message. This chapter turns
// `/name args` into the skill's body, substitutes the arguments, and handles stacking.
func TestChapter09(t *testing.T) {
	grader.Suite(t, "10", "invoking a skill", []grader.Case{
		{
			ID:    "loads-body-on-invoke",
			Title: "loads the body when the skill is invoked",
			Why: "Invocation is the moment the body earns its place in the request. " +
				"Without it the model has a name and nothing to do with it.",
			Hints: hints.Get("09", "loads-body-on-invoke"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "/deploy staging eu-west").ExitCode); err != nil {
					return err
				}
				user := lastUserContent(srv.Last())
				if !strings.Contains(user, "BODY-DEPLOY") {
					return &grader.Mismatch{Label: "user message", Want: "the skill body", Got: user}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "substitutes-arguments",
			Title: "substitutes $ARGUMENTS and positional placeholders",
			Why: "A skill that cannot take input always does the same thing. Both " +
				"placeholder forms appear in real skills, often in the same body.",
			Hints: hints.Get("09", "substitutes-arguments"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "/deploy staging eu-west").ExitCode); err != nil {
					return err
				}
				user := lastUserContent(srv.Last())
				for _, want := range []string{
					"Deploy to staging in eu-west.",
					"Full request was: staging eu-west",
				} {
					if !strings.Contains(user, want) {
						return &grader.Mismatch{Label: "user message", Want: want, Got: user}
					}
				}
				if strings.Contains(user, "$0") || strings.Contains(user, "$ARGUMENTS") {
					return &grader.Mismatch{Label: "user message", Want: "placeholders substituted", Got: user}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "stacks-skills",
			Title: "expands several skills from one prompt, in order",
			Why: "People combine instructions: review this, then deploy it. Each skill " +
				"takes its own arguments and the order has to survive.",
			Hints: hints.Get("09", "stacks-skills"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
					skill("review", "Review the current diff.", reviewBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "/review /deploy production eu-west").ExitCode); err != nil {
					return err
				}
				user := lastUserContent(srv.Last())
				reviewAt := strings.Index(user, "BODY-REVIEW")
				deployAt := strings.Index(user, "BODY-DEPLOY")
				if reviewAt < 0 || deployAt < 0 {
					return &grader.Mismatch{Label: "user message", Want: "both bodies", Got: user}
				}
				if reviewAt > deployAt {
					return &grader.Mismatch{Label: "order", Want: "review before deploy", Got: "deploy came first"}
				}
				if !strings.Contains(user, "Deploy to production in eu-west.") {
					return &grader.Mismatch{Label: "user message", Want: "deploy arguments applied", Got: user}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-invocation-no-body",
			Title: "leaves a prompt without slash commands untouched",
			Why: "Expansion must be a no-op for ordinary text. A prompt that grows a " +
				"header or loses its punctuation is a prompt the user cannot debug.",
			Hints: hints.Get("09", "no-invocation-no-body"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				const prompt = `find the "todo" marker and fix it`

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", prompt).ExitCode); err != nil {
					return err
				}
				if !strings.Contains(systemContent(srv.Last()), "Deploy the app to a region.") {
					return &grader.Mismatch{Label: "system content", Want: "the skill description", Got: systemContent(srv.Last())}
				}
				if err := grader.Eqf("user message", prompt, lastUserContent(srv.Last())); err != nil {
					return err
				}
				if strings.Contains(allContent(srv.Last()), "BODY-DEPLOY") {
					return &grader.Mismatch{Label: "request content", Want: "no body", Got: "body present"}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "unknown-skill-preserved",
			Title: "keeps the text of an unknown slash command",
			Why: "A typo must not delete the person's words. Passing the text through " +
				"means the model can still answer, and the user can see what happened.",
			Hints: hints.Get("09", "unknown-skill-preserved"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "I do not have that skill."})

				res := blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "/nosuch please help")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "I do not have that skill.\n", res.Stdout); err != nil {
					return err
				}
				if !strings.Contains(systemContent(srv.Last()), "deploy") {
					return &grader.Mismatch{Label: "system content", Want: "the known skill advertised", Got: systemContent(srv.Last())}
				}
				user := lastUserContent(srv.Last())
				if !strings.Contains(user, "/nosuch please help") {
					return &grader.Mismatch{Label: "user message", Want: "the original text", Got: user}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "frontmatter-not-leaked",
			Title: "sends the skill body without its frontmatter",
			Why: "The header is metadata for the agent, not instructions for the model. " +
				"Sending it wastes tokens and teaches the model to write headers back.",
			Hints: hints.Get("09", "frontmatter-not-leaked"),
			Run: func(t *testing.T) error {
				dir := skillsWorkspace(t,
					skill("deploy", "Deploy the app to a region.", deployBody),
				)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, env(srv.BaseURL()), "-p", "/deploy staging eu").ExitCode); err != nil {
					return err
				}
				user := lastUserContent(srv.Last())
				if !strings.Contains(user, "BODY-DEPLOY") {
					return &grader.Mismatch{Label: "user message", Want: "the expanded body", Got: user}
				}
				if strings.Contains(user, "description:") || strings.HasPrefix(user, "---") {
					return &grader.Mismatch{Label: "user message", Want: "body only", Got: user}
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

// skill renders a SKILL.md with frontmatter.
func skill(name, description, body string) struct {
	Name, Description, Body string
} {
	return struct{ Name, Description, Body string }{name, description, body}
}

func skillsWorkspace(t *testing.T, skills ...struct{ Name, Description, Body string }) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range skills {
		full := filepath.Join(dir, "skills", s.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		content := "---\nname: " + s.Name + "\ndescription: " + s.Description + "\n---\n" + s.Body + "\n"
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}
	}
	return dir
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

func allContent(req fake.Recorded) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func parseEvents(t *testing.T, stderr string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("event line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}
