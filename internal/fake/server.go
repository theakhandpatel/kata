// Package fake is course infrastructure: a scriptable OpenAI-compatible server.
//
// It is not part of the harness you are building, and you should not need to
// read it to finish a chapter. It exists so that a model's behaviour -- which is
// normally random, slow, and paid for -- becomes a fixture you can assert on.
//
// This is the whole trick. An agent is not testable. The harness around the
// model is.
package fake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ToolCall is a scripted tool call. Args is emitted in fragments to force the
// client to reassemble streamed argument deltas, which is where real harnesses
// break.
type ToolCall struct {
	ID   string
	Name string
	Args string
}

// Reply is one scripted response. Zero value is a plain text reply.
type Reply struct {
	Text      string
	ToolCalls []ToolCall

	// Status, if non-zero, makes the server respond with this HTTP status
	// instead of streaming. Body is sent as the response body.
	Status int
	Body   string

	// FailMidStream closes the connection after the first chunk, simulating a
	// provider dying mid-response.
	FailMidStream bool

	// Usage is reported in the final chunk. Zero means the server fills in
	// plausible numbers.
	Usage *Usage
}

// Usage mirrors the token accounting a real server reports.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
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

// BaseURL is the value to hand to harness.NewOpenAICompat.
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

func (s *Server) next() Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor >= len(s.replies) {
		// Out of script: answer politely rather than hanging.
		return Reply{Text: ""}
	}
	r := s.replies[s.cursor]
	s.cursor++
	return r
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	body := json.NewDecoder(r.Body)
	_ = body.Decode(&raw)

	rec := Recorded{
		Method:        r.Method,
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
		ContentType:   r.Header.Get("Content-Type"),
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

	reply := s.next()

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
	emit := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	usage := reply.Usage
	if usage == nil {
		usage = &Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}
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

	// First chunk carries the role, exactly like the real API.
	emit(chunk(map[string]any{"role": "assistant", "content": ""}, nil))

	if reply.FailMidStream {
		emit(chunk(map[string]any{"content": "partial"}, nil))
		if flusher != nil {
			flusher.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
			}
		}
		return
	}

	// Text arrives in small fragments. A client that overwrites instead of
	// appending will look right on one chunk and wrong on two.
	for _, frag := range split(reply.Text, 4) {
		if frag == "" {
			continue
		}
		emit(chunk(map[string]any{"content": frag}, nil))
	}

	// Tool calls: the id/name arrive in the first fragment for a given index,
	// the arguments stream in pieces afterwards, and later fragments omit
	// id/name entirely. That is the shape that breaks naive parsers.
	for i, tc := range reply.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		emit(chunk(map[string]any{"tool_calls": []any{map[string]any{
			"index": i,
			"id":    id,
			"type":  "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": "",
			},
		}}}, nil))

		for _, frag := range split(tc.Args, 3) {
			if frag == "" {
				continue
			}
			emit(chunk(map[string]any{"tool_calls": []any{map[string]any{
				"index": i,
				"function": map[string]any{
					"arguments": frag,
				},
			}}}, nil))
		}
	}

	finish := "stop"
	if len(reply.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	final := chunk(map[string]any{}, finish)
	final["usage"] = map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	emit(final)

	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
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
