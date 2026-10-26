// Package harness is the coding-agent harness you are building.
//
// This file is scaffolding, not exercise material. It defines the seam every
// chapter is graded through: messages in, messages out. You own everything
// behind it. Change these types only if you also change the checks in
// chapters/*/checks, which is allowed but not recommended -- the point of a
// stable seam is that next chapter's tests still compile.
package harness

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotImplemented is returned by stubs. A check that sees this is telling you
// "this chapter is not started", not "your logic is wrong".
var ErrNotImplemented = errors.New("harness: not implemented yet")

// Message roles. These mirror the OpenAI wire format, which uses a flat list of
// messages rather than typed content blocks.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one entry in a conversation.
//
// A single struct covers all four roles, because that is what the wire format
// does. Which fields are meaningful depends on Role:
//
//	system, user  Content
//	assistant     Content and/or ToolCalls
//	tool          Content and ToolCallID (the call this result answers)
type Message struct {
	Role    string
	Content string

	// ToolCalls is non-empty only on assistant messages that requested tools.
	ToolCalls []ToolCall

	// ToolCallID is set only on tool messages.
	ToolCallID string

	// Name is optional on tool messages; some providers echo the tool name.
	Name string
}

// ToolCall is a model's request to invoke a tool. Arguments is the raw JSON
// string exactly as the model produced it -- do not unmarshal it here. The
// tool decides how to decode its own arguments.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolDef advertises a tool to the model. Parameters is a JSON Schema object.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Usage is token accounting as reported by the provider.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Request is one model call.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature float64
}

// Provider is the seam. Everything above it (loop, sessions, context) is
// deterministic and testable; everything below it is somebody else's server.
type Provider interface {
	Chat(ctx context.Context, req Request) (Message, Usage, error)
}

// HTTPError is a non-2xx response from the provider. Checks assert on Status,
// so keep the shape: status code plus a truncated body.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	body := e.Body
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return fmt.Sprintf("provider: HTTP %d: %s", e.Status, body)
}
