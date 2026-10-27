package agent

import (
	"context"
	"errors"
	"fmt"
)

// DefaultMaxSteps bounds a run when the caller sets no limit.
const DefaultMaxSteps = 20

// ErrStepLimit is returned when a run exceeds its step budget.
var ErrStepLimit = errors.New("step limit reached")

// StreamingProvider is a Provider that can report text as it arrives.
type StreamingProvider interface {
	Provider
	Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error)
}

// LoopOptions controls one run.
type LoopOptions struct {
	// MaxSteps caps the number of model requests. Zero means DefaultMaxSteps.
	MaxSteps int
	// Emit receives events. Nil discards them.
	Emit Emitter
	// MaxTokens and Temperature are copied into every request. Zero means unset, and
	// an unset value is left out of the request body rather than sent as zero.
	MaxTokens   int
	Temperature float64
	// Steering is drained at every turn boundary. Nil means nobody can interrupt.
	Steering func() []Message
}

// Result is what a finished run produced.
type Result struct {
	Message Message
	History []Message
	Usage   Usage
	Turns   int
}

// RunLoop drives the model until it stops asking for tools.
//
// The order inside one turn matters, and every step is graded:
//
//  1. ask the model, reporting text as it streams;
//  2. append the assistant message;
//  3. run every call in the order the model asked, appending one result each;
//  4. drain steering, appending anything that arrived as new user messages;
//  5. stop only when the model asked for no tools *and* nothing was steered in.
func RunLoop(
	ctx context.Context,
	provider StreamingProvider,
	tools *Registry,
	history []Message,
	opts LoopOptions,
) (Result, error) {
	max := opts.MaxSteps
	if max <= 0 {
		max = DefaultMaxSteps
	}
	emit := opts.Emit
	if emit == nil {
		emit = func(Event) {}
	}

	var total Usage

	for turn := 1; turn <= max; turn++ {
		if err := ctx.Err(); err != nil {
			emit(Event{Type: EventAgentEnd, Turns: turn - 1, Reason: "aborted"})
			return Result{History: history, Usage: total, Turns: turn - 1}, err
		}

		emit(Event{Type: EventTurnStart, Turn: turn})

		msg, usage, err := provider.Stream(ctx, Request{
			Messages:    history,
			Tools:       tools.Defs(),
			MaxTokens:   opts.MaxTokens,
			Temperature: opts.Temperature,
		}, func(text string) {
			emit(Event{Type: EventText, Turn: turn, Text: text})
		})
		if err != nil {
			// Keep what arrived. An answer cut in half is still worth printing, and
			// losing it makes an interrupted run indistinguishable from a failed one.
			if msg.Content != "" {
				history = append(history, msg)
			}
			reason := "error"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				reason = "aborted"
			}
			emit(Event{Type: EventAgentEnd, Turns: turn, Reason: reason})
			return Result{Message: msg, History: history, Usage: total, Turns: turn}, err
		}

		total = addUsage(total, usage)
		history = append(history, msg)
		askedForTools := len(msg.ToolCalls) > 0

		for _, call := range msg.ToolCalls {
			emit(Event{Type: EventToolCall, Turn: turn, ID: call.ID, Name: call.Name})
		}
		for _, call := range msg.ToolCalls {
			res := tools.Run(ctx, call)
			emit(Event{
				Type: EventToolResult, Turn: turn,
				ID: call.ID, Name: call.Name,
				IsError: res.IsError, Bytes: len(res.Content),
			})
			history = append(history, Message{
				Role:       RoleTool,
				Content:    res.Content,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}

		steered := false
		if opts.Steering != nil {
			for _, m := range opts.Steering() {
				emit(Event{Type: EventSteer, Turn: turn, Text: m.Content})
				history = append(history, m)
				steered = true
			}
		}

		emit(Event{Type: EventTurnEnd, Turn: turn})

		if !askedForTools && !steered {
			emit(Event{Type: EventAgentEnd, Turns: turn, Reason: "done"})
			return Result{Message: msg, History: history, Usage: total, Turns: turn}, nil
		}
	}

	emit(Event{Type: EventAgentEnd, Turns: max, Reason: "step_limit"})
	return Result{History: history, Usage: total, Turns: max},
		fmt.Errorf("%w after %d turns; the model kept asking for tools", ErrStepLimit, max)
}

func addUsage(a, b Usage) Usage {
	return Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
	}
}
