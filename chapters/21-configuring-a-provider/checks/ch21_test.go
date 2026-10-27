//go:build kata_checks

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

// Chapter 21 -- configuring a provider.
//
// Settings come from files, then the environment, then flags. The report says where each
// value came from, and the key is never printed.
func TestChapter21(t *testing.T) {
	grader.Suite(t, "21", "configuring a provider", []grader.Case{
		{
			ID:    "config-from-file",
			Title: "reads the endpoint and model from agent.json",
			Why: "A model name is not something to re-export in every shell. A project that " +
				"talks to one provider should say so in a file.",
			Hints: hints.Get("21", "config-from-file"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "from the file"})
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "`+srv.BaseURL()+`", "model": "file-model"}`)

				res := runClean(t, dir, "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "from the file\n", res.Stdout); err != nil {
					return err
				}
				if err := grader.Eqf("model", "file-model", srv.Last().Model); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "flag-beats-file",
			Title: "lets a flag override the file",
			Why: "A file is shared and long-lived. One command sometimes needs a different " +
				"model, and the flag is the cheapest way to say so.",
			Hints: hints.Get("21", "flag-beats-file"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "http://127.0.0.1:1/v1", "model": "file-model"}`)

				res := runClean(t, dir, "-p", "hi", "-base-url", srv.BaseURL(), "-m", "flag-model")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("model", "flag-model", srv.Last().Model); err != nil {
					return err
				}
				if err := grader.Eqf("path", "/v1/chat/completions", srv.Last().Path); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "env-beats-file",
			Title: "lets the environment override the file",
			Why: "A shell with one export should beat a file, or you cannot point a single " +
				"session somewhere else without editing the project.",
			Hints: hints.Get("21", "env-beats-file"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "http://127.0.0.1:1/v1", "model": "file-model"}`)

				res := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": srv.BaseURL(),
					"OPENAI_API_KEY":  "k",
					"AGENT_MODEL":   "env-model",
				}, "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("model", "env-model", srv.Last().Model); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "all-three-sources-at-once",
			Title: "resolves files, then environment, then flags when all three disagree",
			Why: "Each case so far sets one source above the file. Only with all three set at " +
				"once does the order itself get tested, and the order is the whole feature.",
			Hints: hints.Get("21", "all-three-sources-at-once"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				home := t.TempDir()
				srv := fake.New(t, fake.Reply{Text: "ok"})

				// Project file says one model.
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "http://127.0.0.1:1/v1", "model": "from-file"}`)

				// The environment says another, and also points at the live server.
				res := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": srv.BaseURL(),
					"OPENAI_API_KEY":  "k",
					"AGENT_MODEL":   "from-env",
					"HOME":            home,
					"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
				}, "-p", "hi", "-m", "from-flag")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("model", "from-flag", srv.Last().Model); err != nil {
					return err
				}
				// And without the flag, the environment still wins over the file.
				srv2 := fake.New(t, fake.Reply{Text: "ok"})
				res2 := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": srv2.BaseURL(),
					"OPENAI_API_KEY":  "k",
					"AGENT_MODEL":   "from-env",
					"HOME":            home,
					"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
				}, "-p", "hi")
				if err := grader.Eqf("exit code", 0, res2.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("model without the flag", "from-env", srv2.Last().Model); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				srv2.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "key-from-file",
			Title: "reads the key from a file when asked",
			Why: "An environment variable is visible in a process listing and a shell " +
				"history. A file can be chmod 600 and stays out of both.",
			Hints: hints.Get("21", "key-from-file"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})
				keyPath := filepath.Join(dir, "key.txt")
				support.WriteFile(t, keyPath, "sk-from-a-file\n")
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "`+srv.BaseURL()+`", "model": "m", "api_key_file": "`+keyPath+`"}`)

				res := runClean(t, dir, "-p", "hi")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("authorization", "Bearer sk-from-a-file", srv.Last().Authorization); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "missing-key-file-is-reported",
			Title: "says which key file is missing",
			Why: "A silent start with no key produces a 401 from the provider, which reads " +
				"like a wrong key rather than a missing file.",
			Hints: hints.Get("21", "missing-key-file-is-reported"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "should not be reached"})
				missing := filepath.Join(dir, "not-here.txt")
				support.WriteFile(t, filepath.Join(dir, "agent.json"),
					`{"base_url": "`+srv.BaseURL()+`", "model": "m", "api_key_file": "`+missing+`"}`)

				res := runClean(t, dir, "-p", "hi")
				if res.ExitCode == 0 {
					return &grader.Mismatch{Label: "exit code", Want: "non-zero", Got: "0"}
				}
				if !strings.Contains(res.Stderr, "not-here.txt") {
					return &grader.Mismatch{Label: "stderr", Want: "the missing path", Got: res.Stderr}
				}
				if len(srv.Requests()) != 0 {
					return &grader.Mismatch{Label: "requests", Want: "none", Got: "the agent called anyway"}
				}
				return nil
			},
		},
		{
			ID:    "config-report-redacts-the-key",
			Title: "prints the resolved settings without the key",
			Why: "The report exists to answer 'why is it using that model'. Printing the key " +
				"would make it the least safe command in the tool.",
			Hints: hints.Get("21", "config-report-redacts-the-key"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				res := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": "https://example.test/v1",
					"OPENAI_API_KEY":  "sk-super-secret-value",
					"AGENT_MODEL":   "some-model",
				}, "-config")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if strings.Contains(res.Stdout, "sk-super-secret-value") {
					return &grader.Mismatch{Label: "stdout", Want: "the key redacted", Got: res.Stdout}
				}
				for _, want := range []string{"https://example.test/v1", "some-model", "environment"} {
					if !strings.Contains(res.Stdout, want) {
						return &grader.Mismatch{Label: "stdout", Want: want, Got: res.Stdout}
					}
				}
				return nil
			},
		},
		{
			ID:    "config-needs-no-provider",
			Title: "reports settings without calling a model",
			Why:   "Asking how you are configured must work before the configuration is valid.",
			Hints: hints.Get("21", "config-needs-no-provider"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				res := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": "http://127.0.0.1:1/v1",
				}, "-config")
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
			ID:    "bad-base-url-is-explained",
			Title: "explains a base URL with no scheme",
			Why: "A URL without https:// makes the client treat the host as a path, and the " +
				"error that comes back is about a missing protocol scheme, not about the " +
				"setting you typed.",
			Hints: hints.Get("21", "bad-base-url-is-explained"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				res := blackbox.RunIn(t, dir, map[string]string{
					"OPENAI_BASE_URL": "api.example.test/v1",
				}, "-config")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if !strings.Contains(res.Stderr, "scheme") {
					return &grader.Mismatch{Label: "stderr", Want: "a hint about the missing scheme", Got: res.Stderr}
				}
				return nil
			},
		},
	})
}

// --- helpers ---

// runClean runs with a HOME and XDG_CONFIG_HOME that hold no config, so the machine's real
// settings cannot influence the result.
func runClean(t *testing.T, dir string, args ...string) blackbox.Result {
	t.Helper()
	home := t.TempDir()
	return blackbox.RunIn(t, dir, map[string]string{
		"OPENAI_BASE_URL": "",
		"OPENAI_API_KEY":  "",
		"AGENT_MODEL":   "",
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
	}, args...)
}
