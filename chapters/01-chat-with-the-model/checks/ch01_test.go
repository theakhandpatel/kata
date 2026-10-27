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

// Chapter 01 -- talk to the model.
//
// Every case runs the real binary against a scripted server, because the
// deliverable is a program a user can run. Nothing here calls your package
// directly, so the internal design is yours.
func TestChapter01(t *testing.T) {
	grader.Suite(t, "01", "talk to the model", []grader.Case{
		{
			ID:    "prints-reply",
			Title: "prints the model's answer on stdout",
			Why: "The program's job in this chapter is to make one call and print the " +
				"answer. Text arrives in pieces, so a program that replaces instead of " +
				"appends prints only the last piece and still looks correct on a short " +
				"reply.",
			Hints: hints.Get("01", "prints-reply"),
			Run: func(t *testing.T) error {
				const want = "The quick brown fox jumps over the lazy dog."
				srv := fake.New(t, fake.Reply{Text: want})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "say something")

				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", want+"\n", res.Stdout); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "request-shape",
			Title: "sends a well-formed streaming request",
			Why: "The server routes on the path, reads the model from the body, and " +
				"authenticates with a bearer token. A wrong path or a missing key produces " +
				"an error that reads like a logic bug.",
			Hints: hints.Get("01", "request-shape"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "hi", "-m", "test-model")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}

				req := srv.Last()
				if err := grader.Eqf("path", "/v1/chat/completions", req.Path); err != nil {
					return err
				}
				if err := grader.Eqf("authorization", "Bearer test-key", req.Authorization); err != nil {
					return err
				}
				if err := grader.Eqf("model", "test-model", req.Model); err != nil {
					return err
				}
				if err := grader.Eqf("stream", true, req.Stream); err != nil {
					return err
				}
				want := []fake.RecordedMessage{{Role: "user", Content: "hi"}}
				if err := grader.Eqf("messages", want, req.Messages); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "request-envelope",
			Title: "sets the transport headers",
			Why: "Content-Type and Accept tell the server how to answer. Omitting them " +
				"works against a lenient fixture and fails against a real endpoint.",
			Hints: hints.Get("01", "request-envelope"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0, blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}

				req := srv.Last()
				if err := grader.Eqf("content-type", "application/json", req.ContentType); err != nil {
					return err
				}
				if !strings.Contains(req.Accept, "text/event-stream") {
					return &grader.Mismatch{Label: "accept", Want: "text/event-stream", Got: req.Accept}
				}
				if err := grader.Subset(map[string]any{
					"stream":   true,
					"messages": []any{map[string]any{"role": "user", "content": "x"}},
				}, req.Raw); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "optional-zero-values",
			Title: "omits optional numbers instead of sending zero",
			Why: "Zero is not the same as unset. A body containing \"max_tokens\": 0 tells " +
				"a real provider to produce nothing, so a harness that always serialises " +
				"the field passes the fixture and fails in production.",
			Hints: hints.Get("01", "optional-zero-values"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})
				if err := grader.Eqf("exit code", 0, blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x").ExitCode); err != nil {
					return err
				}

				raw := srv.Last().Raw
				if v, present := raw["max_tokens"]; present {
					return &grader.Mismatch{Label: "max_tokens", Want: "absent when unset", Got: v}
				}
				if v, present := raw["temperature"]; present {
					return &grader.Mismatch{Label: "temperature", Want: "absent when unset", Got: v}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "sse-framing",
			Title: "tolerates comments, named fields, CRLF and data: without a space",
			Why: "data: lines are only one kind of line in an event stream. Programs that " +
				"split on a blank line and assume every chunk starts with \"data: \" break " +
				"on the first provider that sends a keep-alive comment.",
			Hints: hints.Get("01", "sse-framing"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Frames: []fake.Frame{
					{Raw: []string{": keep-alive\r\n\r\n"}},
					{Raw: []string{"event: message\r\n"}},
					{Raw: []string{`data:{"choices":[{"delta":{"content":"Hello "}}]}` + "\r\n\r\n"}},
					{Raw: []string{"\r\n"}},
					{Raw: []string{`data: {"choices":[{"delta":{"content":"framed."}}]}` + "\r\n\r\n"}},
					{Finish: "stop"},
				}})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "Hello framed.\n", res.Stdout); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "debug-usage",
			Title: "writes token usage to stderr, not stdout",
			Why: "The answer goes to stdout and everything else goes to stderr, so the " +
				"answer can be piped. Token counts are the first thing that must respect " +
				"that split; chapter 07 uses them to enforce a budget.",
			Hints: hints.Get("01", "debug-usage"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{
					Text:  "ok",
					Usage: &fake.Usage{PromptTokens: 42, CompletionTokens: 7, TotalTokens: 49},
				})

				e := support.Env(srv.BaseURL())
				e["AGENT_DEBUG"] = "1"
				res := blackbox.Run(t, e, "-p", "x")

				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "ok\n", res.Stdout); err != nil {
					return err
				}
				for _, want := range []string{"prompt=42", "completion=7", "total=49"} {
					if !strings.Contains(res.Stderr, want) {
						return &grader.Mismatch{Label: "stderr", Want: want, Got: res.Stderr}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "http-error",
			Title: "fails loudly on a non-2xx response",
			Why: "A 429 is not a parse failure. The program must exit non-zero and say " +
				"what the server said, so a caller can retry or stop.",
			Hints: hints.Get("01", "http-error"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Status: 429, Body: `{"error":"slow down"}`})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x")
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if err := grader.Eqf("stdout", "", res.Stdout); err != nil {
					return err
				}
				if !strings.Contains(res.Stderr, "429") {
					return &grader.Mismatch{Label: "stderr", Want: "a 429 in the message", Got: res.Stderr}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "truncated-stream",
			Title: "reports a stream that dies mid-response",
			Why: "This is the bug every harness ships once. If the scanner's error is " +
				"ignored, a dropped connection looks like a short but valid answer and the " +
				"program exits 0.",
			Hints: hints.Get("01", "truncated-stream"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{FailMidStream: true})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x")
				if err := grader.Eqf("requests made", 1, len(srv.Requests())); err != nil {
					return err
				}
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if strings.TrimSpace(res.Stderr) == "" {
					return &grader.Mismatch{Label: "stderr", Want: "an error message", Got: "empty"}
				}
				return nil
			},
		},
		{
			ID:    "request-timeout",
			Title: "honours the request timeout",
			Why: "Cancellation only works if the request carries the caller's context. " +
				"With context.Background() the program waits for a server that never " +
				"answers, and Ctrl-C does nothing.",
			Hints: hints.Get("01", "request-timeout"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "too late", Delay: 2 * time.Second})

				res := blackbox.Run(t, support.Env(srv.BaseURL()), "-p", "x", "-timeout", "50ms")
				if err := grader.Eqf("requests made", 1, len(srv.Requests())); err != nil {
					return err
				}
				if err := grader.Eqf("exit code", 1, res.ExitCode); err != nil {
					return err
				}
				if res.Duration > 1500*time.Millisecond {
					return &grader.Mismatch{
						Label: "duration",
						Want:  "under 1.5s (the timeout must fire)",
						Got:   res.Duration.String(),
					}
				}
				return nil
			},
		},
	})
}
