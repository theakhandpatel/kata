// Package agent is the coding agent you are building.
//
// This file is scaffolding, not exercise material. It defines the vocabulary
// the rest of the program shares, and the one interface the provider
// implementations satisfy. You own everything behind it: change these types if
// your design needs it, but keep the meaning.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotImplemented is returned by stubs. A check that sees this is telling you
// "this chapter is not started", not "your logic is wrong".
var ErrNotImplemented = errors.New("agent: not implemented yet")

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
	Role    string `json:"role"`
	Content string `json:"content"`

	// ToolCalls is non-empty only on assistant messages that requested tools.
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`

	// ToolCallID is set only on tool messages.
	ToolCallID string `json:"toolCallId,omitempty"`

	// Name is optional on tool messages; some providers echo the tool name.
	Name string `json:"name,omitempty"`
}

// ToolCall is a model's request to invoke a tool. Arguments is the raw JSON
// string exactly as the model produced it -- do not unmarshal it here. The
// tool decides how to decode its own arguments.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef advertises a tool to the model. Parameters is a JSON Schema object.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Usage is token accounting as reported by the provider.
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
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

	// RetryAfter is what the server asked for in its Retry-After header, if any.
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	body := e.Body
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return fmt.Sprintf("provider: HTTP %d: %s", e.Status, body)
}

// Tool is one capability the model can call. The agent advertises Name,
// Description and Parameters, then runs Execute when the model asks for it.
//
// Execute returns the text the model will see. A returned error means the tool
// could not run at all; the agent turns it into an error result rather than
// stopping the conversation.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// ToolResult is one tool's output, ready to append to the conversation.
type ToolResult struct {
	Content string
	IsError bool
}
