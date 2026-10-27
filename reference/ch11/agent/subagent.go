package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DefaultSubagentSteps bounds a child run. It is smaller than a parent's budget on
// purpose: a child that cannot finish in a few turns is usually solving the wrong
// problem, and the parent is waiting for it.
const DefaultSubagentSteps = 8

// TaskTool hands a self-contained job to a child run and returns its answer.
//
// Three properties matter, and each is graded:
//
//   - the child starts from the task prompt, not from the parent's conversation;
//   - the child's registry is the one passed in, so leaving this tool out of it
//     makes recursion impossible rather than merely discouraged;
//   - the parent receives exactly one result per call, including when the child
//     fails.
type TaskTool struct {
	// Provider is what the child calls. Use the same one the parent uses.
	Provider StreamingProvider
	// Tools is the child's tool set. It must not contain another TaskTool.
	Tools *Registry
	// System is the system message the child starts from.
	System string
	// MaxSteps caps the child, and the parent is blocked until it returns.
	MaxSteps int
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Delegate a self-contained job to a subagent. The subagent starts with no conversation history and returns its final answer. Use it for work that would flood this conversation with detail."
}

func (t *TaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "A short label for the job, used in logs.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The complete instruction for the subagent. It cannot see this conversation.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return "", errors.New("prompt is required: the subagent cannot see the parent's conversation")
	}
	if t.Provider == nil {
		return "", errors.New("subagent has no provider")
	}
	if t.Tools == nil {
		return "", errors.New("subagent has no tools")
	}

	// Fresh context. The system message plus the task prompt, nothing else, which
	// is what makes a subagent a way to keep the parent's history small.
	history := []Message{
		{Role: RoleSystem, Content: t.System},
		{Role: RoleUser, Content: args.Prompt},
	}

	steps := t.MaxSteps
	if steps <= 0 {
		steps = DefaultSubagentSteps
	}

	res, err := RunLoop(ctx, t.Provider, t.Tools, history, LoopOptions{MaxSteps: steps})
	if err != nil {
		// The parent gets a failed result rather than a silent success. Include the
		// partial answer, because a half-finished child is still informative.
		partial := strings.TrimSpace(res.Message.Content)
		if partial == "" {
			return "", fmt.Errorf("subagent failed: %w", err)
		}
		return "", fmt.Errorf("subagent failed after %d turns: %w (partial answer: %s)",
			res.Turns, err, truncate(partial, 500))
	}

	answer := strings.TrimSpace(res.Message.Content)
	if answer == "" {
		return "", errors.New("subagent returned no answer")
	}
	return answer, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
