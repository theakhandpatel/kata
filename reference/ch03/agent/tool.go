package agent

import (
	"context"
	"encoding/json"
)

// Registry is the set of tools the agent can advertise and run.
//
// The model never calls code directly. It names a tool and sends JSON. The
// registry is the lookup between that name and an implementation, and its Defs
// method is what turns implementations into the schema list the provider sends.
type Registry struct {
	byName map[string]Tool
	order  []string
}

// NewRegistry builds a registry from the given tools. The order is preserved so
// the advertised schema list is stable across requests: an unstable prefix costs
// provider prompt-cache hits.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, exists := r.byName[t.Name()]; exists {
			continue
		}
		r.byName[t.Name()] = t
		r.order = append(r.order, t.Name())
	}
	return r
}

// Lookup finds a tool by the name the model sent.
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// Names lists the registered tools in declaration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Defs converts the registry into the schema list a provider sends.
func (r *Registry) Defs() []ToolDef {
	out := make([]ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		out = append(out, ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return out
}

// Run executes one tool call and converts the outcome into a tool result.
//
// A lookup failure, bad arguments, or an error from the tool all become
// IsError results rather than aborting the run. The model can read the message
// and correct itself, which is the whole reason the result carries an error flag
// as well as text.
func (r *Registry) Run(ctx context.Context, call ToolCall) ToolResult {
	t, ok := r.Lookup(call.Name)
	if !ok {
		return ToolResult{Content: "no such tool: " + call.Name, IsError: true}
	}

	if !json.Valid([]byte(call.Arguments)) {
		return ToolResult{
			Content: "arguments are not valid JSON: " + call.Arguments,
			IsError: true,
		}
	}

	out, err := t.Execute(ctx, json.RawMessage(call.Arguments))
	if err != nil {
		return ToolResult{Content: t.Name() + ": " + err.Error(), IsError: true}
	}
	return ToolResult{Content: out}
}
