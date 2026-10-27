# 16 Example

*The finished implementation, taken apart. Open this after `kata submit 16` passes.*

The file it describes is `reference/ch16/agent/mount.go`.

---

## The advertised set

```go
func (m *Mounted) mountedNames() []string {
	all := m.all.Names()

	m.mu.Lock()
	extra := make([]string, 0, len(m.mounts))
	for name := range m.mounts {
		extra = append(extra, name)
	}
	always := append([]string{}, m.always...)
	m.mu.Unlock()
	sort.Strings(extra)

	seen := map[string]bool{}
	out := make([]string, 0, len(all))

	for _, name := range always {
		if seen[name] { continue }
		out = append(out, name)
		seen[name] = true
	}

	for _, name := range all {
		if seen[name] { continue }
		if len(out) >= m.max { break }
		out = append(out, name)
		seen[name] = true
	}

	for _, name := range extra {
		if seen[name] { continue }
		out = append(out, name)
		seen[name] = true
	}
	return out
}
```

Three loops, in priority order, and the order is the design.

**Always-on first.** `load_tool` has to be advertised even when the ceiling is one,
because it is the only way back to anything hidden. Putting it first means no later rule
can exclude it.

**Then registration order up to the ceiling.** Registration order is a policy: whoever
wrote `main` decided which tools matter most, and honouring that is better than sorting
alphabetically.

**Then explicitly mounted, past the ceiling.** A tool the model asked for is advertised
even if that takes the request over the limit. The alternative is a mount that silently
fails when the ceiling is already full, which is worse than one extra schema.

`sort.Strings(extra)` before iterating: map iteration order is random, and a request
that reorders its tool list between turns loses the provider's prefix cache.

## Why a set of names, not a slice of tools

```go
mounts map[string]bool
```

The registry is the source of truth for what a tool is. The mounted set only records
*which* of those to advertise. Keeping a second copy of the tool values would let them
drift, and a stale copy is a schema that no longer matches its implementation.

## Mounting

```go
func (m *Mounted) Mount(name string) bool {
	if _, ok := m.all.Lookup(name); !ok {
		return false
	}
	m.mu.Lock()
	m.mounts[name] = true
	m.mu.Unlock()
	return true
}
```

The lookup comes first. Without it, `load_tool{name: "reed"}` records a name that no
tool answers to, and every later request advertises a schema that cannot be called.

## The list of hidden tools

```go
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
```

Empty when nothing is hidden, so a small project's system message does not carry a line
saying there is nothing to see. That also keeps the prefix stable as the registry grows
and shrinks around the ceiling.

## The loader

```go
func (t *ToolLoader) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct{ Name string `json:"name"` }
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	...
	return fmt.Sprintf("Loaded %s. Its schema is available on the next turn; call it with arguments matching that schema.", args.Name), nil
}
```

The result says what happens next. A model that is told only "loaded" sometimes calls the
tool in the same turn, before its schema is in the request, and gets a confusing unknown
tool error.

## The view

```go
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
```

A plain registry built from the advertised tools, so the loop's dispatch works without
knowing anything changed. The spill wrapper from the previous chapter still applies, and
the two decorators compose in the order that makes sense: filter first, then wrap what
survives.

The cost of this shape is one allocation per turn. It is worth it, because the loop,
the provider and the registry all stay unchanged.

## The fixes that matter

**1. Truncating the registry instead of filtering** (`no-ceiling-advertises-everything`)

```diff
- return defs[:m.max]
+ // copy up to the ceiling, so a small registry is returned whole
```

**2. Letting the ceiling hide the loader** (`load-tool-is-always-advertised`)

```diff
- if len(out) >= m.max { break }
+ // always-on names first, then the ceiling
```

**3. Recording a mount without checking** (`unknown-tool-load-is-a-result`)

```diff
- m.mounts[name] = true; return true
+ if _, ok := m.all.Lookup(name); !ok { return false }
```

**4. Mounting for one turn only** (`loading-a-tool-makes-it-callable`)

```diff
- m.pending = append(m.pending, name)   // cleared after the turn
+ m.mounts[name] = true                 // permanent
```

## What to compare against your own

- [ ] the ceiling applies to the advertised set, not to the registry
- [ ] the loader is advertised at every ceiling
- [ ] hidden tools are named once, in the system message
- [ ] a mount is checked against the registry before it is recorded
- [ ] a mount survives the next turn
- [ ] the order is the same on every turn
- [ ] the estimate counts what was advertised

Everything outside that list is style.
