package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Limits on what a tool may return. Model context is the scarce resource, so
// every tool that can produce a lot of text has a ceiling.
const (
	DefaultReadLimit = 20_000
	DefaultBashLimit = 20_000
	DefaultBashWait  = 10 * time.Second
)

// ReadTool returns the contents of a text file.
type ReadTool struct {
	Root     string // workspace root; paths outside it are refused
	MaxBytes int    // 0 means DefaultReadLimit
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

	full, err := resolveInside(t.Root, args.Path)
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

// WriteTool creates or replaces a file in the workspace.
//
// It reports the byte count so the model can confirm what happened without
// reading the file back, which would spend context twice.
type WriteTool struct {
	Root string
}

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string {
	return "Write a text file in the workspace. Creates parent directories. Replaces the file if it exists."
}

func (t *WriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path relative to the workspace root.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The full contents to write.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", errors.New("path is required")
	}

	full, err := resolveInside(t.Root, args.Path)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}

// BashTool runs a shell command in the workspace.
//
// The result always reports an exit code, including a non-zero one. A command
// that exits 1 is a normal outcome the model can read and react to; only a
// failure to start the process, or a cancelled run, is an error.
type BashTool struct {
	Root     string
	Timeout  time.Duration // 0 means DefaultBashWait
	MaxBytes int           // 0 means DefaultBashLimit
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Run a shell command in the workspace and return its combined output and exit code."
}

func (t *BashTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command line to run with sh -c.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", errors.New("command is required")
	}

	wait := t.Timeout
	if wait <= 0 {
		wait = DefaultBashWait
	}
	runCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", args.Command)
	cmd.Dir = t.Root
	// Kill the child on cancel rather than leaving it running. Grandchildren are
	// a known limitation; see the chapter notes.
	cmd.WaitDelay = time.Second

	out, err := cmd.CombinedOutput()
	body := t.cap(string(out))

	if err != nil {
		// The caller cancelled, or the run hit its own timeout. A timeout is a
		// result: the model asked for something that took too long and can decide
		// what to do next. A cancelled run is an error: the user aborted.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fmt.Sprintf("command timed out after %s\n%s", wait, body), nil
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Sprintf("exit code: %d\n%s", exit.ExitCode(), body), nil
		}
		return "", err
	}
	return "exit code: 0\n" + body, nil
}

func (t *BashTool) cap(s string) string {
	limit := t.MaxBytes
	if limit <= 0 {
		limit = DefaultBashLimit
	}
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n... [truncated: %d of %d bytes shown]", limit, len(s))
}

// resolveInside turns a model-supplied path into an absolute path inside root,
// or refuses it.
//
// The check runs on the resolved path, so "a/../../etc/passwd" is caught as well
// as "../etc/passwd". Absolute paths are refused outright: the model has no
// business naming a path outside the workspace.
func resolveInside(root, p string) (string, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}

	full, err := filepath.Abs(filepath.Join(absRoot, p))
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
