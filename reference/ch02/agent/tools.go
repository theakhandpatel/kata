// Chapter 02 reference: no tools yet.
//
// This chapter is about the protocol, so the registry can be empty. Chapter 03 adds
// the read tool and the file handling that goes with it.
package agent

import (
	"context"
	"encoding/json"
)

// StubTool makes the chapter's protocol testable without a real tool.
//
// It exists so the registry can be exercised: a tool that answers one fixed string.
// Chapter 03 replaces this with something that reads files.
type StubTool struct{}

func (t *StubTool) Name() string        { return "read" }
func (t *StubTool) Description() string { return "Read a file. (Chapter 02 placeholder.)" }
func (t *StubTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}
}
func (t *StubTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	return "stub", nil
}
