# 06 Example

*Full, annotated implementation. Read it after `kata submit 07` passes.*

The compilable files are `reference/ch07/agent/session.go` and
`reference/ch07/agent/cmd/agent/main.go`. The loop and the provider are unchanged.

---

## The shape

```
session.go   Recorder, JSONLSession, OpenSession, Load, Append
main.go      load prior, prepend system, run, save only the suffix
loop.go      untouched
```

## Step 1: open, and notice whether the file was new

```go
_, statErr := os.Stat(path)
isNew := errors.Is(statErr, os.ErrNotExist)

f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
```

`O_APPEND` is the important flag, not `O_CREATE`. Two writers with `O_APPEND` each
get their bytes appended atomically; two writers with a seek-and-write can
overwrite each other.

The `Stat` before the open is how the header is written exactly once. Checking
after opening cannot distinguish a new file from an old one.

## Step 2: the header carries a version

```go
header := sessionHeader{Type: recordHeader, Version: SessionVersion, Created: ...}
```

A version is cheap and its absence is expensive. When the record shape changes,
an old file must be rejected with a message, not parsed into a conversation that
is subtly wrong. `Load` refuses a `version` higher than the program understands.

## Step 3: load is a scanner with a probe

```go
var probe struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}
if err := json.Unmarshal(raw, &probe); err != nil {
	return nil, fmt.Errorf("session line %d: %w", line, err)
}

switch probe.Type {
case recordHeader: // version check only
case recordMessage:
	var rec sessionMessage
	if err := json.Unmarshal(raw, &rec); err != nil { ... }
	out = append(out, rec.Message)
}
```

Two passes of unmarshalling looks wasteful. It is not: the first pass decodes the
small envelope, and only message lines pay for the full decode. It also means an
unknown record type is skipped rather than misread, which is what makes the format
extensible.

`line` in the error is what turns "invalid character" into a sentence someone can
act on.

## Step 4: the slice that prevents duplication

```go
prior := []agent.Message{}
if *sessionPath != "" {
	rec, err := agent.OpenSession(*sessionPath)
	...
	prior, err = rec.Load()
}

history := make([]agent.Message, 0, len(prior)+2)
history = append(history, agent.Message{Role: agent.RoleSystem, Content: system})
history = append(history, prior...)
history = append(history, agent.Message{Role: agent.RoleUser, Content: *prompt})

res, err := agent.RunLoop(ctx, provider, tools, history, opts)

if recorder != nil {
	for _, m := range res.History[len(prior)+1:] {
		_ = recorder.Append(m)
	}
}
```

Index by index, with `prior` of length 2:

```
  history:   [0]=system  [1]=prior[0]  [2]=prior[1]  [3]=user  [4]=assistant
  save from: len(prior)+1 = 3
  writes:    [3]=user  [4]=assistant
```

Take the `+1` away and you write the system message into the session, which then
appears twice on the next resume. Use `len(history)` and you write nothing.

## Step 5: persist before returning the error

```go
res, err := agent.RunLoop(...)

if recorder != nil {
	for _, m := range res.History[len(prior)+1:] {
		if appendErr := recorder.Append(m); appendErr != nil && err == nil {
			err = appendErr
		}
	}
}
if err != nil {
	return err
}
```

The order matters. A run that hit the step limit produced real turns. Throwing
them away means the next run repeats the work that already happened.

The `appendErr != nil && err == nil` guard keeps the original failure: if the loop
failed *and* the disk is full, the loop's error is the one a user needs.

## The diffs

**1. Saving the loaded history too** (`no-duplicate-turns`)

```diff
- for _, m := range res.History { rec.Append(m) }
+ for _, m := range res.History[len(prior)+1:] { rec.Append(m) }
```

**2. Storing the system message** (`system-not-duplicated`)

```diff
- rec.Append(agent.Message{Role: agent.RoleSystem, Content: system})
+ // not stored: rebuilt from AGENTS.md on every run
```

**3. Rewriting the file** (`appends-not-overwrites`)

```diff
- os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
+ os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
```

**4. Recording results but not calls** (`tool-history-persisted`)

```diff
- if m.Role == agent.RoleTool { rec.Append(m) }
+ for _, m := range res.History[len(prior)+1:] { rec.Append(m) }
```

## Complete files

| File | Lines | What to read it for |
|---|---|---|
| `reference/ch07/agent/session.go` | 185 | format, versioning, the two-pass decode |
| `reference/ch07/agent/cmd/agent/main.go` | 150 | load, prepend, save the suffix |

## Comparison checklist

- [ ] append-only file, header written once
- [ ] header carries a version, and a newer version is rejected
- [ ] load skips the header and collects message records only
- [ ] malformed line reports the line number
- [ ] system message rebuilt, never stored
- [ ] save starts at `len(prior)+1`
- [ ] persistence happens before the error return
- [ ] missing file and missing directories both handled
- [ ] assistant tool calls recorded alongside results

Differences outside this list are style.
