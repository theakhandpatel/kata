package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// DefaultMaxContext is the token budget when a caller sets none.
const DefaultMaxContext = 8000

// EstimateTokens returns a rough token count for a request.
//
// It is deliberately crude: four bytes per token, plus a small fixed cost per
// message and per tool schema. A real tokenizer would be exact and would need a
// dependency, a vocabulary file, and a model-specific table. For deciding what to
// drop, crude is enough, because the decision has a wide margin.
//
// The estimate must be monotonic: adding content never lowers it. A budget that
// moves the wrong way makes pruning oscillate.
func EstimateTokens(msgs []Message, tools []ToolDef) int {
	bytes := 0
	for _, m := range msgs {
		// Role and content, plus a fixed per-message envelope cost.
		bytes += len(m.Role) + len(m.Content) + 4
		for _, tc := range m.ToolCalls {
			bytes += len(tc.ID) + len(tc.Name) + len(tc.Arguments)
		}
	}
	for _, t := range tools {
		bytes += len(t.Name) + len(t.Description)
		if schema, err := json.Marshal(t.Parameters); err == nil {
			bytes += len(schema)
		}
	}
	return bytes / 4
}

// Fit returns a message list that fits the budget, and how many messages it had
// to drop.
//
// The rules, in priority order:
//
//  1. the system message is kept, because policy is not negotiable;
//  2. the newest messages are kept, because the current task is what the model
//     needs;
//  3. a tool result is never kept without the call it answers;
//  4. every drop is replaced by one marker, so the model knows it is missing
//     history rather than assuming the conversation started there.
func Fit(msgs []Message, tools []ToolDef, budget int) ([]Message, int) {
	if budget <= 0 {
		budget = DefaultMaxContext
	}
	if EstimateTokens(msgs, tools) <= budget {
		return msgs, 0
	}

	var system []Message
	rest := msgs
	if len(msgs) > 0 && msgs[0].Role == RoleSystem {
		system = msgs[:1]
		rest = msgs[1:]
	}

	// Walk backwards, so the tail is what survives.
	kept := make([]Message, 0, len(rest))
	for i := len(rest) - 1; i >= 0; i-- {
		candidate := append([]Message{rest[i]}, kept...)
		trial := append(append([]Message{}, system...), candidate...)
		// Always keep at least one message: a single oversized message is a fact
		// about the request, not something pruning can fix.
		if len(kept) > 0 && EstimateTokens(trial, tools) > budget {
			break
		}
		kept = candidate
	}

	dropped := len(rest) - len(kept)

	// A tool result at the boundary lost its call. Drop the orphan rather than
	// send a conversation a provider will reject.
	for len(kept) > 0 && kept[0].Role == RoleTool {
		kept = kept[1:]
		dropped++
	}

	out := make([]Message, 0, len(system)+1+len(kept))
	out = append(out, system...)
	if dropped > 0 {
		out = append(out, Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("[context pruned: %d earlier messages omitted to fit %d tokens]", dropped, budget),
		})
	}
	out = append(out, kept...)
	return out, dropped
}

// WithBudget wraps a provider so that every request is fitted to a token budget
// before it is sent.
//
// This is a decorator rather than a change to the loop, and that is the point: the
// loop sends a conversation, and where a conversation comes from is not its
// business. Chapter 01 built the seam; this is what it was for.
func WithBudget(p StreamingProvider, tools *Registry, budget int, emit Emitter) StreamingProvider {
	if budget <= 0 {
		return p
	}
	if emit == nil {
		emit = func(Event) {}
	}
	return &budgetedProvider{inner: p, tools: tools, budget: budget, emit: emit}
}

type budgetedProvider struct {
	inner  StreamingProvider
	tools  *Registry
	budget int
	emit   Emitter
	calls  int
}

func (b *budgetedProvider) Chat(ctx context.Context, req Request) (Message, Usage, error) {
	return b.Stream(ctx, req, nil)
}

func (b *budgetedProvider) Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error) {
	b.calls++

	fitted, pruned := Fit(req.Messages, req.Tools, b.budget)
	b.emit(Event{
		Type:   EventBudget,
		Turn:   b.calls,
		Tokens: EstimateTokens(fitted, req.Tools),
		Budget: b.budget,
		Pruned: pruned,
	})

	req.Messages = fitted
	return b.inner.Stream(ctx, req, onText)
}
