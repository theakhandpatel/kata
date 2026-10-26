# 05 — Problem

> **Your task:** assemble a system message from a stable base instruction and the
> project's `AGENTS.md` files, and send it before the user's message.

**You edit:** `agent/prompt.go`, `agent/cmd/agent/main.go`
**Given:** `agent/loop.go`, `agent/provider_openai.go`, `agent/tools.go`
**Time:** 45 to 60 minutes

---

## The contract

```go
// prompt.go

const BasePrompt = `...`   // a constant, and it must contain "coding agent"

type Contributor interface {
    Contribute(ctx context.Context) (string, error)
}

func BuildSystemPrompt(ctx context.Context, base string, contributors ...Contributor) (string, error)

type AgentsFile struct {
    Root     string // discovery starts here; empty means the working directory
    MaxDepth int    // 0 means 8
}
```

The rendering format the checks look for:

```
You are a coding agent working in a terminal.
...

# Project instructions

## /abs/path/AGENTS.md
<file body>

## /abs/path/pkg/AGENTS.md
<file body>
```

Outermost file first, nearest last. No section at all when nothing contributed.

## What the verifier does

It builds the binary, creates a temporary workspace (sometimes with nested
`AGENTS.md` files), runs the program there, and inspects the **first message** of
the recorded request. It also compares two runs for byte equality.

## The verifier will assert that

- `system-message-first` — `messages[0].role` is `system`, it contains the base
  instruction, and `messages[1]` is the user's prompt.
- `agents-md-included` — the workspace `AGENTS.md` body appears in the system
  message, and the message names its path.
- `agents-md-from-parent` — running in `pkg/`, an `AGENTS.md` one directory up is
  included.
- `agents-md-order` — with both files present, the outer body appears before the
  inner one.
- `no-agents-md` — with no such file, the base instruction is still sent and no
  project section appears.
- `empty-agents-md` — a whitespace-only file produces no project section, and the
  base instruction is still there.
- `system-prompt-stable` — two runs in the same workspace produce identical system
  content.
- `user-message-preserved` — the user's prompt arrives unchanged as `messages[1]`,
  quotes and all.

## Notes

- **The base prompt is a constant.** No clock, no working directory sentence, no
  random id. The stability case runs the program twice and compares bytes.
- **Order is the resolution rule.** Walk up, reverse, then render. Do not sort by
  path length or alphabetically; both break the nearest-wins property.
- **Emit the heading once.** Track whether any file was included, and write
  `# Project instructions` before the first one only.
- **Do not rewrite the prompt.** Assembly adds a message; it does not wrap or trim
  the user's text.
- **Discovery is bounded.** Eight levels is enough for a monorepo and stops the
  walk from reaching `/`.

## Hints

<details><summary>Rung 1 — the mechanism</summary>

The system message is built before the loop starts, from parts. Discovery walks up
the tree; rendering goes back down it.
</details>

<details><summary>Rung 2 — the APIs</summary>

`os.Getwd`, `filepath.Dir` until it stops changing, `os.Stat`, `os.ReadFile`,
`strings.TrimSpace`, `strings.Builder`.
</details>

<details><summary>Rung 3 — what the code does</summary>

`BuildSystemPrompt` writes the trimmed base, then each non-empty contributor
section separated by a blank line. `AgentsFile.discover` collects `AGENTS.md` from
the working directory upward, bounded by depth, then reverses the slice.
`Contribute` reads each file, skips empty bodies, writes the heading once, and
renders `## <path>` followed by the body. In `main`, build the system string and
prepend `{Role: RoleSystem, Content: system}` to the history.

`kata hint 05 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 05` reports 8/8, and:

- an `AGENTS.md` at the repo root applies when you run the agent in a subdirectory;
- you can explain why the nearest file is rendered last;
- you can explain what a changing byte in the prefix costs.
