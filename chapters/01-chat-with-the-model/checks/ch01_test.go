//go:build lathe_checks

package checks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lathe/harness"
	"lathe/internal/fake"
	"lathe/internal/grader"
	"lathe/internal/hints"
)

// Chapter 01 -- Talk to the model.
//
// Everything here runs against a scripted server. No API key, no network, no
// randomness. If these pass, your wire code is correct; if they fail, the
// message names the mechanism you got wrong.
func TestChapter01(t *testing.T) {
	grader.Suite(t, "01", "talk to the model", []grader.Case{
		{
			ID:    "request-shape",
			Title: "sends a well-formed streaming request",
			Why: "An OpenAI-compatible server routes on the path, reads the model from the " +
				"body, and authenticates with a bearer token. Get any of those wrong and you " +
				"get a 401 or 404 that looks like a logic bug.",
			Hints: hints.Get("01", "request-shape"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})
				p := harness.NewOpenAICompat(srv.BaseURL(), "test-key", "gpt-4o-mini")

				_, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{
						{Role: harness.RoleSystem, Content: "You are terse."},
						{Role: harness.RoleUser, Content: "hi"},
					},
				})
				if err := grader.NoErr(err); err != nil {
					return err
				}

				req := srv.Last()
				if err := grader.Eqf("path", "/v1/chat/completions", req.Path); err != nil {
					return err
				}
				if err := grader.Eqf("authorization", "Bearer test-key", req.Authorization); err != nil {
					return err
				}
				if err := grader.Eqf("model", "gpt-4o-mini", req.Model); err != nil {
					return err
				}
				if err := grader.Eqf("stream", true, req.Stream); err != nil {
					return err
				}
				want := []fake.RecordedMessage{
					{Role: "system", Content: "You are terse."},
					{Role: "user", Content: "hi"},
				}
				if err := grader.Eqf("messages", want, req.Messages); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "request-envelope",
			Title: "sets the transport headers and the optional fields it is given",
			Why: "A request is an envelope, not only a body. Content-Type and Accept are " +
				"what tell the server how to answer; omitting them works against a lenient " +
				"fixture and fails against a real one.",
			Hints: hints.Get("01", "request-envelope"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				_, _, err := p.Chat(context.Background(), harness.Request{
					Messages:    []harness.Message{{Role: harness.RoleUser, Content: "x"}},
					MaxTokens:   128,
					Temperature: 0.7,
				})
				if err := grader.NoErr(err); err != nil {
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
					"model":  "m",
					"stream": true,
					"messages": []any{
						map[string]any{"role": "user", "content": "x"},
					},
				}, req.Raw); err != nil {
					return err
				}
				if err := grader.Eqf("max_tokens", float64(128), req.Raw["max_tokens"]); err != nil {
					return err
				}
				if err := grader.Eqf("temperature", 0.7, req.Raw["temperature"]); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "optional-zero-values",
			Title: "omits optional numbers instead of sending zero",
			Why: "Zero is not the same as unset. A body containing \"max_tokens\": 0 tells a " +
				"real provider to produce nothing, so a harness that always serialises the " +
				"field works in tests and fails in production.",
			Hints: hints.Get("01", "optional-zero-values"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "ok"})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				_, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
					// MaxTokens and Temperature deliberately left at zero.
				})
				if err := grader.NoErr(err); err != nil {
					return err
				}
				raw := srv.Last().Raw
				if _, present := raw["max_tokens"]; present {
					return &grader.Mismatch{Label: "max_tokens", Want: "absent when zero", Got: raw["max_tokens"]}
				}
				if _, present := raw["temperature"]; present {
					return &grader.Mismatch{Label: "temperature", Want: "absent when zero", Got: raw["temperature"]}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "text-reassembly",
			Title: "reassembles text split across SSE chunks",
			Why: "The response arrives as many small deltas. Assigning instead of appending " +
				"passes a one-chunk test and silently truncates real answers.",
			Hints: hints.Get("01", "text-reassembly"),
			Run: func(t *testing.T) error {
				const want = "The quick brown fox jumps over the lazy dog."
				srv := fake.New(t, fake.Reply{Text: want})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "gpt-4o-mini")

				msg, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				if err := grader.NoErr(err); err != nil {
					return err
				}
				if err := grader.Eqf("role", harness.RoleAssistant, msg.Role); err != nil {
					return err
				}
				if err := grader.Eqf("content", want, msg.Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "sse-framing",
			Title: "tolerates comments, named fields, CRLF and data: without a space",
			Why: "data: lines are only one kind of line in an event stream. Harnesses that " +
				"split on a blank line and assume every chunk starts with \"data: \" break on " +
				"the first provider that sends a keep-alive comment.",
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
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				msg, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				if err := grader.NoErr(err); err != nil {
					return err
				}
				if err := grader.Eqf("content", "Hello framed.", msg.Content); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "usage",
			Title: "reads token usage from the final chunk",
			Why: "Usage rides on the last chunk only. Harnesses that never read it cannot " +
				"budget context or report cost.",
			Hints: hints.Get("01", "usage"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{
					Text:  "ok",
					Usage: &fake.Usage{PromptTokens: 42, CompletionTokens: 7, TotalTokens: 49},
				})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				_, usage, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				if err := grader.NoErr(err); err != nil {
					return err
				}
				if err := grader.Eqf("usage", harness.Usage{
					PromptTokens: 42, CompletionTokens: 7, TotalTokens: 49,
				}, usage); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "http-error",
			Title: "surfaces a non-2xx response as *harness.HTTPError",
			Why: "A 429 is not a parse failure. Wrapping it in a typed error is what lets the " +
				"loop later retry with backoff and lets the CLI say something useful.",
			Hints: hints.Get("01", "http-error"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Status: 429, Body: `{"error":"slow down"}`})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				_, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				if err := grader.WantedErr(err); err != nil {
					return err
				}
				var he *harness.HTTPError
				if !errors.As(err, &he) {
					return &grader.Mismatch{Label: "error type", Want: "*harness.HTTPError", Got: err}
				}
				if err := grader.Eqf("status", 429, he.Status); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "truncated-stream",
			Title: "reports a stream that dies mid-response",
			Why: "This is the bug every harness ships once. If you ignore the scanner's error, " +
				"a connection reset looks like a short but valid answer.",
			Hints: hints.Get("01", "truncated-stream"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{FailMidStream: true})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				_, _, err := p.Chat(context.Background(), harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				return grader.WantedErr(err)
			},
		},
		{
			ID:    "context-cancel",
			Title: "honours context cancellation",
			Why: "Abort is a core harness feature. It only works if the request carries the " +
				"caller's context instead of context.Background().",
			Hints: hints.Get("01", "context-cancel"),
			Run: func(t *testing.T) error {
				srv := fake.New(t, fake.Reply{Text: "never sent"})
				p := harness.NewOpenAICompat(srv.BaseURL(), "k", "m")

				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				ctx, cancel2 := context.WithTimeout(ctx, 2*time.Second)
				defer cancel2()

				_, _, err := p.Chat(ctx, harness.Request{
					Messages: []harness.Message{{Role: harness.RoleUser, Content: "x"}},
				})
				if err := grader.WantedErr(err); err != nil {
					return err
				}
				if !errors.Is(err, context.Canceled) {
					return &grader.Mismatch{Label: "error", Want: "wrapping context.Canceled", Got: err}
				}
				return nil
			},
		},
	})
}
