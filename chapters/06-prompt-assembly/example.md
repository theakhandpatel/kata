# 05 Example

*Full, annotated implementation. Read it after `kata submit 05` passes.*

The compilable files are `reference/ch05/agent/prompt.go` and
`reference/ch05/agent/cmd/agent/main.go`. The loop and the provider are unchanged
from the previous chapter, which is the point of that interface.

---

## The shape

```
prompt.go   BasePrompt, Contributor, BuildSystemPrompt, AgentsFile
main.go     build the system message, prepend it to history
loop.go     untouched
provider    untouched
```

Two files changed. That is the design working.

## Step 1: the builder walks a list

```go
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
```

Two details carry weight:

- `if c == nil { continue }` lets a caller pass an optional contributor without a
  wrapper type.
- `if section == "" { continue }` is where "nothing to add" is handled once, so no
  contributor has to think about spacing.

A contributor returns a *section*, not a line. It decides its own heading. That
keeps the builder ignorant of what kinds of things contribute, which is what lets
the Skills: advertise chapter reuses it unchanged.

## Step 2: discovery walks up, rendering runs down

```go
func (a *AgentsFile) discover() ([]string, error) {
	dir := abs            // the workspace
	var found []string
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
	for i, j := 0, len(found)-1; i < j; i, j = i+1, j-1 {
		found[i], found[j] = found[j], found[i]
	}
	return found, nil
}
```

The reverse is the whole chapter. Collected order is nearest-first, because that
is the direction you can walk. Rendered order must be nearest-last, because that is
the direction a model reads.

`parent == dir` is how the walk terminates at `/` without a special case. The depth
bound is a second guard: a stray `/AGENTS.md` on a container image should not
silently become project instructions.

## Step 3: empty is absent, and the heading is written once

```go
for _, path := range files {
	body, err := os.ReadFile(path)
	if err != nil {
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
```

Three failure modes collapsed into one loop:

| Without the guard | What the model sees |
|---|---|
| no `included` counter | `# Project instructions` with nothing under it |
| no empty check | `## /repo/AGENTS.md` followed by a blank line |
| a read error treated as fatal | the agent refuses to run because one file was unreadable |

A file that disappears between discovery and reading is skipped on purpose. The
instruction is nice to have; the run is not.

## Step 4: the loop does not change

```go
system, err := agent.BuildSystemPrompt(ctx, agent.BasePrompt,
	&agent.AgentsFile{Root: root},
)
history := []agent.Message{
	{Role: agent.RoleSystem, Content: system},
	{Role: agent.RoleUser, Content: *prompt},
}
res, err := agent.RunLoop(ctx, provider, tools, history, opts)
```

The system message is just the first element of history. `RunLoop` already sends
whatever it is given, so nothing in the loop needed to learn about prompts.

That is worth pausing on. The instinct is to add a `SystemPrompt` field to
`LoopOptions` and let the loop build it. That works and it couples the loop to
prompt policy forever. Prepending a message keeps the loop's contract simple: it
sends a conversation.

## The diffs

**1. Pasting instead of assembling** (`system-message-first`)

```diff
- history := []Message{{Role: RoleUser, Content: prompt}}
+ history := []Message{{Role: RoleSystem, Content: system}, {Role: RoleUser, Content: prompt}}
```

**2. Rendering in discovery order** (`agents-md-order`)

```diff
- return found                     // nearest first
+ reverse(found); return found     // outermost first
```

**3. A heading per file** (`empty-agents-md`)

```diff
- b.WriteString("# Project instructions\n")    // outside the loop
+ if included == 0 { b.WriteString("# Project instructions\n") }
```

**4. Unstable prefix** (`system-prompt-stable`)

```diff
- fmt.Fprintf(&b, "# Context\nworking in %s at %s\n", dir, time.Now())
+ // nothing that varies between runs
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch05/agent/prompt.go` | 165 | the builder, discovery, empty handling |
| `reference/ch05/agent/cmd/agent/main.go` | 115 | prepending instead of threading a field |

## Comparison checklist

- [ ] base prompt is a constant with no varying content
- [ ] contributors are read in order, nil and empty skipped
- [ ] discovery walks up and is bounded
- [ ] rendered order is outermost first
- [ ] heading written once, only if something was included
- [ ] unreadable and empty files skipped rather than fatal
- [ ] system message prepended, loop unchanged
- [ ] user message byte-identical to what was typed

Differences outside this list are style.
