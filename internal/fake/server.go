// Package fake is course infrastructure: a scriptable OpenAI-compatible server.
//
// It is not part of the harness you are building, and you should not need to
// read it to finish a chapter. It exists so that a model's behaviour -- which is
// normally random, slow, and paid for -- becomes a fixture you can assert on.
//
// This is the whole trick. An agent is not testable. The harness around the
// model is.
//
// Two ways to script a reply:
//
//	fake.Reply{Text: "hello"}                        // convenience: one text answer
//	fake.Reply{Frames: []fake.Frame{{Text: "a"}, {ToolCall: ...}}}  // exact stream
//
// The frame form exists because chapter 02 is *about* stream shape: fragments
// keyed by index, omitted ids on continuation fragments, text interleaved with
// tool calls, and SSE framing that is not just `data:` lines. A fixture language
// that cannot express the bug cannot grade the fix.
package fake

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Usage mirrors the token accounting a real server reports.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CallFrame is one streamed fragment of a tool call. Continuation fragments
// deliberately carry only Index and Args, matching real providers that omit
// ID and Name after the first fragment.
type CallFrame struct {
	Index int    // which tool call this fragment belongs to
	ID    string // set on the first fragment only
	Name  string // set on the first fragment only
	Args  string // appended to this index's argument buffer
}

// Frame is one emitted chunk. Zero-valued fields are omitted, so a Frame with
// only Text emits a content delta.
type Frame struct {
	Text     string     // content delta
	ToolCall *CallFrame // tool_calls delta
	Usage    *Usage     // metadata chunk
	Finish   string     // finish_reason: "stop", "tool_calls", ...
	Raw      []string   // emitted verbatim, with no "data: " prefix added
	Cut      bool       // close the connection after this frame
}

// Reply is one scripted response. Zero value is an empty text reply.
type Reply struct {
	// Convenience form, used when Frames is empty.
	Text      string
	ToolCalls []ScriptedCall

	// Exact form. When non-empty, Text/ToolCalls are ignored.
	Frames []Frame

	// OmitDone suppresses the trailing `data: [DONE]`, for tests that need to
	// prove the client handles a missing terminator.
	OmitDone bool

	// Status, if non-zero, makes the server respond with this HTTP status
	// instead of streaming.
	Status int
	Body   string

	// FailMidStream closes the connection after the first chunk, simulating a
	// provider dying mid-response.
	FailMidStream bool

	// Delay holds the response before the first byte, so a client timeout can be
	// tested without a real slow network.
	Delay time.Duration

	// FrameDelay pauses between frames, so a caller can be interrupted mid-stream.
	FrameDelay time.Duration

	// Usage is reported in the final chunk of the convenience form.
	Usage *Usage
}

// ScriptedCall is a tool call for the convenience form.
type ScriptedCall struct {
	ID   string
	Name string
	Args string
}

