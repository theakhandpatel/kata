package fake

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Meta-tests for the fixture language itself. If the fake cannot express a
// stream shape, no chapter can grade code that handles it -- so these tests are
// prerequisites for chapter 02, not polish.

func post(t *testing.T, url string, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

// chunks decodes every `data: ` line into a generic map.
func chunks(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("chunk is not JSON: %q: %v", payload, err)
		}
		out = append(out, m)
	}
	return out
}

func delta(m map[string]any) map[string]any {
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	c, _ := choices[0].(map[string]any)
	d, _ := c["delta"].(map[string]any)
	return d
}

func TestConvenienceTextIsSplitAndTerminated(t *testing.T) {
	s := New(t, Reply{Text: "the quick brown fox"})
	_, body := post(t, s.URL, `{"model":"m","stream":true}`)

	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream is not terminated with [DONE]:\n%s", body)
	}

	var got strings.Builder
	textFrames := 0
	for _, c := range chunks(t, body) {
		if d := delta(c); d != nil {
			if s, ok := d["content"].(string); ok && s != "" {
				got.WriteString(s)
				textFrames++
			}
		}
	}
	if got.String() != "the quick brown fox" {
		t.Fatalf("fragments do not reassemble: %q", got.String())
	}
	if textFrames < 2 {
		t.Fatalf("text was not split across chunks (%d frames); a one-chunk fixture cannot catch assignment-instead-of-append", textFrames)
	}
}

func TestFramesCanInterleaveAndOmitIDs(t *testing.T) {
	s := New(t, Reply{Frames: []Frame{
		{Text: "thinking"},
		{ToolCall: &CallFrame{Index: 0, ID: "call_1", Name: "read"}},
		{ToolCall: &CallFrame{Index: 0, Args: `{"path":`}},
		{Text: "still thinking"},
		{ToolCall: &CallFrame{Index: 0, Args: `"a.txt"}`}},
		{Finish: "tool_calls"},
	}})
	_, body := post(t, s.URL, `{"model":"m","stream":true}`)

	var order []string
	var args, firstID, secondID string
	for _, c := range chunks(t, body) {
		d := delta(c)
		if d == nil {
			continue
		}
		if txt, ok := d["content"].(string); ok && txt != "" {
			order = append(order, "text:"+txt)
		}
		if tcs, ok := d["tool_calls"].([]any); ok {
			order = append(order, "call")
			tc := tcs[0].(map[string]any)
			fn, _ := tc["function"].(map[string]any)
			if a, ok := fn["arguments"].(string); ok {
				args += a
			}
			if id, ok := tc["id"].(string); ok {
				if firstID == "" {
					firstID = id
				} else {
					secondID = id
				}
			}
		}
	}

	want := []string{"text:thinking", "call", "call", "text:still thinking", "call"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("frame order not preserved:\n got %v\nwant %v", order, want)
	}
	if firstID != "call_1" || secondID != "" {
		t.Fatalf("continuation fragment should omit id: first=%q second=%q", firstID, secondID)
	}
	if args != `{"path":"a.txt"}` {
		t.Fatalf("argument fragments do not reassemble: %q", args)
	}
}

func TestOmitDone(t *testing.T) {
	s := New(t, Reply{Text: "x", OmitDone: true})
	_, body := post(t, s.URL, `{"model":"m","stream":true}`)
	if strings.Contains(body, "[DONE]") {
		t.Fatal("[DONE] was emitted despite OmitDone")
	}
}

func TestOverrunIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	s := New(t, Reply{Text: "only one turn"})

	post(t, s.URL, `{"model":"m","stream":true}`)
	resp, _ := post(t, s.URL, `{"model":"m","stream":true}`)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("an unscripted request returned %d; it must fail loudly so a runaway loop cannot pass", resp.StatusCode)
	}
	if got := s.Overrun(); got != 1 {
		t.Fatalf("overrun counter = %d, want 1", got)
	}
}

func TestAssertConsumedDetectsUnusedReplies(t *testing.T) {
	s := New(t, Reply{Text: "one"}, Reply{Text: "two"})
	post(t, s.URL, `{"model":"m","stream":true}`)

	if got := s.Unused(); got != 1 {
		t.Fatalf("unused = %d, want 1", got)
	}
}

func TestRecordedCapturesEnvelope(t *testing.T) {
	s := New(t, Reply{Text: "x"})
	req, _ := http.NewRequest(http.MethodPost, s.URL+"/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"max_tokens":7}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer k")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	r := s.Last()
	if r.ContentType != "application/json" || r.Accept != "text/event-stream" {
		t.Fatalf("headers not recorded: %+v", r)
	}
	if r.Authorization != "Bearer k" {
		t.Fatalf("authorization not recorded: %q", r.Authorization)
	}
	if r.Raw["max_tokens"].(float64) != 7 {
		t.Fatalf("raw body not recorded: %+v", r.Raw)
	}
}
