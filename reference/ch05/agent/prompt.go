package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BasePrompt is the instruction that is always present.
//
// It is a constant on purpose. The system message is the first thing every
// request sends, and a prefix that changes between runs costs providers their
// prompt cache. Nothing here may depend on the clock, the directory, or the
// process.
const BasePrompt = `You are a coding agent working in a terminal.
You can read and write files and run shell commands.
Prefer small, verifiable steps. Say what you did.`

// Contributor adds one section to the system prompt.
//
// The interface exists so that later chapters can add sections without editing
// the loop or the prompt builder: project instructions now, skills in chapter 09.
type Contributor interface {
	// Contribute returns a section, or an empty string to add nothing.
	Contribute(ctx context.Context) (string, error)
}

// BuildSystemPrompt assembles the system message from the base instruction and
// each contributor, in order.
//
// Empty sections are dropped rather than left as blank headings, because an empty
// heading reads to a model as "there were instructions here and they were
// removed", which is noise.
func BuildSystemPrompt(ctx context.Context, base string, contributors ...Contributor) (string, error) {
	var b strings.Builder
	b.WriteString(strings.TrimRight(base, "\n"))
	b.WriteString("\n")

	for _, c := range contributors {
		if c == nil {
			continue
		}
		section, err := c.Contribute(ctx)
		if err != nil {
			return "", err
		}
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(section)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// DefaultContextDepth bounds how far up the tree the agent looks for AGENTS.md.
const DefaultContextDepth = 8

// AgentsFile contributes the contents of every AGENTS.md from the workspace
// upward.
//
// Discovery runs from the outermost file to the nearest one, so the most specific
// instructions come last. A model reading top to bottom reaches the rules for the
// directory it is actually working in at the end, which is where a final
// instruction carries the most weight.
type AgentsFile struct {
	// Root is where discovery starts. Empty means the process working directory.
	Root string
	// MaxDepth bounds the walk upward. Zero means DefaultContextDepth.
	MaxDepth int
}

func (a *AgentsFile) Contribute(ctx context.Context) (string, error) {
	files, err := a.discover()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	included := 0
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			// A file that vanished between discovery and reading is not worth
			// failing a run over.
			continue
		}
		text := strings.TrimRight(string(body), "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		if included == 0 {
			b.WriteString("# Project instructions\n")
		}
		included++
		fmt.Fprintf(&b, "\n## %s\n%s\n", path, text)
	}
	if included == 0 {
		return "", nil
	}
	return b.String(), nil
}

// discover returns the AGENTS.md files from the outermost directory down to
// Root.
func (a *AgentsFile) discover() ([]string, error) {
	root := a.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	depth := a.MaxDepth
	if depth <= 0 {
		depth = DefaultContextDepth
	}

	var found []string
	dir := abs
	for i := 0; i < depth; i++ {
		candidate := filepath.Join(dir, "AGENTS.md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			found = append(found, candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Reverse: outermost first, nearest last.
	for i, j := 0, len(found)-1; i < j; i, j = i+1, j-1 {
		found[i], found[j] = found[j], found[i]
	}
	return found, nil
}
