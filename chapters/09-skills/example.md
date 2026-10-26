# 09 — Example

*Full, annotated implementation. Read it after `kata submit 09` passes.*

The compilable files are `reference/ch09/agent/skill.go` and
`reference/ch09/agent/cmd/agent/main.go`. The prompt builder and the loop are
unchanged; the catalogue arrives through the `Contributor` seam from chapter 05.

---

## The shape

```
skill.go    ParseSkill, LoadSkills, Catalog, Expand, expandBody
main.go     load, pass to BuildSystemPrompt, expand the prompt
```

## Step 1 — parse without a YAML dependency

```go
if !strings.HasPrefix(text, "---") {
	return Skill{}, ErrNoFrontmatter
}
rest := text[3:]
end := strings.Index(rest, "\n---")
if end < 0 {
	return Skill{}, ErrNoFrontmatter
}

for _, line := range strings.Split(rest[:end], "\n") {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		continue
	}
	meta[strings.ToLower(strings.TrimSpace(key))] = trimQuotes(strings.TrimSpace(value))
}
```

`strings.Cut` splits on the first colon, so a description containing a colon still
works: `description: Deploy: to a region` keeps the rest. That is the one place a
naive `strings.Split(line, ":")` fails, and descriptions contain colons constantly.

Quotes are trimmed rather than required, because both `name: deploy` and
`name: "deploy"` appear in the wild.

## Step 2 — required fields, and skipping is not failing

```go
name := meta["name"]
description := meta["description"]
if name == "" || description == "" {
	return Skill{}, fmt.Errorf("%s: name and description are required", path)
}
```

```go
skill, err := ParseSkill(path, data)
if err != nil {
	continue
}
```

A skill with no description cannot be advertised, so there is nothing useful to do
with it. The choice is between failing the run and skipping the file. Skipping wins:
the agent still works, and the person can fix the file when they notice.

## Step 3 — advertise the minimum

```go
b.WriteString("# Skills\n\n")
b.WriteString("Invoke a skill by writing /name followed by its arguments.\n")
for _, s := range c.skills {
	fmt.Fprintf(&b, "\n- %s: %s\n", s.Name, s.Description)
}
```

One line of instructions, then one line per skill. The body is nowhere in this
function, which is what keeps a large catalogue cheap.

The instruction line matters: without it the model sees a list of names and no
statement about how they are used.

## Step 4 — expansion is a left-to-right walk

```go
for i, m := range matches {
	name := prompt[m[4]:m[5]]

	argsEnd := len(prompt)
	if i+1 < len(matches) {
		argsEnd = matches[i+1][0]
	}
	args := strings.TrimSpace(prompt[m[1]:argsEnd])

	skill, ok := c.byName[name]
	if !ok {
		b.WriteString(prompt[last:argsEnd])
		last = argsEnd
		continue
	}

	b.WriteString(strings.TrimRight(prompt[last:m[0]], " \t"))
	b.WriteString(expandBody(skill.Body, args))
	used = append(used, Expansion{Name: name, Args: args})
	last = argsEnd
}
b.WriteString(prompt[last:])
```

Four details, each with a failure behind it:

- **`argsEnd` from the next match** is what makes stacking work. Without it the
  first skill swallows the whole prompt.
- **Unknown names are copied, not dropped.** `prompt[last:argsEnd]` includes the
  slash and its arguments.
- **`TrimRight` on the preceding text** removes the single space before the command,
  so the body does not start with an indent.
- **The trailing write after the loop** keeps text that follows the last command.

## Step 5 — placeholders, longest first

```go
out := strings.ReplaceAll(body, "$ARGUMENTS", args)

words := strings.Fields(args)
out = positional.ReplaceAllStringFunc(out, func(m string) string {
	idx, err := strconv.Atoi(m[1:])
	if err != nil || idx >= len(words) {
		return ""
	}
	return words[idx]
})
```

`$ARGUMENTS` is replaced first because it shares the `$` prefix. A positional
regex that ran first would match nothing in `$ARGUMENTS` (the next character is a
letter), but a hand-rolled `strings.Replace(body, "$0", ...)` loop over indices
would happily corrupt `$0` inside `$0-something`. A regex with a digit group is the
version that does not need care.

A missing argument becomes empty. Leaving `$3` literal would send the model a
placeholder it cannot resolve.

## Step 6 — wiring through the existing seam

```go
skills, err := agent.LoadSkills(root)
catalog := agent.NewCatalog(skills)

system, err := agent.BuildSystemPrompt(ctx, agent.BasePrompt,
	&agent.AgentsFile{Root: root},
	catalog,
)
```

```go
userText, _ := catalog.Expand(*prompt)
history = append(history, agent.Message{Role: agent.RoleUser, Content: userText})
```

Two lines of wiring, no change to `BuildSystemPrompt` and none to `RunLoop`. The
`Contribute` method is the whole interface.

Expansion happens before the message enters history, so a resumed session replays
the expanded text. The session stores what was sent, not the shorthand.

## The diffs

**1. Sending every body** — `body-not-advertised`

```diff
- fmt.Fprintf(&b, "\n- %s: %s\n%s\n", s.Name, s.Description, s.Body)
+ fmt.Fprintf(&b, "\n- %s: %s\n", s.Name, s.Description)
```

**2. Replacing `$0` before `$ARGUMENTS`** — `substitutes-arguments`

```diff
- out = strings.ReplaceAll(out, "$0", words[0])
  out = strings.ReplaceAll(out, "$ARGUMENTS", args)
```

**3. Dropping an unknown command** — `unknown-skill-preserved`

```diff
- if !ok { continue }
+ if !ok { b.WriteString(prompt[last:argsEnd]); last = argsEnd; continue }
```

**4. Editing the prompt in place for every invocation** — `stacks-skills`

```diff
- prompt = strings.Replace(prompt, "/"+name, body, 1)   // loses arguments and order
+ // walk matches and rebuild once
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch09/agent/skill.go` | 235 | parsing, catalogue, expansion |
| `reference/ch09/agent/cmd/agent/main.go` | 180 | two lines of wiring |

## Comparison checklist

- [ ] frontmatter parsed without a YAML dependency, colons in values survive
- [ ] name and description required; malformed files skipped
- [ ] `skills/` missing is not an error
- [ ] catalogue advertises name and description only
- [ ] expansion walks matches left to right
- [ ] arguments end at the next slash command
- [ ] unknown commands pass through verbatim
- [ ] `$ARGUMENTS` replaced before positional forms
- [ ] missing positional argument becomes empty
- [ ] expansion happens before the turn is recorded

Differences outside this list are style.
