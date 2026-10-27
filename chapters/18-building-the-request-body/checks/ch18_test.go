//go:build kata_checks

package checks

import (
	"strings"
	"testing"

	"kata/internal/blackbox"
	support "kata/internal/checksupport"
	"kata/internal/fake"
	"kata/internal/grader"
	"kata/internal/hints"
)

// Chapter 18 -- building the request body.
//
// The body is where optional fields are easy to get wrong: a zero that should be absent,
// a field that should always be present, a header that decides whether the server streams
// or buffers.
func TestChapter18(t *testing.T) {
	grader.Suite(t, "18", "building the request body", []grader.Case{
		{
			ID:    "required-fields-always-present",
			Title: "sends model, messages and stream every time",
			Why: "A server needs all three to answer. Any of them missing is a 400 that " +
				"reads like a broken client.",
			Hints: hints.Get("18", "required-fields-always-present"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-m", "a-model").ExitCode); err != nil {
					return err
				}
				raw := srv.Last().Raw
				for _, key := range []string{"model", "messages", "stream"} {
					if _, ok := raw[key]; !ok {
						return &grader.Mismatch{Label: key, Want: "present in the body", Got: "absent"}
					}
				}
				if err := grader.Eqf("model", "a-model", raw["model"]); err != nil {
					return err
				}
				if err := grader.Eqf("stream", true, raw["stream"]); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "unset-optionals-absent",
			Title: "leaves unset optional fields out of the body",
			Why: "A provider reads max_tokens zero as produce nothing, and temperature " +
				"zero as a deliberate setting. Neither may be sent unless chosen.",
			Hints: hints.Get("18", "unset-optionals-absent"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				raw := srv.Last().Raw
				for _, key := range []string{"max_tokens", "temperature"} {
					if v, ok := raw[key]; ok {
						return &grader.Mismatch{Label: key, Want: "absent when unset", Got: v}
					}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "set-optionals-present",
			Title: "sends optional fields that were chosen",
			Why:   "Omitting them when unset only matters if the chosen values still arrive.",
			Hints: hints.Get("18", "set-optionals-present"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				res := blackbox.RunIn(t, dir, support.Env(srv.BaseURL()),
					"-p", "hi", "-max-tokens", "128", "-temperature", "0.4")
				if err := grader.Eqf("exit code", 0, res.ExitCode); err != nil {
					return err
				}
				raw := srv.Last().Raw
				if err := grader.Eqf("max_tokens", float64(128), raw["max_tokens"]); err != nil {
					return err
				}
				if err := grader.Eqf("temperature", 0.4, raw["temperature"]); err != nil {
					return err
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "messages-keep-their-order",
			Title: "sends messages in the order they were built",
			Why: "The conversation is a sequence. A body that reorders it, or drops a role, " +
				"changes the question being asked.",
			Hints: hints.Get("18", "messages-keep-their-order"),
			Run: func(t *testing.T) error {
				dir := support.DirWith(t, map[string]string{"AGENTS.md": "be brief\n"})
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "the question").ExitCode); err != nil {
					return err
				}
				msgs := srv.Last().Messages
				if len(msgs) < 2 {
					return &grader.Mismatch{Label: "messages", Want: "system then user", Got: support.Itoa(len(msgs))}
				}
				if msgs[0].Role != "system" {
					return &grader.Mismatch{Label: "messages[0].role", Want: "system", Got: msgs[0].Role}
				}
				if msgs[1].Role != "user" || msgs[1].Content != "the question" {
					return &grader.Mismatch{Label: "messages[1]", Want: "user: the question", Got: msgs[1].Role + ": " + msgs[1].Content}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "headers-decide-the-response",
			Title: "asks for a stream with the Accept header",
			Why: "Content-Type says what you sent. Accept says what you can read. A server " +
				"that does not see text/event-stream may answer with one buffered response, " +
				"and the parser waits forever.",
			Hints: hints.Get("18", "headers-decide-the-response"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi").ExitCode); err != nil {
					return err
				}
				req := srv.Last()
				if err := grader.Eqf("content-type", "application/json", req.ContentType); err != nil {
					return err
				}
				if !strings.Contains(req.Accept, "text/event-stream") {
					return &grader.Mismatch{Label: "accept", Want: "text/event-stream", Got: req.Accept}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
		{
			ID:    "base-url-trailing-slash",
			Title: "sends to the right path whether the base URL ends in a slash or not",
			Why: "A doubled slash produces a 404 that looks like a wrong route. Half the " +
				"'why is it 404' reports in the world are this.",
			Hints: hints.Get("18", "base-url-trailing-slash"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				base := srv.BaseURL() + "/"
				e := support.Env(base)
				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, e, "-p", "hi").ExitCode); err != nil {
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
			ID:    "empty-tool-list-is-sent",
			Title: "sends a tool list even when it is empty",
			Why: "A server distinguishes no tools from a request that never mentioned them. " +
				"Some reject the second.",
			Hints: hints.Get("18", "empty-tool-list-is-sent"),
			Run: func(t *testing.T) error {
				dir := support.Dir(t)
				srv := fake.New(t, fake.Reply{Text: "ok"})

				if err := grader.Eqf("exit code", 0,
					blackbox.RunIn(t, dir, support.Env(srv.BaseURL()), "-p", "hi", "-max-tools", "0").ExitCode); err != nil {
					return err
				}
				raw := srv.Last().Raw
				if _, ok := raw["tools"]; !ok {
					return &grader.Mismatch{Label: "tools", Want: "present, possibly empty", Got: "absent"}
				}
				if _, ok := raw["tools"].([]any); !ok {
					return &grader.Mismatch{Label: "tools", Want: "an array", Got: raw["tools"]}
				}
				srv.AssertConsumed(t)
				return nil
			},
		},
	})
}

// --- helpers ---
