# 10 Problem

> **Your task:** expand `/name args` into the skill's body, substituting
> `$ARGUMENTS` and positional placeholders, and handle several skills in one prompt.

**You edit:** `agent/skill.go`, `agent/cmd/agent/main.go`
**Given:** the catalogue from chapter 09
**Time:** 30 to 50 minutes

---

## The contract

```go
// skill.go
type Expansion struct {
    Name string
    Args string
}

func (c *Catalog) Expand(prompt string) (string, []Expansion)
```

One line in `main`, before the prompt becomes history:

```go
userText, _ := catalog.Expand(*prompt)
```

Placeholders:

| Placeholder | Replaced with |
|---|---|
| `$ARGUMENTS` | everything after the skill name, as one string |
| `$0`, `$1`, ... | one whitespace-separated word, by position |
| missing position | the empty string |

## What the verifier does

It builds the binary with `skills/` present, runs the program with a prompt containing
slash commands, and inspects the **user message** of the recorded request. No test reads
your parser.

## The verifier will assert that

- `loads-body-on-invoke`: `/deploy staging eu-west` puts the body in the user message.
- `substitutes-arguments`: `$0`, `$1` and `$ARGUMENTS` are replaced, and no placeholder
  survives.
- `stacks-skills`: `/review /deploy production eu-west` expands both, review first, with
  deploy's own arguments applied.
- `no-invocation-no-body`: an ordinary prompt arrives byte-identical and no body is sent.
- `unknown-skill-preserved`: `/nosuch please help` keeps the original text, and the
  program still answers.
- `frontmatter-not-leaked`: the expanded body is present, and neither `---` nor
  `description:` appears in it.

## Notes

- **Replace `$ARGUMENTS` before `$0`.** Both start with `$`.
- **Arguments end at the next slash command**, so several skills share a prompt.
- **Preserve unknown commands verbatim.** A typo must not delete the person's words.
- **Expand before recording the turn.** The session should hold what was sent.
- **Do not rewrite the rest of the prompt.** Only the commands you recognise change.
- **Deliberate omission:** no scripts, no named arguments, no model-chosen invocation.

## Hints

<details><summary>Rung 1, the mechanism</summary>

Walk the prompt for slash commands, and replace each recognised one with its body.
Arguments run to the next command.
</details>

<details><summary>Rung 2, the APIs</summary>

`regexp` with `(^|\s)/([a-z0-9][a-z0-9-]*)` for commands, `strings.Fields` for arguments,
`regexp.ReplaceAllStringFunc` for `\$(\d+)`.
</details>

<details><summary>Rung 3, what the code does</summary>

`Expand` finds every command match, takes the text between one command and the next as
its arguments, and writes either the expanded body or the original text. `expandBody`
replaces `$ARGUMENTS` first, then each positional match against the whitespace-split
arguments, substituting the empty string when the index is out of range.

`kata hint 10 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 10` reports 6/6, and:

- `/deploy staging eu-west` sends the body with the arguments applied;
- two skills in one prompt both expand, in order;
- a mistyped command leaves the text alone.
