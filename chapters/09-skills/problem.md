# 09 — Problem

> **Your task:** discover skills in the workspace, advertise their names and
> descriptions, and expand `/name args` into the skill's body when invoked.

**You edit:** `agent/skill.go`, `agent/cmd/agent/main.go`
**Given:** `agent/prompt.go` (the `Contributor` seam), `agent/loop.go`
**Time:** 75 to 100 minutes

---

## The contract

```go
// skill.go
type Skill struct {
    Name        string
    Description string
    Body        string
    Path        string
}

func LoadSkills(root string) ([]Skill, error)
func ParseSkill(path string, data []byte) (Skill, error)

type Catalog struct{ /* ... */ }
func NewCatalog(skills []Skill) *Catalog
func (c *Catalog) Contribute(ctx context.Context) (string, error)  // Contributor
func (c *Catalog) Expand(prompt string) (string, []Expansion)
```

File layout, one directory per skill:

```
skills/deploy/SKILL.md
---
name: deploy
description: Deploy the app to a region.
---
Check the tests pass, then deploy to $0 in $1.
Full request: $ARGUMENTS
```

## What the verifier does

It builds the binary, writes a temporary workspace containing `skills/`, runs the
program with a prompt, and inspects the **system message** and the **user message**
of the recorded request. No skill file is ever read by the checks directly.

## The verifier will assert that

- `advertises-skills` — the system message names `deploy` and `review` with their
  descriptions.
- `body-not-advertised` — with the catalogue present, no uninvoked body appears
  anywhere in the request.
- `loads-body-on-invoke` — `/deploy staging eu-west` puts the body in the user
  message.
- `substitutes-arguments` — `$0`, `$1` and `$ARGUMENTS` are replaced, and no
  placeholder survives.
- `stacks-skills` — `/review /deploy production eu-west` expands both, review
  first, with deploy's own arguments applied.
- `no-invocation-no-body` — a plain prompt arrives byte-identical and no body is
  sent.
- `unknown-skill-preserved` — `/nosuch please help` keeps the original text and the
  program still answers.
- `frontmatter-not-leaked` — the expanded body is present, and neither `---` nor
  `description:` appears in it.

## Notes

- **Two audiences.** Description goes in the system message; body goes in the user
  message, only on invocation.
- **Replace `$ARGUMENTS` before `$0`.** Both start with `$`.
- **Arguments end at the next slash command**, so several skills share a prompt.
- **Preserve unknown commands verbatim.** A typo must not delete the person's words.
- **Expand before recording the turn.** The session should hold what was sent.
- **Skip malformed skills.** One bad file must not stop the agent from starting.
- **Deliberate omission:** no scripts, no named arguments, no model-chosen
  invocation, no skill events.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

A skill is a file with a small header and a body. The header is advertised
everywhere; the body is substituted in when the person types its name.
</details>

<details><summary>Rung 2 — the APIs</summary>

`os.ReadDir` on `skills/`, `strings.Cut` for `key: value`, `strings.Index` for the
closing fence, `regexp` for slash commands and positional placeholders,
`strings.Fields` for arguments.
</details>

<details><summary>Rung 3 — what the code does</summary>

`ParseSkill` requires a leading `---`, finds the next `\n---`, reads `key: value`
lines into a map, and takes the rest as the body. `LoadSkills` reads each
`skills/*/SKILL.md`, skipping any parse error and sorting by name. `Contribute`
renders `# Skills`, one line of instructions, then `- name: description` per skill.
`Expand` walks slash-command matches, takes arguments up to the next match, and
writes either the expanded body or the original text. `expandBody` replaces
`$ARGUMENTS` then `\$(\d+)` against the whitespace-split arguments. `main` builds
the catalogue, passes it to `BuildSystemPrompt`, and expands the prompt before it
becomes history.

`kata hint 09 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 09` reports 8/8, and:

- `/deploy staging eu-west` sends the skill body with the arguments applied;
- a project with no `skills/` directory still runs;
- you can explain why the body is not in the system message.
