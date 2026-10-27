# 15 Example

*The finished implementation, taken apart. Open this after `kata submit 15` passes.*

The file it describes is `reference/ch15/agent/spill.go`.

---

## When to move, and when to leave it alone

```go
func (s *SpillStore) Put(name, content string, limit int) (string, bool, error) {
	if limit <= 0 {
		limit = DefaultSpillLimit
	}
	if len(content) <= limit {
		return content, false, nil
	}
	...
}
```

The early return is the whole difference between a ceiling and a rule. Most tool results
are small, and moving them to disk costs a file write plus a round trip to read it back.

The `name` parameter is accepted and unused. A caller that wants per-tool directories can
pass the tool name; this chapter keeps one flat directory because the handle is a hash
and collisions between tools are not a problem worth solving.

## Content addressing

```go
sum := sha256.Sum256([]byte(content))
handle := hex.EncodeToString(sum[:8])
path := filepath.Join(s.Dir, handle+".txt")
```

Eight bytes of a SHA-256 is sixteen hex characters: short enough to read in a message,
and long enough that two different results colliding is not a practical concern at this
scale.

Hashing rather than counting has a second benefit. The same `kata test` twice produces
the same handle, so the directory does not fill with copies of identical output.

## The notice

```go
head := content[:limit]
lines := strings.Count(content, "\n") + 1

summary := fmt.Sprintf(
	"%s\n\n... [moved to disk: %d bytes, %d lines in total]\nhandle: %s\nRead it with the read_result tool: {\"handle\": %q, \"offset\": %d}",
	head, len(content), lines, handle, handle, limit)
```

Four things, each doing a job:

- **the head**, so the model can often answer without a second call;
- **the size in bytes and lines**, so it knows how much it is missing;
- **the handle**, which is the only name it can use;
- **a literal call**, because a JSON example is followed far more reliably than a
  sentence describing one.

`offset: limit` rather than `offset: 0`, so the follow-up continues where the head
stopped instead of repeating it.

## Reading a slice

```go
if offset >= total {
	return "", total, fmt.Errorf("offset %d is past the end of %d bytes", offset, total)
}

buf := make([]byte, length)
n, err := f.ReadAt(buf, int64(offset))
```

`ReadAt` rather than seeking and reading, because it does not move the file position and
cannot interfere with a concurrent read of the same handle.

Returning the total alongside an error is deliberate: the caller needs the real size to
say something useful. "Offset past the end" is less helpful than "offset past the end of
20044 bytes".

## Validating the handle

```go
for _, r := range handle {
	switch {
	case r >= '0' && r <= '9':
	case r >= 'a' && r <= 'f':
	default:
		return "", fmt.Errorf("invalid handle: %q", handle)
	}
}
```

An allow-list, not a deny-list. Rejecting `..` and `/` means remembering every way a path
can escape; accepting only hex means the question does not arise.

## Wrapping the registry

```go
func WithSpill(r *Registry, store *SpillStore, limit int) *Registry {
	out := &Registry{byName: make(map[string]Tool, len(r.order))}
	for _, name := range r.order {
		out.byName[name] = &spillTool{inner: r.byName[name], store: store, limit: limit}
		out.order = append(out.order, name)
	}
	return out
}
```

A decorator over the registry rather than a change inside it. Every tool gets the same
treatment without knowing it exists, and a caller that wants raw output keeps the
original registry.

`spillTool` forwards `Name`, `Description` and `Parameters` unchanged, so the schema the
model sees is still the tool's own. Only `Execute` differs.

## The fixes that matter

**1. Cutting instead of moving** (`large-result-moved`)

```diff
- return content[:limit] + "... [truncated]"
+ return s.Put(name, content, limit)   // writes the whole thing, returns a summary
```

**2. A handle with no way to use it** (`read-result-fetches-a-slice`)

```diff
- summary := fmt.Sprintf("... handle: %s", handle)
+ summary := fmt.Sprintf("... handle: %s\nRead it with the read_result tool: {...}", handle)
```

**3. Trusting the handle as a filename** (`handle-is-not-a-path`)

```diff
- path := filepath.Join(s.Dir, handle+".txt")
+ // validate hex first, then join
```

**4. An error instead of a result** (`unknown-handle-is-a-result`)

```diff
- if os.IsNotExist(err) { panic(err) }
+ return "", 0, fmt.Errorf("no result stored under handle %q", handle)
```

## What to compare against your own

- [ ] small results pass through untouched
- [ ] the whole result is on disk, not the truncated version
- [ ] the notice names the size, the handle and the exact next call
- [ ] the first slice is inline, so the model can start working
- [ ] slices report the total size and the next offset
- [ ] the handle is validated as hex before it becomes a path
- [ ] a missing handle and a bad offset are results, not crashes

Everything outside that list is style.
