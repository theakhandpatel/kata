# 10 — Example

*Full, annotated implementation. Read it after `kata submit 10` passes.*

The compilable file is `reference/ch10/agent/skill.go` (the catalogue plus expansion),
with one changed line in `reference/ch10/agent/cmd/agent/main.go`.

---

## Step 1 — expansion is a left-to-right walk

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

- **`argsEnd` from the next match** is what makes stacking work. Without it the first
  skill swallows the whole prompt.
- **Unknown names are copied, not dropped.** `prompt[last:argsEnd]` includes the slash
  and its arguments.
- **`TrimRight` on the preceding text** removes the single space before the command.
- **The write after the loop** keeps text that follows the last command.

## Step 2 — placeholders, longest first

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

`$ARGUMENTS` goes first because it shares the `$` prefix. A regex with a digit group
avoids the trap a hand-rolled `strings.Replace(body, "$0", ...)` loop falls into:
matching `$0` inside something like `$0x`.

A missing argument becomes empty. Leaving `$3` literal would send the model a
placeholder it cannot resolve.

## Step 3 — one line of wiring

```go
userText, _ := catalog.Expand(*prompt)
history = append(history, agent.Message{Role: agent.RoleUser, Content: userText})
```

Expansion happens before the message enters history, so a resumed session replays the
expanded text. The session stores what was sent, not the shorthand.

## The diffs

**1. Replacing `$0` before `$ARGUMENTS`** — `substitutes-arguments`

```diff
- out = strings.ReplaceAll(out, "$0", words[0])
  out = strings.ReplaceAll(out, "$ARGUMENTS", args)
```

**2. Dropping an unknown command** — `unknown-skill-preserved`

```diff
- if !ok { continue }
+ if !ok { b.WriteString(prompt[last:argsEnd]); last = argsEnd; continue }
```

**3. Replacing in place for every invocation** — `stacks-skills`

```diff
- prompt = strings.Replace(prompt, "/"+name, body, 1)   // loses arguments and order
+ // walk matches and rebuild once
```

**4. Sending the header** — `frontmatter-not-leaked`

```diff
- b.WriteString(skill.Path)   // or the whole file
+ b.WriteString(expandBody(skill.Body, args))
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch10/agent/skill.go` | 290 | the walk, argument delimiting, substitution |
| `reference/ch10/agent/cmd/agent/main.go` | 180 | one changed line |

## Comparison checklist

- [ ] expansion walks matches left to right
- [ ] arguments end at the next slash command
- [ ] unknown commands pass through verbatim
- [ ] `$ARGUMENTS` replaced before positional forms
- [ ] missing positional argument becomes empty
- [ ] body only, no frontmatter
- [ ] expansion happens before the turn is recorded

Differences outside this list are style.
