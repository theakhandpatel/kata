# 04 Example

*Full, annotated implementation. Read it after `kata submit 04` passes.*

The compilable files are `reference/ch04/agent/tools.go` and
`reference/ch04/agent/cmd/agent/main.go`. This document takes them apart in the
order the thinking happens.

---

## The shape

`tools.go` holds three tools and one shared helper. The helper is the important
part, because it is the only place the workspace rule lives.

```
resolveInside(root, p)   ← one rule, used by read and write
ReadTool                 ← read, capped
WriteTool                ← resolve, parents, write
BashTool                 ← bound, run, cap, classify
```

## Step 1: one boundary check, two tools

```go
func resolveInside(root, p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	full, _ := filepath.Abs(filepath.Join(root, p))
	rel, _ := filepath.Rel(root, full)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace: %s", p)
	}
	return full, nil
}
```

Putting it here rather than in each tool means `write` cannot drift from `read`.
That matters more than it looks: a write escape creates files on the host, so the
two tools failing differently is a real bug class.

## Step 2: write creates parents after it resolves

```go
full, err := resolveInside(t.Root, args.Path)
if err != nil {
	return "", err
}
if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
	return "", err
}
if err := os.WriteFile(full, []byte(args.Content), 0o644); err != nil {
	return "", err
}
return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
```

Order matters. Resolve, then create. Reversed, a refused path can still leave an
empty directory tree outside the workspace, which is a small bug with a large
blast radius.

The result reports the byte count. The model can confirm the write without calling
`read` to check, which would spend the same context twice.

## Step 3: bash classifies four outcomes

```go
runCtx, cancel := context.WithTimeout(ctx, wait)
defer cancel()

cmd := exec.CommandContext(runCtx, "sh", "-c", args.Command)
cmd.Dir = t.Root
cmd.WaitDelay = time.Second

out, err := cmd.CombinedOutput()

if err != nil {
	if ctx.Err() != nil {                       // the user cancelled
		return "", ctx.Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {   // the tool's own bound
		return fmt.Sprintf("command timed out after %s\n%s", wait, body), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {                  // the command failed normally
		return fmt.Sprintf("exit code: %d\n%s", exit.ExitCode(), body), nil
	}
	return "", err                              // the process could not start
}
return "exit code: 0\n" + body, nil
```

The order of those four checks is the lesson.

`ctx.Err()` first, because a cancelled parent must not be reported as a tool
failure. `runCtx` second, because the timeout belongs to this tool and becomes a
result. `*exec.ExitError` third, because a failing command is information.
Everything else is a genuine error.

Get the order wrong and one of two things happens: cancelling the agent looks like
a broken command, or a hung command looks like a crash.

## Step 4: cap the output, mark the cap

```go
func (t *BashTool) cap(s string) string {
	limit := t.MaxBytes
	if limit <= 0 {
		limit = DefaultBashLimit
	}
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n... [truncated: %d of %d bytes shown]", limit, len(s))
}
```

This is a byte cap, not a line cap, and it can cut a multi-byte UTF-8 sequence in
half. That is a real bug with a real fix (`utf8.DecodeLastRune`), and it is left
out on purpose: the course grades the cap and the marker, not the rune boundary.
Say so in a comment if you fix it.

## Step 5: the flag is the contract

```go
toolTTL = flag.Duration("tool-timeout", 10*time.Second, "cancel one tool call after this long")
...
&agent.BashTool{Root: root, Timeout: *toolTTL},
```

Without a way to shorten it, the timeout case either takes ten seconds or cannot
be tested at all. A bound you cannot exercise is a bound you cannot trust, and
this chapter's own first draft proved it: the case passed a command that finished
before the default fired, so it graded nothing.

That is worth keeping in mind for your own work. A test that cannot fail is not a
test.

## The diffs

**1. Trusting the path** (`write-refuses-escape`)

```diff
- full := filepath.Join(t.Root, args.Path)
+ full, err := resolveInside(t.Root, args.Path)
```

**2. Creating parents before resolving** (`write-refuses-escape`)

```diff
- os.MkdirAll(filepath.Dir(filepath.Join(t.Root, p)), 0o755)
- full, err := resolveInside(t.Root, p)
+ full, err := resolveInside(t.Root, p)
+ os.MkdirAll(filepath.Dir(full), 0o755)
```

**3. Treating a non-zero exit as an error** (`bash-reports-exit-code`)

```diff
- if err != nil { return "", err }
+ if errors.As(err, &exit) {
+     return fmt.Sprintf("exit code: %d\n%s", exit.ExitCode(), body), nil
+ }
```

**4. Passing the tool timeout to the caller's context** (`bash-timeout-is-a-result`)

```diff
- cmd := exec.CommandContext(ctx, "sh", "-c", command)   // uses the request bound
+ runCtx, cancel := context.WithTimeout(ctx, t.Timeout)
+ cmd := exec.CommandContext(runCtx, "sh", "-c", command)
```

## Complete files

| File | What to read it for |
|---|---|
| `reference/ch04/agent/tools.go` | the shared boundary helper, the four-way classification |
| `reference/ch04/agent/cmd/agent/main.go` | the `-tool-timeout` flag and the three-tool registry |

## Comparison checklist

- [ ] `read` and `write` share one path check
- [ ] absolute paths refused, escapes refused after resolution
- [ ] parents created only after the path is accepted
- [ ] `bash` runs `sh -c` with the workspace as its working directory
- [ ] combined output captured
- [ ] exit codes returned as results, not errors
- [ ] tool timeout distinct from request timeout
- [ ] timeout is a result, cancellation is an error
- [ ] output capped with a visible marker

Differences outside this list are style.
