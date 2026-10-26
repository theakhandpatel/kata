package agent

import (
	"encoding/json"
	"io"
)

// Event is one observable step in a run.
//
// Events exist so that a terminal UI, a log file, or a test can follow the loop's
// decisions in order without reconstructing them from the conversation. The
// conversation says what was said; the events say what the program did.
type Event struct {
	Type string `json:"type"`

	// Turn is 1-based and groups everything that belongs to one model request.
	Turn int `json:"turn,omitempty"`

	// ID and Name identify a tool call. A result repeats the call's ID, which is
	// what lets a reader pair them.
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`

	// Text carries one content delta.
	Text string `json:"text,omitempty"`

	// IsError and Bytes describe a tool result without copying its content into
	// the event stream.
	IsError bool `json:"isError,omitempty"`
	Bytes   int  `json:"bytes,omitempty"`

	// Turns and Reason describe the end of a run.
	Turns  int    `json:"turns,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Event types. The sequence is the grammar a run must satisfy:
//
//	turn_start text* tool_call+ tool_result+ turn_end  (repeated)
//	agent_end
const (
	EventTurnStart  = "turn_start"
	EventText       = "text"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventTurnEnd    = "turn_end"
	EventAgentEnd   = "agent_end"
)

// Emitter receives events. A nil Emitter discards them, which is what a caller
// that only wants the answer passes.
type Emitter func(Event)

// JSONEmitter writes one JSON object per line, flushed per event so a reader can
// follow a long run.
func JSONEmitter(w io.Writer) Emitter {
	enc := json.NewEncoder(w)
	return func(e Event) {
		_ = enc.Encode(e)
	}
}
