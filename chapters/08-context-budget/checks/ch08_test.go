//go:build kata_checks

package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kata/internal/blackbox"
	support "kata/internal/checksupport"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 07 -- the context budget.
//
// Two things are graded: the estimate the program reports, and what the request
// actually contains after pruning. The estimate is observable through the budget
// event, so no case reaches inside the package.
func TestChapter08(t *testing.T) {
	grader.Suite(t, "08", "the context budget", []grader.Case{
		{
			ID:    "budget-event-reported",
			Title: "reports the estimate and the budget for every request",
			Why: "A budget that cannot be observed cannot be trusted or debugged. One " +
				"event per request turns the accounting into something a person can read.",
			Hints: hints.Get("08", "budget-event-reported"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "hello", "-events", "-max-context", "1000")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				budgets := support.EventsOfType(support.Events(t, res.Stderr), "budget")
				if len(budgets) == 0 {
					return &grader.Mismatch{Label: "events", Want: "at least one budget event", Got: "none"}
				}
				b := budgets[0]
				if tokens, _ := b["tokens"].(float64); tokens <= 0 {
					return &grader.Mismatch{Label: "tokens", Want: "greater than 0", Got: b["tokens"]}
				}
				if budget, _ := b["budget"].(float64); int(budget) != 1000 {
					return &grader.Mismatch{Label: "budget", Want: 1000, Got: b["budget"]}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "estimate-counts-tools",
			Title: "counts the tool schemas, not only the messages",
			Why: "Schemas are sent on every request and are often larger than the " +
				"conversation. An estimate that ignores them lets a request overflow " +
				"while the accounting says there is room.",
			Hints: hints.Get("08", "estimate-counts-tools"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-events", "-max-context", "100000")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				budgets := support.EventsOfType(support.Events(t, res.Stderr), "budget")
				if len(budgets) == 0 {
					return &grader.Mismatch{Label: "events", Want: "a budget event", Got: "none"}
				}
				tokens, _ := budgets[0]["tokens"].(float64)
				// One short prompt plus a system message is far under 50 tokens; the
				// three tool schemas are several hundred bytes each.
				if tokens < 50 {
					return &grader.Mismatch{Label: "tokens", Want: "at least 50 with three tools advertised", Got: tokens}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "estimate-grows-with-history",
			Title: "the estimate grows when history grows",
			Why: "Monotonicity is the property a pruning decision rests on. An estimate " +
				"that can fall when content is added makes the loop prune the wrong end.",
			Hints: hints.Get("08", "estimate-grows-with-history"),
			Run: func(t *testing.T) error {
				short := support.Dir(t)
				writeSession(t, filepath.Join(short, "s.jsonl"), repeat(2, 400))
				small := fake.New(t, fake.Reply{Text: "ok"})
				res1 := blackbox.RunIn(t, short, support.Env(small.BaseURL()),
					"-p", "q", "-session", filepath.Join(short, "s.jsonl"),
					"-events", "-max-context", "100000")
				if err := grader.Eqf("exit code", 0, res1.ExitCode); err != nil {
					return err
				}

				long := support.Dir(t)
				writeSession(t, filepath.Join(long, "s.jsonl"), repeat(12, 400))
				big := fake.New(t, fake.Reply{Text: "ok"})
				res2 := blackbox.RunIn(t, long, support.Env(big.BaseURL()),
					"-p", "q", "-session", filepath.Join(long, "s.jsonl"),
					"-events", "-max-context", "100000")
				if err := grader.Eqf("exit code", 0, res2.ExitCode); err != nil {
					return err
				}

				t1 := firstBudgetTokens(t, support.Events(t, res1.Stderr))
				t2 := firstBudgetTokens(t, support.Events(t, res2.Stderr))
				if t2 <= t1 {
					return &grader.Mismatch{Label: "tokens",
						Want: "more with 12 prior messages than with 2",
						Got:  support.Itoa(t1) + " then " + support.Itoa(t2)}
				}
				return nil
			},
		},
		{
			ID:    "ceiling-respected",
			Title: "keeps every request under the ceiling",
			Why: "A threshold that fires eventually is not a budget. The ceiling has to " +
				"hold on every request, including the ones a long run makes later.",
			Hints: hints.Get("08", "ceiling-respected"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				writeSession(t, filepath.Join(dir, "s.jsonl"), repeat(40, 400))
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "q", "-session", filepath.Join(dir, "s.jsonl"),
					"-events", "-max-context", "500")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				budgets := support.EventsOfType(support.Events(t, res.Stderr), "budget")
				if len(budgets) == 0 {
					return &grader.Mismatch{Label: "events", Want: "a budget event", Got: "none"}
				}
				for _, b := range budgets {
					tokens, _ := b["tokens"].(float64)
					if int(tokens) > 500 {
						return &grader.Mismatch{Label: "tokens", Want: "at most 500", Got: int(tokens)}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "prunes-when-over",
			Title: "drops history when the budget cannot hold it",
			Why: "Reporting a number and then sending everything is the easiest way to " +
				"look correct. The event has to agree with what was dropped.",
			Hints: hints.Get("08", "prunes-when-over"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				writeSession(t, filepath.Join(dir, "s.jsonl"), repeat(40, 400))
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "q", "-session", filepath.Join(dir, "s.jsonl"),
					"-events", "-max-context", "500")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				budgets := support.EventsOfType(support.Events(t, res.Stderr), "budget")
				if len(budgets) == 0 {
					return &grader.Mismatch{Label: "events", Want: "a budget event", Got: "none"}
				}
				pruned, _ := budgets[0]["pruned"].(float64)
				if pruned <= 0 {
					return &grader.Mismatch{Label: "pruned", Want: "more than 0", Got: budgets[0]["pruned"]}
				}
				if len(srv.Last().Messages) > 10 {
					return &grader.Mismatch{Label: "request messages", Want: "a pruned history", Got: support.Itoa(len(srv.Last().Messages))}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "prune-marker-present",
			Title: "marks the place where history was removed",
			Why: "A model that believes the conversation started in the middle will " +
				"answer confidently about work it cannot see. The marker is how it knows " +
				"it is missing context.",
			Hints: hints.Get("08", "prune-marker-present"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				writeSession(t, filepath.Join(dir, "s.jsonl"), repeat(40, 400))
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "q",
						"-session", filepath.Join(dir, "s.jsonl"), "-max-context", "500").ExitCode); err != nil {
					return err
				}
				joined := ""
				for _, m := range srv.Last().Messages {
					joined += m.Content + "\n"
				}
				if !strings.Contains(joined, "[context pruned") {
					return &grader.Mismatch{Label: "request content", Want: "a pruning marker", Got: joined}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "no-orphaned-tool-pairs",
			Title: "never sends a tool result whose call was dropped",
			Why: "Pruning at a boundary is where conversations become invalid. A tool " +
				"result without its call is rejected by every provider, so the cut has to " +
				"respect the pair.",
			Hints: hints.Get("08", "no-orphaned-tool-pairs"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				path := filepath.Join(dir, "s.jsonl")
				writeRawSession(t, path, []string{
					rec("user", strings.Repeat("a", 400)),
					`{"type":"message","message":{"role":"assistant","content":"","toolCalls":[{"id":"call_1","name":"read","arguments":"{\"path\":\"a.txt\"}"}]}}`,
					`{"type":"message","message":{"role":"tool","content":"alpha","toolCallId":"call_1"}}`,
					rec("assistant", strings.Repeat("b", 400)),
					rec("user", strings.Repeat("c", 400)),
					rec("assistant", strings.Repeat("d", 400)),
					rec("user", strings.Repeat("e", 400)),
				})
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "q",
						"-session", path, "-max-context", "500").ExitCode); err != nil {
					return err
				}

				announced := map[string]bool{}
				for _, m := range srv.Last().Messages {
					for _, tc := range m.ToolCalls {
						announced[tc.ID] = true
					}
					if m.Role == "tool" && !announced[m.ToolCallID] {
						return &grader.Mismatch{
							Label: "tool result " + m.ToolCallID,
							Want:  "a preceding assistant call",
							Got:   "orphaned",
						}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "keeps-system-and-newest",
			Title: "keeps the system message and the newest prompt",
			Why: "The two things pruning must never remove are the instructions and the " +
				"thing the person just asked for. Dropping either makes the answer wrong " +
				"while the request looks valid.",
			Hints: hints.Get("08", "keeps-system-and-newest"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"AGENTS.md": "project rule\n"})
				writeSession(t, filepath.Join(dir, "s.jsonl"), repeat(40, 400))
				srv := fake.New(t, fake.Reply{Text: "ok"})
				const prompt = "what should I do next"

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", prompt,
						"-session", filepath.Join(dir, "s.jsonl"), "-max-context", "500").ExitCode); err != nil {
					return err
				}

				msgs := srv.Last().Messages
				if len(msgs) == 0 {
					return &grader.Mismatch{Label: "messages", Want: "at least one", Got: "none"}
				}
				if err := grader.Eqf("messages[0].role", "system", msgs[0].Role); err != nil {
					return err
				}
				if !strings.Contains(msgs[0].Content, "project rule") {
					return &grader.Mismatch{Label: "system content", Want: "the AGENTS.md body", Got: msgs[0].Content}
				}
				if err := grader.Eqf("last message", prompt, msgs[len(msgs)-1].Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---

// repeat builds alternating user and assistant records of a fixed size.
func repeat(n, size int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		out = append(out, rec(role, strings.Repeat(string(rune('a'+i%26)), size)))
	}
	return out
}

func rec(role, content string) string {
	b, _ := json.Marshal(map[string]any{
		"type":    "message",
		"message": map[string]any{"role": role, "content": content},
	})
	return string(b)
}

// writeSession writes a session file in the format chapter 06 established. The
// format is a contract, so building one directly is not reaching inside the
// program.
func writeSession(t *testing.T, path string, records []string) {
	t.Helper()
	writeRawSession(t, path, records)
}

func writeRawSession(t *testing.T, path string, records []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b strings.Builder
	b.WriteString(`{"type":"header","version":1,"created":"2026-01-01T00:00:00Z"}`)
	b.WriteString("\n")
	for _, r := range records {
		b.WriteString(r)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

func firstBudgetTokens(t *testing.T, events []map[string]any) int {
	t.Helper()
	for _, e := range events {
		if e["type"] != "budget" {
			continue
		}
		tokens, _ := e["tokens"].(float64)
		return int(tokens)
	}
	t.Fatalf("no budget event in %v", events)
	return 0
}
