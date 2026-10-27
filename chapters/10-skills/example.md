# 09 Example

*Full, annotated implementation. Read it after `kata submit 10` passes.*

The compilable file is `reference/ch10/agent/skill.go`, plus three lines of wiring in
`reference/ch10/agent/cmd/agent/main.go`.

---

## Step 1: parse without a YAML dependency

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
works: `description: Deploy: to a region` keeps the rest. A `strings.Split(line, ":")`
does not.

Quotes are trimmed rather than required, because both `name: deploy` and
`name: "deploy"` appear in the wild.

## Step 2: required fields, and skipping is not failing

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
with it. Skipping keeps the agent usable while the person fixes the file.

## Step 3: a missing directory is empty, not broken

```go
entries, err := os.ReadDir(base)
if err != nil {
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}
```

Most projects have no `skills/`. Treating that as an error makes the agent refuse to
work in an empty folder, which is the state every new project starts in.

## Step 4: advertise the minimum

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

## Step 5: three lines of wiring

```go
skills, err := agent.LoadSkills(root)
catalog := agent.NewCatalog(skills)

system, err := agent.BuildSystemPrompt(ctx, agent.BasePrompt,
	&agent.AgentsFile{Root: root},
	catalog,
)
```

`Contribute` is the whole interface. `BuildSystemPrompt` and `RunLoop` are unchanged.

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch10/agent/skill.go` | 175 | parsing, discovery, the catalogue |
| `reference/ch10/agent/cmd/agent/main.go` | 175 | three lines of wiring |

## Comparison checklist

- [ ] frontmatter parsed without a YAML dependency, colons in values survive
- [ ] name and description required; malformed files skipped
- [ ] `skills/` missing is not an error
- [ ] catalogue advertises name and description only
- [ ] body parsed and unused
- [ ] catalogue reaches the prompt through `Contributor`

Differences outside this list are style.
