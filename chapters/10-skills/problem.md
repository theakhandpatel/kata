# 09 Problem

> **Your task:** discover `skills/*/SKILL.md`, parse the small header, and advertise
> each skill's name and description in the system message.

**You edit:** `agent/skill.go`, `agent/cmd/agent/main.go`
**Given:** `agent/prompt.go` (the `Contributor` interface), `agent/tools.go`
**Time:** 30 to 40 minutes

---

## The contract

```go
// skill.go
type Skill struct {
    Name        string
    Description string
    Body        string   // parsed, and not advertised in this chapter
    Path        string
}

func ParseSkill(path string, data []byte) (Skill, error)
func LoadSkills(root string) ([]Skill, error)

type Catalog struct{ /* ... */ }
func NewCatalog(skills []Skill) *Catalog
func (c *Catalog) Contribute(ctx context.Context) (string, error)   // Contributor
```

File layout:

```
skills/deploy/SKILL.md
---
name: deploy
description: Deploy the app to a region.
---
Check the tests pass, then deploy to $0 in $1.
```

## What the verifier does

It builds the binary, writes a temporary workspace containing `skills/`, and runs the
program. Then it reads the **system message** of the recorded request. No test reads
your parser directly.

## The verifier will assert that

- `advertises-skills`: the system message names `deploy` and `review` with their
  descriptions.
- `body-not-advertised`: with the catalogue present, no uninvoked body appears
  anywhere in the request.
- `malformed-skill-skipped`: a `SKILL.md` with no frontmatter is left out, and the
  run still succeeds with the valid skill advertised.
- `missing-skills-dir`: with no `skills/` directory the base prompt is sent, there is
  no skills section, and the exit code is 0.

## Notes

- **Parse the header, keep the body.** In this chapter the body is parsed and unused.
  Chapter 10 consumes it.
- **Skip, do not fail.** A file without `name` or `description` cannot be advertised.
- **A missing directory is empty, not broken.**
- **The catalogue is a `Contributor`.** Add it to `BuildSystemPrompt`; the loop and the
  provider stay untouched.
- **Deliberate omission:** no invocation, no scripts, no model-chosen skills.

## Hints

<details><summary>Rung 1, the mechanism</summary>

A skill is a small header plus a body. The header is advertised everywhere; the body
waits.
</details>

<details><summary>Rung 2, the APIs</summary>

`os.ReadDir`, `strings.Cut` for `key: value`, `strings.Index` for the closing fence,
`os.ErrNotExist` for the missing directory.
</details>

<details><summary>Rung 3, what the code does</summary>

`ParseSkill` requires a leading `---`, finds the next `\n---`, reads `key: value` lines
into a map, and takes the rest as the body; it errors when `name` or `description` is
missing. `LoadSkills` reads each `skills/*/SKILL.md`, skips parse errors, returns nil
when the directory is absent, and sorts by name. `Contribute` renders `# Skills`, one
line of instructions, then `- name: description` per skill. `main` passes the catalogue
to `BuildSystemPrompt`.

`kata hint 09 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 09` reports 4/4, and:

- a skill file on disk appears in the system message by name and description;
- a broken skill file does not stop the agent;
- you can explain why the body is not in the system message.
