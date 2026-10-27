//go:build kata_checks

package checks

import (
	"strings"
	"testing"
	"time"

	"agent/internal/blackbox"
	support "agent/internal/checksupport"
	"agent/internal/fake"
	"agent/internal/grader"
	"agent/internal/hints"
)

// Chapter 14 -- retries and rate limits.
//
// A server that fails and then succeeds must not be visible to the rest of the program.
// A server that keeps failing must stop quickly, with the reason it stopped.
func TestChapter14(t *testing.T) {
	grader.Suite(t, "14", "retries and rate limits", []grader.Case{
		{
			ID:    "retries-a-429",
			Title: "retries a rate-limited request and answers",
			Why: "A 429 means the server is busy, not that the request is wrong. Giving " +
				"up turns a temporary condition into a failure the person has to handle " +
				"by hand.",
			Hints: hints.Get("14", "retries-a-429"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Status: 429, Body: `{"error":"slow down"}`},
					fake.Reply{Text: "second time worked"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-retry-delay", "10ms")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "second time worked\n", res.Stdout); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 2, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "retries-a-500",
			Title: "retries a server error",
			Why: "The 5xx family is the server admitting a problem on its side. The same " +
				"request has a reasonable chance the second time.",
			Hints: hints.Get("14", "retries-a-500"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Status: 503, Body: `unavailable`},
					fake.Reply{Text: "recovered"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-retry-delay", "10ms")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 2, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "does-not-retry-a-400",
			Title: "does not retry a bad request",
			Why: "A 400 means the request is wrong. Sending the same wrong request again " +
				"wastes the person's money and hides the real problem behind a pause.",
			Hints: hints.Get("14", "does-not-retry-a-400"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Status: 400, Body: `{"error":"bad model"}`},
					fake.Reply{Text: "should never arrive"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-retry-delay", "10ms")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				if !strings.Contains(res.Stderr, "400") {
					return &grader.Mismatch{Label: "stderr", Want: "the status in the message", Got: res.Stderr}
				}
				return nil
			},
		},
		{
			ID:    "honours-retry-after",
			Title: "waits as long as the server asked",
			Why: "Retry-After is the server telling you when to come back. Retrying " +
				"sooner earns another 429, and in a shared service it is rude as well as " +
				"useless.",
			Hints: hints.Get("14", "honours-retry-after"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{
						Status:  429,
						Body:    `{"error":"slow down"}`,
						Headers: map[string]string{"Retry-After": "1"},
					},
					fake.Reply{Text: "ok"},
				)

				started := time.Now()
				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-retry-delay", "10ms")
				elapsed := time.Since(started)

				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				// The header asks for one second; a 10ms base delay would be too early.
				if elapsed < 900*time.Millisecond {
					return &grader.Mismatch{
						Label: "elapsed",
						Want:  "at least 900ms, waiting as Retry-After asked",
						Got:   elapsed.Round(time.Millisecond).String(),
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "stops-after-the-limit",
			Title: "gives up after the configured number of attempts",
			Why: "A server that is down stays down. Retrying forever turns an outage into " +
				"a program that never returns.",
			Hints: hints.Get("14", "stops-after-the-limit"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Status: 500, Body: "down"},
					fake.Reply{Status: 500, Body: "down"},
					fake.Reply{Status: 500, Body: "down"},
					fake.Reply{Status: 500, Body: "down"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-retries", "3", "-retry-delay", "10ms")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 3, len(srv.Requests())); err != nil {
					return err
				}
				if strings.TrimSpace(res.Stderr) == "" {
					return &grader.Mismatch{Label: "stderr", Want: "an error message", Got: "empty"}
				}
				return nil
			},
		},
		{
			ID:    "no-retry-on-success",
			Title: "sends one request when nothing fails",
			Why: "Retry logic that fires on success doubles the cost of every call and " +
				"prints the answer twice.",
			Hints: hints.Get("14", "no-retry-on-success"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "first time"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "x")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("requests", 1, len(srv.Requests())); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "does-not-retry-a-cancellation",
			Title: "stops immediately when the caller cancels",
			Why: "Ctrl-C has to mean stop. A retry loop that ignores the context turns an " +
				"interrupt into a program that keeps trying for another minute.",
			Hints: hints.Get("14", "does-not-retry-a-cancellation"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t,
					fake.Reply{Status: 500, Body: "down", Delay: 2 * time.Second},
					fake.Reply{Status: 500, Body: "down"},
				)

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "x", "-timeout", "100ms", "-retries", "5", "-retry-delay", "10ms")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if res.Duration > 1500*time.Millisecond {
					return &grader.Mismatch{
						Label: "duration",
						Want:  "under 1.5s, because the context was cancelled",
						Got:   res.Duration.Round(time.Millisecond).String(),
					}
				}
				return nil
			},
		},
	})
}

// --- helpers ---
