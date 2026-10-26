package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultReadLimit is how much of a file the model sees in one call.
//
// Model context is the scarce resource, so a 2 MB log must not be pasted into
// it. The cut is marked, because silent truncation teaches the model that it saw
// the whole file when it did not.
const DefaultReadLimit = 20_000

// ReadTool returns the contents of a text file.
//
// Root is the workspace. Paths are resolved inside it, and anything that escapes
// is refused: a coding agent that can read /etc/shadow on request is not a
// tool-call demo, it is a security incident.
type ReadTool struct {
	Root     string
	MaxBytes int
}

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
	return "Read a text file from the workspace. Returns the contents, truncated if the file is large."
}

func (t *ReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path relative to the workspace root.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", errors.New("path is required")
	}

	full, err := t.resolve(args.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such file: %s", args.Path)
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("is a directory: %s", args.Path)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	limit := t.MaxBytes
	if limit <= 0 {
		limit = DefaultReadLimit
	}

	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, limit+1)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}

	if n > limit {
		return string(buf[:limit]) + fmt.Sprintf(
			"\n... [truncated: %d of %d bytes shown]", limit, info.Size()), nil
	}
	return string(buf[:n]), nil
}

// resolve turns a model-supplied path into an absolute path inside Root, or
// refuses it. The check is on the resolved path, so "a/../../etc/passwd" is
// caught as well as "../etc/passwd".
func (t *ReadTool) resolve(p string) (string, error) {
	root := t.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	candidate := p
	if filepath.IsAbs(candidate) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	full, err := filepath.Abs(filepath.Join(absRoot, candidate))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absRoot, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace: %s", p)
	}
	return full, nil
}
