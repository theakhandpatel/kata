package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultSpillLimit is how many bytes a tool result may occupy before it is moved to
// disk and replaced with a handle.
const DefaultSpillLimit = 8_000

// DefaultSliceBytes is how much one fetch returns.
const DefaultSliceBytes = 8_000

// SpillStore keeps large tool output on disk and gives the model a short handle.
//
// A tool result competes with the conversation for the same budget. Cutting it at a
// byte count keeps the budget and loses the information. Moving it to a file keeps
// both, as long as the model can ask for a part of it afterwards.
type SpillStore struct {
	Dir string
}

// Put writes a result to disk and returns the string the model sees, and whether the
// content was moved.
//
// Small results pass through untouched. Large ones become a short summary plus a
// handle, because a model that receives a bare path has no way to read it.
func (s *SpillStore) Put(name, content string, limit int) (string, bool, error) {
	if limit <= 0 {
		limit = DefaultSpillLimit
	}
	if len(content) <= limit {
		return content, false, nil
	}

	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", false, err
	}

	sum := sha256.Sum256([]byte(content))
	handle := hex.EncodeToString(sum[:8])
	path := filepath.Join(s.Dir, handle+".txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", false, err
	}

	head := content[:limit]
	lines := strings.Count(content, "\n") + 1

	summary := fmt.Sprintf(
		"%s\n\n... [moved to disk: %d bytes, %d lines in total]\nhandle: %s\nRead it with the read_result tool: {\"handle\": %q, \"offset\": %d}",
		head, len(content), lines, handle, handle, limit)

	return summary, true, nil
}

// Get returns a slice of a stored result.
func (s *SpillStore) Get(handle string, offset, length int) (string, int, error) {
	if length <= 0 {
		length = DefaultSliceBytes
	}
	if offset < 0 {
		offset = 0
	}

	path, err := s.pathFor(handle)
	if err != nil {
		return "", 0, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, fmt.Errorf("no result stored under handle %q", handle)
		}
		return "", 0, err
	}
	total := int(info.Size())

	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	if offset >= total {
		return "", total, fmt.Errorf("offset %d is past the end of %d bytes", offset, total)
	}

	buf := make([]byte, length)
	n, err := f.ReadAt(buf, int64(offset))
	if err != nil && n == 0 {
		return "", total, err
	}
	return string(buf[:n]), total, nil
}

// Handles lists what is stored, newest name first, for a person looking at the directory.
func (s *SpillStore) Handles() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".txt") {
			out = append(out, strings.TrimSuffix(e.Name(), ".txt"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// pathFor rejects a handle that is not a plain hex name.
//
// The handle arrives as a model argument, so it is untrusted input like any other.
func (s *SpillStore) pathFor(handle string) (string, error) {
	if handle == "" {
		return "", fmt.Errorf("handle is required")
	}
	for _, r := range handle {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return "", fmt.Errorf("invalid handle: %q", handle)
		}
	}
	return filepath.Join(s.Dir, handle+".txt"), nil
}

// ReadResultTool fetches a slice of a stored result.
type ReadResultTool struct {
	Store *SpillStore
}

func (t *ReadResultTool) Name() string { return "read_result" }

func (t *ReadResultTool) Description() string {
	return "Read part of a large tool result that was moved to disk. Returns up to 8000 bytes, and the total size."
}

func (t *ReadResultTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"handle": map[string]any{
				"type":        "string",
				"description": "The handle from the notice.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Byte offset to start at. Default 0.",
			},
			"length": map[string]any{
				"type":        "integer",
				"description": "How many bytes. Default 8000.",
			},
		},
		"required": []string{"handle"},
	}
}

func (t *ReadResultTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Handle string `json:"handle"`
		Offset int    `json:"offset"`
		Length int    `json:"length"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if t.Store == nil {
		return "", fmt.Errorf("no store configured")
	}

	slice, total, err := t.Store.Get(args.Handle, args.Offset, args.Length)
	if err != nil {
		return "", err
	}

	end := args.Offset + len(slice)
	note := fmt.Sprintf("[bytes %d-%d of %d]", args.Offset, end, total)
	if end < total {
		note += fmt.Sprintf("\n[next: {\"handle\": %q, \"offset\": %d}]", args.Handle, end)
	}
	return note + "\n" + slice, nil
}

// WithSpill wraps a registry so every result over the limit is moved to disk.
//
// It returns a new registry rather than changing the existing one, because a caller may
// want both: the raw registry for its own use, and a spilled one for the loop.
func WithSpill(r *Registry, store *SpillStore, limit int) *Registry {
	out := &Registry{byName: make(map[string]Tool, len(r.order))}
	for _, name := range r.order {
		out.byName[name] = &spillTool{inner: r.byName[name], store: store, limit: limit}
		out.order = append(out.order, name)
	}
	return out
}

type spillTool struct {
	inner Tool
	store *SpillStore
	limit int
}

func (t *spillTool) Name() string               { return t.inner.Name() }
func (t *spillTool) Description() string        { return t.inner.Description() }
func (t *spillTool) Parameters() map[string]any { return t.inner.Parameters() }

func (t *spillTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	out, err := t.inner.Execute(ctx, input)
	if err != nil {
		return "", err
	}
	spilled, _, err := t.store.Put(t.inner.Name(), out, t.limit)
	if err != nil {
		return "", err
	}
	return spilled, nil
}