// RecordedMessage is what the client actually sent, decoded for assertions.
type RecordedMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	ToolCalls  []RecordedTC `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
}

// RecordedTC is a tool call as it appeared in the request.
type RecordedTC struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Recorded is one captured request.
type Recorded struct {
	Method        string
	Path          string
	Authorization string
	ContentType   string
	Accept        string
	Model         string
	Stream        bool
	Temperature   *float64
	MaxTokens     int
	Messages      []RecordedMessage
	Tools         []map[string]any
	Raw           map[string]any
	Header        http.Header
}

// Server is a scripted OpenAI-compatible endpoint.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	replies  []Reply
	cursor   int
	recorded []Recorded
	overrun  int
}

// New starts a server with the given scripted replies, in order. It shuts down
// when t ends.
func New(t *testing.T, replies ...Reply) *Server {
	t.Helper()
	s := &Server{replies: replies}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Server.Close)
	return s
}

// BaseURL is the value to hand to agent.NewOpenAICompat.
func (s *Server) BaseURL() string { return s.URL + "/v1" }

// Requests returns every captured request in arrival order.
func (s *Server) Requests() []Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Recorded, len(s.recorded))
	copy(out, s.recorded)
	return out
}

// Last returns the most recent captured request.
func (s *Server) Last() Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.recorded) == 0 {
		return Recorded{}
	}
	return s.recorded[len(s.recorded)-1]
}

// Overrun is how many requests arrived after the script was exhausted. Any
// non-zero value is a harness bug: it made an extra round trip the fixture did
// not anticipate.
func (s *Server) Overrun() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overrun
}

// Unused is how many scripted replies were never consumed. In a strict test that
// is also a bug, though a milder one: the conversation ended early.
func (s *Server) Unused() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.replies) - s.cursor
}

// AssertConsumed fails the test if the conversation did not use exactly the
// scripted number of turns. Call it at the end of any case that scripts more
// than one reply.
func (s *Server) AssertConsumed(t *testing.T) {
	t.Helper()
	if n := s.Overrun(); n > 0 {
		t.Errorf("the harness made %d request(s) beyond the script; it looped further than expected", n)
	}
	if n := s.Unused(); n > 0 {
		t.Errorf("%d scripted repl(ies) were never requested; the harness stopped early", n)
	}
}

func (s *Server) next() (Reply, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor >= len(s.replies) {
		s.overrun++
		return Reply{}, false
	}
	r := s.replies[s.cursor]
	s.cursor++
	return r, true
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	_ = json.NewDecoder(r.Body).Decode(&raw)

	rec := Recorded{
		Method:        r.Method,
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
		ContentType:   r.Header.Get("Content-Type"),
		Accept:        r.Header.Get("Accept"),
		Header:        r.Header.Clone(),
		Raw:           raw,
	}
	if v, ok := raw["model"].(string); ok {
		rec.Model = v
	}
	if v, ok := raw["stream"].(bool); ok {
		rec.Stream = v
	}
	if v, ok := raw["temperature"].(float64); ok {
		rec.Temperature = &v
	}
	if v, ok := raw["max_tokens"].(float64); ok {
		rec.MaxTokens = int(v)
	}
	if msgs, ok := raw["messages"].([]any); ok {
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			var rm RecordedMessage
			_ = json.Unmarshal(b, &rm)
			rec.Messages = append(rec.Messages, rm)
		}
	}
	if tools, ok := raw["tools"].([]any); ok {
		for _, t := range tools {
			if tm, ok := t.(map[string]any); ok {
				rec.Tools = append(rec.Tools, tm)
			}
		}
	}

	s.mu.Lock()
	s.recorded = append(s.recorded, rec)
	s.mu.Unlock()

	reply, ok := s.next()
	if reply.Delay > 0 {
		time.Sleep(reply.Delay)
	}
	if !ok {
		// An unscripted request is a failure, never a polite empty answer. A
		// harness that loops forever must not be able to "pass" on the final text.
		http.Error(w, "fake: script exhausted (the harness made an unexpected request)", http.StatusInternalServerError)
		return
	}

	if reply.Status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.Status)
		_, _ = fmt.Fprint(w, reply.Body)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	writeEvent := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeRaw := func(s string) {
		fmt.Fprint(w, s)
		if flusher != nil {
			flusher.Flush()
		}
	}

	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id":      "chatcmpl-fake",
			"object":  "chat.completion.chunk",
			"created": 1700000000,
			"model":   rec.Model,
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			}},
		}
	}

	// Convenience form: expand Text/ToolCalls into the canonical frame shape.
	if len(reply.Frames) == 0 {
		reply.Frames = expand(reply)
	}

	for _, fr := range reply.Frames {
		if reply.FrameDelay > 0 {
			time.Sleep(reply.FrameDelay)
		}
		if fr.Cut {
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
			return
		}
		for _, line := range fr.Raw {
			writeRaw(line)
		}
		if fr.Text != "" || fr.ToolCall == nil && fr.Usage == nil && fr.Finish == "" && len(fr.Raw) == 0 {
			writeEvent(chunk(map[string]any{"content": fr.Text}, nil))
			continue
		}
		if fr.ToolCall != nil {
			inner := map[string]any{"arguments": fr.ToolCall.Args}
			if fr.ToolCall.ID != "" {
				inner["name"] = fr.ToolCall.Name
			}
			tc := map[string]any{"index": fr.ToolCall.Index, "function": inner}
			if fr.ToolCall.ID != "" {
				tc["id"] = fr.ToolCall.ID
				tc["type"] = "function"
			}
			writeEvent(chunk(map[string]any{"tool_calls": []any{tc}}, nil))
			continue
		}
		if fr.Usage != nil || fr.Finish != "" {
			finish := any(nil)
			if fr.Finish != "" {
				finish = fr.Finish
			}
			ev := chunk(map[string]any{}, finish)
			if fr.Usage != nil {
				ev["usage"] = *fr.Usage
			}
			writeEvent(ev)
		}
	}

	if !reply.OmitDone {
		writeRaw("data: [DONE]\n\n")
	}
}

// expand turns the convenience fields into an explicit frame list.
func expand(reply Reply) []Frame {
	var frames []Frame
	frames = append(frames, Frame{Text: ""}) // role chunk, like the real API

	if reply.FailMidStream {
		return append(frames, Frame{Text: "partial"}, Frame{Cut: true})
	}

	for _, frag := range split(reply.Text, 4) {
		if frag == "" {
			continue
		}
		frames = append(frames, Frame{Text: frag})
	}

	for i, tc := range reply.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		frames = append(frames, Frame{ToolCall: &CallFrame{Index: i, ID: id, Name: tc.Name, Args: ""}})
		for _, frag := range split(tc.Args, 3) {
			if frag == "" {
				continue
			}
			frames = append(frames, Frame{ToolCall: &CallFrame{Index: i, Args: frag}})
		}
	}

	usage := reply.Usage
	if usage == nil {
		usage = &Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}
	}
	finish := "stop"
	if len(reply.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	return append(frames, Frame{Finish: finish, Usage: usage})
}

// split cuts s into n roughly equal pieces, so streamed fragments are
// deterministic and easier to reason about than random chunking.
func split(s string, n int) []string {
	if s == "" {
		return nil
	}
	if n < 1 {
		n = 1
	}
	size := (len(s) + n - 1) / n
	var out []string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}

// ErrScriptExhausted is returned by helpers that need a scripted turn and find
// none. Callers normally assert on Server.Overrun instead.
var ErrScriptExhausted = errors.New("fake: script exhausted")
