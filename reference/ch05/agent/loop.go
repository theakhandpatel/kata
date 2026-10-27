package agent

import (
	"context"
	"errors"
	"fmt"
)

// DefaultMaxSteps bounds a run when the caller sets no limit.
//
// A positive default matters. Some harnesses treat zero as unbounded, which
// turns a model that keeps asking for tools into a program that never returns.
const DefaultMaxSteps = 20

// StreamingProvider is a Provider that can report text as it arrives.
//
// It is declared here rather than added to Provider so that the earlier chapters
// keep compiling: a provider that cannot stream is still a provider.
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
}

// Result is what a finished run produced.
type Result struct {
	// Message is the final assistant message, the one that asked for no tools.
	Message Message
	// History is the conversation including every tool call and result.
	History []Message
	// Usage is summed across every request in the run.
	Usage Usage
	// Turns is how many model requests the run made.
	Turns int
}

// RunLoop drives the model until it stops asking for tools.
//
// The rules, in order, and each one is graded:
//
//  1. Ask for a turn, reporting text as it streams.
//  2. Append the assistant message, whether or not it asked for tools.
//  3. If it asked for no tools, the run is done.
//  4. Otherwise run every call, in the order the model asked for them, and
//     append one result per call, carrying the call's id.
//  5. Stop after MaxSteps requests, and say so rather than returning a partial
//     answer as if it were final.
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
			return Result{History: history, Usage: total, Turns: turn}, err
		}
		total = addUsage(total, usage)
		history = append(history, msg)

		if len(msg.ToolCalls) == 0 {
			emit(Event{Type: EventTurnEnd, Turn: turn})
			emit(Event{Type: EventAgentEnd, Turns: turn, Reason: "done"})
			return Result{Message: msg, History: history, Usage: total, Turns: turn}, nil
		}

		// Announce every call before running any of them. A reader that only sees
		// results cannot tell a slow tool from a missing one.
		for _, call := range msg.ToolCalls {
			emit(Event{Type: EventToolCall, Turn: turn, ID: call.ID, Name: call.Name})
		}

		// Execute and append in the model's order. Providers pair results with
		// calls by id, and a conversation whose order drifts is rejected.
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

		emit(Event{Type: EventTurnEnd, Turn: turn})
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

// ErrStepLimit is returned when a run exceeds its step budget. Callers can test
// for it with errors.Is to print a friendlier message.
var ErrStepLimit = errors.New("step limit reached")
