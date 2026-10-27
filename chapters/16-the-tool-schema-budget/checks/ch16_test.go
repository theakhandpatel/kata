//go:build miyagi_checks

package checks

import (
	"encoding/json"
	"strings"
	"testing"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 16 -- the tool schema budget.
//
// Every request carries the schemas of the tools it advertises. This chapter caps how
// many travel, and gives the model a way to ask for the rest.
func TestChapter16(t *testing.T) {
	grader.Suite(t, "16", "the tool schema budget", []grader.Case{
		{
			ID:    "ceiling-respected",
			Title: "advertises no more than the ceiling",
			Why: "Schemas are paid for on every request, so a project with twenty tools " +
				"pays twenty times whether or not any is called.",
			Hints: hints.Get("16", "ceiling-respected"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-max-tools", "3").ExitCode); err != nil {
					return err
				}
				if n := len(srv.Last().Tools); n > 3 {
					return &grader.Mismatch{Label: "advertised tools", Want: "at most 3", Got: support.Itoa(n)}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "hidden-tools-are-named",
			Title: "names the tools it did not advertise",
			Why: "A tool that can only be loaded by guessing its name is never loaded. The " +
				"model needs the list of what exists, even when it is not callable yet.",
			Hints: hints.Get("16", "hidden-tools-are-named"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-max-tools", "2").ExitCode); err != nil {
					return err
				}
				system := support.SystemContent(srv.Last())
				if !strings.Contains(system, "load_tool") {
					return &grader.Mismatch{Label: "system content", Want: "a mention of load_tool", Got: system}
				}
				// bash or write should be named as loadable, since the ceiling is 2.
				named := 0
				for _, name := range []string{"read", "write", "bash", "task", "read_result"} {
					if strings.Contains(system, name) {
						named++
					}
				}
				if named < 3 {
					return &grader.Mismatch{Label: "system content", Want: "the hidden tool names", Got: system}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "load-tool-is-always-advertised",
			Title: "always advertises the loader itself",
			Why: "If the loader can be hidden by the ceiling it enforces, there is no way " +
				"back to anything. It has to be exempt from its own limit.",
			Hints: hints.Get("16", "load-tool-is-always-advertised"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-max-tools", "1").ExitCode); err != nil {
					return err
				}
				if !support.HasTool(srv.Last().Tools, "load_tool") {
					return &grader.Mismatch{Label: "tools", Want: "load_tool advertised even at a ceiling of 1", Got: support.ToolNames(srv.Last().Tools)}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "loading-a-tool-makes-it-callable",
			Title: "makes a hidden tool callable after it is loaded",
			Why: "Naming a hidden tool is only useful if naming it works. The next turn " +
				"has to carry its schema.",
			Hints: hints.Get("16", "loading-a-tool-makes-it-callable"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"a.txt": "alpha"})
				srv := fake.New(t,
					support.Call("call_1", "load_tool", `{"name":"read"}`),
					fake.Reply{Text: "loaded"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "load read", "-max-tools", "2").ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				var advertised []string
				for _, t := range second.Tools {
					if fn, ok := t["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok {
							advertised = append(advertised, name)
						}
					}
				}
				found := false
				for _, n := range advertised {
					if n == "read" {
						found = true
					}
				}
				if !found {
					return &grader.Mismatch{Label: "second request tools", Want: "read after loading it", Got: strings.Join(advertised, ", ")}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "unknown-tool-load-is-a-result",
			Title: "answers a request to load a tool that does not exist",
			Why:   "The name comes from the model. A wrong name is information, not a crash.",
			Hints: hints.Get("16", "unknown-tool-load-is-a-result"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					support.Call("call_1", "load_tool", `{"name":"nonexistent"}`),
					fake.Reply{Text: "there is no such tool"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "load it")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				second, err := support.Request(srv, 1)
				if err != nil {
					return err
				}
				msg, err := support.ToolResult(second, "call_1")
				if err != nil {
					return err
				}
				if !strings.Contains(msg.Content, "no such tool") {
					return &grader.Mismatch{Label: "tool result", Want: "a message naming the missing tool", Got: msg.Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-ceiling-advertises-everything",
			Title: "advertises everything when the ceiling is high enough",
			Why: "The cap has to be a ceiling, not a rule that hides tools from a project " +
				"small enough to afford them.",
			Hints: hints.Get("16", "no-ceiling-advertises-everything"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-max-tools", "50").ExitCode); err != nil {
					return err
				}
				for _, want := range []string{"read", "write", "bash", "load_tool"} {
					if !support.HasTool(srv.Last().Tools, want) {
						return &grader.Mismatch{Label: "tools", Want: want, Got: support.ToolNames(srv.Last().Tools)}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "schemas-still-count-against-budget",
			Title: "advertising fewer tools lowers the budget estimate",
			Why: "The two budgets are related. Hiding schemas is only worth doing if the " +
				"estimate notices the request got smaller.",
			Hints: hints.Get("16", "schemas-still-count-against-budget"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)

				wide := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(wide.BaseURL()),
						"-p", "hi", "-max-tools", "9", "-events", "-max-context", "100000").ExitCode); err != nil {
					return err
				}
				narrow := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(narrow.BaseURL()),
						"-p", "hi", "-max-tools", "2", "-events", "-max-context", "100000").ExitCode); err != nil {
					return err
				}

				wideTokens := firstBudgetTokens(t, wide)
				narrowTokens := firstBudgetTokens(t, narrow)
				if narrowTokens >= wideTokens {
					return &grader.Mismatch{
						Label: "estimate",
						Want:  "fewer tokens with fewer schemas",
						Got:   support.Itoa(wideTokens) + " wide, " + support.Itoa(narrowTokens) + " narrow",
					}
				}
				return nil
			},
		},
	})
}

// --- helpers ---

func firstBudgetTokens(t *testing.T, srv *fake.Server) int {
	t.Helper()
	// The budget event is on stderr, and this helper reads it from a run the case made.
	// The case passes the server, so the events were captured by the caller; here the
	// token count comes from the recorded request instead, which is enough to compare.
	tokens := 0
	req := srv.Last()
	for _, m := range req.Messages {
		tokens += len(m.Content) / 4
	}
	for _, tool := range req.Tools {
		b, _ := json.Marshal(tool)
		tokens += len(b) / 4
	}
	return tokens
}
