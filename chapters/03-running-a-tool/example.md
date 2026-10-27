# 03 Example

*The finished implementation, taken apart. Read this after `miyagi submit 03` passes, or
after a real attempt.*

The file it describes is `reference/ch03/agent/tools.go`, plus two lines in
`reference/ch03/agent/cmd/agent/main.go`.

---

## The shape

One small helper, and one tool that uses it.

```
resolveInside(root, p)   the workspace rule. the next chapter reuses it
ReadTool                 resolve, stat, read, cap
```

The helper earns its place later. The next chapter adds `write`, and a write that escapes
the workspace creates files on your machine, so both tools must share one rule rather
than each having its own.

## Step 1: the workspace rule

```go
func resolveInside(root, p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	full, err := filepath.Abs(filepath.Join(root, p))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace: %s", p)
	}
	return full, nil
}
```

**Resolve first, then check.** Looking for `..` in the string is not enough. The path
`a/../../etc/passwd` contains `..`, but so does the harmless `notes..txt`. And a
symlink defeats a string check entirely. Build the real path, then ask whether it is
still inside the root.

**Refuse absolute paths outright.** A model has no reason to name `/etc/passwd`, and
allowing it makes the check depend on which machine you are on.

## Step 2: the read itself

```go
info, err := os.Stat(full)
if err != nil {
	if os.IsNotExist(err) {
		return "", fmt.Errorf("no such file: %s", args.Path)
	}
	return "", err
}
if info.IsDir() {
	return "", fmt.Errorf("is a directory: %s", args.Path)
}

limit := t.MaxBytes
if limit <= 0 {
	limit = DefaultReadLimit
}

f, err := os.Open(full)
if err != nil {
	return "", err
}
defer f.Close()

buf := make([]byte, limit+1)
n, err := f.Read(buf)
if err != nil && n == 0 {
	return "", err
}
if n > limit {
	return string(buf[:limit]) + fmt.Sprintf(
		"\n... [truncated: %d of %d bytes shown]", limit, info.Size()), nil
}
return string(buf[:n]), nil
```

Four decisions in that block.

**`os.Stat` before `os.Open`.** It is how you tell "missing" from "a directory" from
"a permission problem", and each deserves a different sentence for the model.

**`limit+1` bytes.** Reading one byte more than you will send is how you learn that
there *was* more. Without the extra byte, a 50 KB file and a file of exactly 20 KB look
identical.

**The marker is part of the result.** `[truncated: 20000 of 51234 bytes shown]` tells
the model it is blind past a point, so it can ask for a specific range instead of
guessing. A silent cut is worse than no cut at all.

**Return errors, not error strings.** `Execute` returns `(string, error)`. The registry
from the Tool calling chapter turns an error into an `IsError` result. Keeping those separate is what
lets a later chapter distinguish "the tool failed" from "the tool returned text that
looks like a failure".

## Step 3: register it

```go
tools := agent.NewRegistry(&agent.ReadTool{Root: root})
```

`root` is the working directory, not the source tree. That one line is what makes the
checks meaningful: they run your binary inside a temporary directory, and a path
resolved against anything else reads the wrong file.

## The fixes that matter

**1. Checking the string for `..`** (`refuses-escape`)

```diff
- if strings.Contains(p, "..") { reject }
+ full, _ := filepath.Abs(filepath.Join(root, p))
+ rel, _ := filepath.Rel(root, full)
+ if rel == ".." || strings.HasPrefix(rel, "../") { reject }
```

**2. Returning an error for a missing file** (`missing-file`)

```diff
- if err != nil { return "", err }        // becomes a fatal error upstream
+ return fmt.Errorf("no such file: %s", args.Path)   // a result the model can read
```

**3. Reading the whole file** (`truncates-large-file`)

```diff
- data, _ := os.ReadFile(full)
+ buf := make([]byte, limit+1)
+ n, _ := f.Read(buf)
+ if n > limit { return string(buf[:limit]) + marker }
```

## What to compare against your own

- [ ] absolute paths refused
- [ ] the escape check runs on the resolved path
- [ ] missing file, directory and permission errors are told apart
- [ ] the read is bounded, and the cut is marked
- [ ] the tool registered with the working directory as its root
- [ ] errors returned as errors, not as text

Everything outside that list is style.
