# 03 Problem

> **Your task:** add `write` and `bash` to the agent, and bound what they return
> and how long they may take.

**You edit:** `agent/tools.go`, `agent/cmd/agent/main.go`
**Given:** `agent/tool.go` (the registry from chapter 02), `agent/types.go`
**Time:** 60 to 90 minutes

---

## The contract

Two new tools, same interface as `read`:

```go
type WriteTool struct{ Root string }

type BashTool struct {
    Root     string
    Timeout  time.Duration // 0 means 10s
    MaxBytes int           // 0 means 20000
}
```

The program gains one flag, which the checks use:

```sh
agent -p "run the tests" -tool-timeout 300ms
```

`-tool-timeout` bounds **one tool execution**. It is separate from `-timeout`,
which bounds **one provider request**.

Arguments:

| Tool | Arguments |
|---|---|
| `write` | `{"path": string, "content": string}`, both required |
| `bash` | `{"command": string}`, run with `sh -c` in the workspace |

## What the verifier does

It builds the binary, creates a temporary workspace, scripts a tool call, runs the
program with that directory as its working directory, then checks three things:
the request trace, the files on disk, and stdout.

Disk matters here. A tool result that claims a write is not evidence that the file
exists.

## The verifier will assert that

- `advertises-three-tools`: the request advertises `read`, `write` and `bash`.
- `writes-a-file`: after the run, the file exists with exactly the requested
  contents.
- `write-creates-parents`: `a/b/c/deep.txt` is created even though none of those
  directories existed.
- `write-refuses-escape`: `../escaped.txt` is not created, and the tool result
  names the escape.
- `bash-runs-a-command`: `ls && cat here.txt` returns the file contents, proving
  the working directory is the workspace.
- `bash-reports-exit-code`: `echo before; exit 3` yields a result containing both
  the output and the code, and the program exits 0.
- `bash-truncates-output`: 5000 lines of output yields at most 25 KB of tool
  content, with a marker containing `truncated`.
- `bash-timeout-is-a-result`: with `-tool-timeout 300ms`, `sleep 5` produces a
  result containing `timed out`, the program exits 0, and the whole run finishes
  in under three seconds.

## Notes

- **Order of operations for `write`:** resolve, refuse, create parents, write. If
  you create the parent before resolving, an escape can leave directories behind.
- **Combined output.** Capture stdout and stderr together. A failing command
  usually explains itself on stderr.
- **A timeout is not an error.** It is a result. A cancelled run is an error.
  Distinguish `runCtx.Err()` from `ctx.Err()`.
- **Known limitation:** `exec.CommandContext` kills the direct child only. Killing
  a process group needs platform-specific code. Do not attempt it here; the
  overview explains why.
- **Deliberate omission:** no parallel tool execution, no permission prompts.
  Chapter 04 adds the loop; chapter 08 adds policy.

## Hints

<details><summary>Rung 1, the mechanism</summary>

Three edges: whose input, how much output, how long. Each tool answers all three
before it does any work.
</details>

<details><summary>Rung 2, the APIs</summary>

`os.MkdirAll`, `os.WriteFile`, `exec.CommandContext`, `cmd.CombinedOutput`,
`errors.As(err, &exitErr)`, `context.WithTimeout`.
</details>

<details><summary>Rung 3, what the code does</summary>

Share one `resolveInside(root, p)` between `read` and `write`. `write` calls
`MkdirAll(filepath.Dir(full), 0o755)` then `WriteFile(full, content, 0o644)` and
returns the byte count. `bash` wraps the caller's context with the tool timeout,
runs `sh -c` with `cmd.Dir = Root`, caps the combined output, and maps the
outcome: deadline to a timeout result, `*exec.ExitError` to an exit-code result,
anything else to a Go error.

`kata hint 03 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 03` reports 8/8, and:

- you can ask the agent to make a file and it appears;
- you can explain why an exit code 3 is not an error;
- you know which of the three edges your first implementation got wrong.
