package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DefaultMaxTools is how many tool schemas a request may advertise.
const DefaultMaxTools = 6

// Mounted is a registry that advertises only some of its tools.
//
// Schemas travel with every request, so a project with twenty tools pays for twenty
// definitions on every turn whether or not the model calls any. A ceiling on how many
// are advertised keeps that cost bounded; the rest stay callable through a tool that
// loads them on demand.
type Mounted struct {
	all    *Registry
	max    int
	mu     sync.Mutex
	mounts map[string]bool // extra names mounted on request
	always []string        // names that are never hidden
}

// NewMounted wraps a registry with a ceiling.
func NewMounted(all *Registry, max int) *Mounted {
	if max <= 0 {
		max = DefaultMaxTools
	}
	return &Mounted{all: all, max: max, mounts: map[string]bool{}}
}

// Mounted lists the tools a request will advertise, in registration order.
//
// The first `max` tools are always mounted, so a registry that fits advertises
// everything and behaviour does not change. Everything after the ceiling is reachable
// only after being asked for.
func (m *Mounted) Mounted() []ToolDef {
	names := m.mountedNames()
	defs := m.all.Defs()
	out := make([]ToolDef, 0, len(names))
	for _, d := range defs {
		for _, n := range names {
			if d.Name == n {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// Mount adds a tool to the advertised set and reports whether it was there to add.
func (m *Mounted) Mount(name string) bool {
	if _, ok := m.all.Lookup(name); !ok {
		return false
	}
	m.mu.Lock()
	m.mounts[name] = true
	m.mu.Unlock()
	return true
}

// Names lists every tool the registry knows, mounted or not.
func (m *Mounted) Names() []string { return m.all.Names() }

func (m *Mounted) mountedNames() []string {
	all := m.all.Names()

	m.mu.Lock()
	extra := make([]string, 0, len(m.mounts))
	for name := range m.mounts {
		extra = append(extra, name)
	}
	m.mu.Unlock()
	sort.Strings(extra)

	seen := map[string]bool{}
	out := make([]string, 0, len(all))

	// Names that must never be hidden come first, whatever the ceiling.
	m.mu.Lock()
	always := append([]string{}, m.always...)
	m.mu.Unlock()
	for _, name := range always {
		if seen[name] {
			continue
		}
		out = append(out, name)
		seen[name] = true
	}

	// Then the rest, in registration order, up to the ceiling.
	for _, name := range all {
		if seen[name] {
			continue
		}
		if len(out) >= m.max {
			break
		}
		out = append(out, name)
		seen[name] = true
	}

	// Then anything explicitly mounted, even past the ceiling, because the model asked.
	for _, name := range extra {
		if seen[name] {
			continue
		}
		out = append(out, name)
		seen[name] = true
	}
	return out
}

// ToolLoader is the tool that mounts others.
//
// It is itself always mounted, so the model has a way back to anything the ceiling hid.
type ToolLoader struct {
	Mounted *Mounted
}

func (t *ToolLoader) Name() string { return "load_tool" }

func (t *ToolLoader) Description() string {
	return "Make another tool callable. Ask for one by name. The tools you have are listed in the system message; the rest are named here."
}

func (t *ToolLoader) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The tool to make callable.",
			},
		},
		"required": []string{"name"},
	}
}

func (t *ToolLoader) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Name == "" {
		return "", fmt.Errorf("name is required")
	}

	known := false
	for _, n := range t.Mounted.Names() {
		if n == args.Name {
			known = true
			break
		}
	}
	if !known {
		return "", fmt.Errorf("no such tool: %s", args.Name)
	}
	if !t.Mounted.Mount(args.Name) {
		return "", fmt.Errorf("could not load %s", args.Name)
	}
	return fmt.Sprintf("Loaded %s. Its schema is available on the next turn; call it with arguments matching that schema.", args.Name), nil
}

// ToolList renders the names that are not currently advertised.
//
// Without it the model cannot know what to ask for, and a tool that can only be loaded
// by guessing its name is a tool that is never loaded.
func (m *Mounted) ToolList() string {
	mounted := map[string]bool{}
	for _, n := range m.mountedNames() {
		mounted[n] = true
	}
	var hidden []string
	for _, n := range m.all.Names() {
		if !mounted[n] {
			hidden = append(hidden, n)
		}
	}
	if len(hidden) == 0 {
		return ""
	}
	return "Other tools you can load with load_tool: " + strings.Join(hidden, ", ")
}

// View exposes a Mounted registry to the loop as an ordinary Registry.
//
// The loop asks for a tool list and a way to run a call. It does not need to know that
// the list is capped, or that results are spilled on the way out.
type View struct {
	Mounted *Mounted
	Spill   *SpillStore
	Limit   int
}

// Defs returns only the advertised tools.
func (v *View) Defs() []ToolDef { return v.Mounted.Mounted() }

// Registry returns a plain registry built from the advertised tools, so the loop's
// existing dispatch works unchanged.
func (v *View) Registry() *Registry {
	advertised := v.Mounted.Mounted()
	tools := make([]Tool, 0, len(advertised))
	for _, d := range advertised {
		if t, ok := v.Mounted.all.Lookup(d.Name); ok {
			tools = append(tools, t)
		}
	}
	r := NewRegistry(tools...)
	if v.Spill != nil {
		return WithSpill(r, v.Spill, v.Limit)
	}
	return r
}

// WithBudgetedRegistry keeps the spill wrapper and the schema ceiling in one place.
func WithBudgetedRegistry(m *Mounted, store *SpillStore, limit int) *Registry {
	v := &View{Mounted: m, Spill: store, Limit: limit}
	return v.Registry()
}

// Always keeps a tool mounted even when it sits past the ceiling.
//
// The loader itself needs this: a model that cannot see the tool for loading tools has
// no way back to anything the ceiling hid.
func (m *Mounted) Always(name string) {
	m.mu.Lock()
	m.always = append(m.always, name)
	m.mu.Unlock()
}
