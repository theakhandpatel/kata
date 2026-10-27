# 17 Example

*The finished implementation, taken apart. Open this after `kata submit 17` passes.*

The file it describes is `reference/ch17/agent/scoped.go`.

---

## Resolving collisions

```go
for _, s := range skills {
	candidate := ScopedSkill{Skill: s, Scope: scope.Name, Priority: scope.Priority}

	existing, taken := cat.byName[s.Name]
	if !taken {
		cat.byName[s.Name] = candidate
		continue
	}

	winner, loser := pickWinner(existing, candidate)
	cat.byName[s.Name] = winner
	cat.shadow = append(cat.shadow, Shadowed{
		Name:   s.Name,
		Winner: winner.Scope,
		Loser:  loser.Scope,
	})
}
```

The map is keyed by skill name, so the second definition of `deploy` replaces the first
rather than sitting beside it. Everything else accumulates.

Recording the shadow is not bookkeeping for its own sake. A person who edits a skill and
sees no change needs one line telling them another copy won.

## The tiebreak

```go
func pickWinner(a, b ScopedSkill) (winner, loser ScopedSkill) {
	if a.Priority != b.Priority {
		if a.Priority > b.Priority {
			return a, b
		}
		return b, a
	}
	if a.Scope <= b.Scope {
		return a, b
	}
	return b, a
}
```

Without the second half, two scopes at the same priority resolve differently depending on
the order the caller listed them. That is a bug that only appears on another person's
machine, which is the worst kind.

Comparing scope names is arbitrary, but it is arbitrary in the same way every time.

## Scope roots

```go
func DefaultScopes(projectRoot string) []Scope {
	var scopes []Scope

	if config := os.Getenv("XDG_CONFIG_HOME"); config != "" {
		scopes = append(scopes, Scope{
			Name:     "user",
			Root:     filepath.Join(config, "agent"),
			Priority: 10,
		})
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		...
	}

	scopes = append(scopes, Scope{
		Name:     "project",
		Root:     projectRoot,
		Priority: 20,
	})
	return scopes
}
```

`Root` is the directory that contains `skills`, not `skills` itself, because the loader
appends the folder name. Getting that wrong produces an empty catalogue with no error,
which is the failure this chapter's own first draft had.

`XDG_CONFIG_HOME` is checked before `HOME`. It is the correct location on Linux, and it
lets the checks point the user scope at a temporary directory instead of your real one.

## Expansion uses the winners

```go
func (c *ScopedCatalog) Expand(prompt string) (string, []Expansion) {
	skills := make([]Skill, 0, len(c.chosen))
	for _, s := range c.chosen {
		skills = append(skills, s.Skill)
	}
	return NewCatalog(skills).Expand(prompt)
}
```

A plain catalogue built from the resolved set, so the expansion code from the previous
chapter is reused unchanged and cannot accidentally read the losing copy.

This is the step that is easy to forget. Catalogue precedence and invocation precedence
are different code paths, and a program where the first is right and the second is wrong
passes a casual test and fails the one that matters.

## The catalogue

```go
b.WriteString("# Skills\n\n")
b.WriteString("Invoke a skill by writing /name followed by its arguments.\n")
for _, s := range c.chosen {
	fmt.Fprintf(&b, "\n- %s (%s): %s\n", s.Name, s.Scope, s.Description)
}
```

The scope in brackets costs a few characters per skill and answers the question a reader
actually has: which copy is this.

## The fixes that matter

**1. Listing both copies** (`project-outranks-user`)

```diff
- out = append(out, candidate)          // a slice, duplicates and all
+ cat.byName[s.Name] = winner           // a map, one per name
```

**2. Comparing priorities but not tiebreaking** (`both-scopes-contribute`)

```diff
- if a.Priority >= b.Priority { return a, b }
+ if a.Priority != b.Priority { ... }
+ if a.Scope <= b.Scope { return a, b }
```

**3. Expanding from the directories** (`losing-copy-is-not-expanded`)

```diff
- return NewCatalog(allSkills).Expand(prompt)
+ return NewCatalog(winners).Expand(prompt)
```

**4. Passing the skills directory as the root** (`missing-scope-is-skipped`)

```diff
- Root: filepath.Join(projectRoot, "skills")
+ Root: projectRoot        // the loader appends "skills"
```

## What to compare against your own

- [ ] one skill per name survives
- [ ] priority decides, and the scope name breaks ties
- [ ] the shadowed copy is recorded
- [ ] expansion uses the winning body
- [ ] a missing scope is skipped, not fatal
- [ ] a malformed file costs only itself
- [ ] the catalogue names each skill's scope

Everything outside that list is style.
