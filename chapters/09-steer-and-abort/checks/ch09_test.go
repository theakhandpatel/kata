//go:build kata_checks

package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kata/internal/blackbox"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 08 -- steer and abort.
//
// Two ways a run stops being something you wait for: you interrupt it, or you say
// something while it works. Both are graded against the real process, because both
// are about timing.
func TestChapter09(t *testing.T) {
	grader.Suite(t, "09", "steer and abort", []grader.Case{
		{
			ID:    "steer-continues-after-done",
			Title: "a steered message keeps the run going after the model stops",
			Why: "The model says it is done, but a person typed something. Merging that " +
				"into the next prompt loses it; dropping it ignores the person. It " +
				"belongs in the same run, after the turn that just finished.",
			Hints: hints.Get("09", "steer-continues-after-done"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					fake.Reply{Text: "first pass"},
					fake.Reply{Text: "second pass, with tests"},
				)

				res := blackbox.RunWithInput(t, dir, env(srv.BaseURL()), "also run the tests\n",
					"-p", "do the thing", "-steer-stdin")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 2, len(srv.Requests())); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "second pass, with tests\n", res.Stdout); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "steer-enters-after-turn",
			Title: "the steered message arrives after the first turn, not inside it",
			Why: "Two failures are easy here. Appending it to the first prompt changes " +
				"what the model is answering. Appending it before the assistant message " +
				"puts the person's words in the wrong mouth.",
			Hints: hints.Get("09", "steer-enters-after-turn"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					fake.Reply{Text: "first pass"},
					fake.Reply{Text: "done"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunWithInput(t, dir, env(srv.BaseURL()), "remember the branch is release\n",
						"-p", "do the thing", "-steer-stdin").ExitCode); err != nil {
					return err
				}

				first, err := requestN(srv, 0)
				if err != nil {
					return err
				}
				for _, m := range first.Messages {
					if strings.Contains(m.Content, "remember the branch") {
						return &grader.Mismatch{Label: "first request", Want: "no steering text", Got: m.Content}
					}
				}

				second, err := requestN(srv, 1)
				if err != nil {
					return err
				}
				got := roleContents(second.Messages)
				want := []string{
					"system",
					"user|do the thing",
					"assistant|first pass",
					"user|remember the branch is release",
				}
				if len(got) < len(want) {
					return &grader.Mismatch{Label: "second request", Want: strings.Join(want, ", "), Got: strings.Join(got, ", ")}
				}
				for i := range want {
					if got[i] != want[i] {
						return &grader.Mismatch{Label: "message " + itoa(i), Want: want[i], Got: got[i]}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "steer-not-duplicated",
			Title: "each steered line is delivered exactly once",
			Why: "A queue that is drained but not cleared injects the same instruction " +
				"every turn. The model then behaves as if the person repeated themselves " +
				"forever.",
			Hints: hints.Get("09", "steer-not-duplicated"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					fake.Reply{Text: "one"},
					fake.Reply{Text: "two"},
					fake.Reply{Text: "three"},
				)

				if err := grader.Eqf("exit code", 0,
					blackbox.RunWithInput(t, dir, env(srv.BaseURL()), "keep going\n",
						"-p", "start", "-steer-stdin").ExitCode); err != nil {
					return err
				}

				n := 0
				for _, req := range srv.Requests() {
					for _, m := range req.Messages {
						n += strings.Count(m.Content, "keep going")
					}
				}
				if n != 1 {
					return &grader.Mismatch{Label: "occurrences of the steered line", Want: "1", Got: itoa(n)}
				}
				return nil
			},
		},
		{
			ID:    "no-steer-single-request",
			Title: "an empty stdin adds no turn",
			Why: "The flag must not change behaviour by itself. A run with nothing " +
				"steered has to cost exactly one request, whether or not stdin is being " +
				"watched.",
			Hints: hints.Get("09", "no-steer-single-request"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t, fake.Reply{Text: "just an answer"})

				res := blackbox.RunWithInput(t, dir, env(srv.BaseURL()), "",
					"-p", "hello", "-steer-stdin")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "just an answer\n", res.Stdout); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "steer-event-reported",
			Title: "reports a steer event when a message is injected",
			Why: "A terminal UI has to show the person that their message was accepted, " +
				"and when. Without the event the input looks swallowed.",
			Hints: hints.Get("09", "steer-event-reported"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t,
					fake.Reply{Text: "one"},
					fake.Reply{Text: "two"},
				)

				res := blackbox.RunWithInput(t, dir, env(srv.BaseURL()), "check the tests\n",
					"-p", "start", "-steer-stdin", "-events")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				steers := eventsOfType(parseEvents(t, res.Stderr), "steer")
				if len(steers) != 1 {
					return &grader.Mismatch{Label: "steer events", Want: "1", Got: itoa(len(steers))}
				}
				if txt, _ := steers[0]["text"].(string); !strings.Contains(txt, "check the tests") {
					return &grader.Mismatch{Label: "steer event text", Want: "the steered line", Got: txt}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "abort-keeps-partial",
			Title: "keeps the text that arrived before the interrupt",
			Why: "A person who stops a long answer wants the part they already read. " +
				"Throwing it away makes an interrupted run look like a failed one.",
			Hints: hints.Get("09", "abort-keeps-partial"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				// Eight slow frames give the test a wide window to interrupt inside.
				srv := fake.New(t, fake.Reply{
					Text:       "the first part of a long answer that keeps going",
					FrameDelay: 120 * time.Millisecond,
				})

				res := blackbox.RunInterrupt(t, dir, env(srv.BaseURL()), 250*time.Millisecond,
					"-p", "write a lot")
				if res.ExitCode != 130 {
					return &grader.Mismatch{Label: "exit code", Want: 130, Got: res.ExitCode}
				}
				partial := strings.TrimSpace(res.Stdout)
				if partial == "" {
					return &grader.Mismatch{Label: "stdout", Want: "the partial answer", Got: "empty"}
				}
				const full = "the first part of a long answer that keeps going"
				if !strings.HasPrefix(full, partial) {
					return &grader.Mismatch{Label: "stdout", Want: "a prefix of the streamed text", Got: partial}
				}
				return nil
			},
		},
		{
			ID:    "abort-emits-event",
			Title: "ends the event stream with reason aborted",
			Why: "A consumer of the event stream has to distinguish a finished run from " +
				"one the person stopped. Both end with agent_end; only the reason differs.",
			Hints: hints.Get("09", "abort-emits-event"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				srv := fake.New(t, fake.Reply{
					Text:       "a long answer that will be cut short by the interrupt",
					FrameDelay: 120 * time.Millisecond,
				})

				res := blackbox.RunInterrupt(t, dir, env(srv.BaseURL()), 250*time.Millisecond,
					"-p", "write a lot", "-events")
				events := parseEvents(t, res.Stderr)
				end := lastOfType(events, "agent_end")
				if end == nil {
					return &grader.Mismatch{Label: "events", Want: "an agent_end", Got: "none"}
				}
				if reason, _ := end["reason"].(string); reason != "aborted" {
					return &grader.Mismatch{Label: "agent_end reason", Want: "aborted", Got: reason}
				}
				return nil
			},
		},
		{
			ID:    "abort-persists-session",
			Title: "writes the interrupted turn to the session",
			Why: "An interrupted run still produced work. Forgetting it means the next " +
				"run repeats what the person already read, and the session no longer " +
				"matches what happened.",
			Hints: hints.Get("09", "abort-persists-session"),
			Run: func(t *testing.T) error {
				dir := workspace(t, nil)
				path := filepath.Join(dir, "s.jsonl")
				srv := fake.New(t, fake.Reply{
					Text:       "a partial answer that should be recorded somewhere",
					FrameDelay: 120 * time.Millisecond,
				})

				blackbox.RunInterrupt(t, dir, env(srv.BaseURL()), 250*time.Millisecond,
					"-p", "say something", "-session", path)

				body, err := os.ReadFile(path)
				if err != nil {
					return &grader.Mismatch{Label: "session file", Want: "created", Got: err.Error()}
				}
				text := string(body)
				if !strings.Contains(text, "say something") {
					return &grader.Mismatch{Label: "session content", Want: "the user message", Got: "missing"}
				}
				if !strings.Contains(text, `"role":"assistant"`) {
					return &grader.Mismatch{Label: "session content", Want: "an assistant record", Got: text}
				}
				// Only a prefix arrives before the interrupt, so match the stable
				// beginning rather than assuming a whole fragment landed.
				if !strings.Contains(text, "a partial") {
					return &grader.Mismatch{Label: "session content", Want: "the start of the interrupted reply", Got: text}
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

func workspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func requestN(srv *fake.Server, n int) (fake.Recorded, error) {
	reqs := srv.Requests()
	if len(reqs) <= n {
		return fake.Recorded{}, &grader.Mismatch{Label: "requests", Want: "at least " + itoa(n+1), Got: itoa(len(reqs))}
	}
	return reqs[n], nil
}

// roleContents renders messages as "role" or "role|content" for stable comparison.
func roleContents(msgs []fake.RecordedMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		if m.Role == "system" {
			out[i] = "system"
			continue
		}
		out[i] = m.Role + "|" + m.Content
	}
	return out
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

func eventsOfType(events []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

func lastOfType(events []map[string]any, typ string) map[string]any {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["type"] == typ {
			return events[i]
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
