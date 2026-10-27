# 19 Example

*The finished implementation, taken apart. Open this after `miyagi submit 19` passes.*

The file it describes is `reference/ch19/agent/store.go`.

---

## Opening the database

```go
db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
if err != nil {
	return nil, err
}
if _, err := db.Exec(schema); err != nil {
	_ = db.Close()
	return nil, fmt.Errorf("create schema: %w", err)
}
```

Two pragmas, each earning its place. `foreign_keys` is off by default in SQLite, and
without it `ON DELETE CASCADE` does nothing, so deleting a session leaves its messages
behind forever. `busy_timeout` makes a second process wait for a lock instead of failing
immediately, which matters the moment you run two agents in one workspace.

`db.Exec(schema)` with `CREATE TABLE IF NOT EXISTS` is the migration strategy for this
chapter. It is not a strategy that survives a change to an existing column, and the
comment says so rather than implying otherwise.

## Identity from the clock and a suffix

```go
now := time.Now().UTC()
id := fmt.Sprintf("%s-%04x", now.Format("20060102-150405"), now.UnixNano()&0xffff)
```

Readable first, unique second. `20261027-121501-9a3f` sorts by time, looks like a
timestamp to a person, and the four hex characters are enough to separate two sessions
started in the same second.

A counter would need a table. A random UUID would be unique and unreadable. This is the
middle: sortable, printable, and short enough to paste.

## Times need microseconds

```go
const stampLayout = "2006-01-02T15:04:05.000000Z07:00"

func stamp(t time.Time) string { return t.UTC().Format(stampLayout) }
```

`time.RFC3339` writes `2026-10-27T12:15:01Z`, which has one-second resolution. Two runs
inside the same second then compare equal, and `ORDER BY updated DESC` returns them in
whatever order SQLite feels like. `-continue` picks the wrong conversation, intermittently,
which is the worst kind of bug to reproduce.

## Listing

```go
SELECT s.id, s.title, s.created, s.updated,
       (SELECT COUNT(*) FROM messages m WHERE m.session_id = s.id)
FROM sessions s
ORDER BY s.updated DESC
```

A correlated subquery for the count, rather than a `JOIN ... GROUP BY`. Both work; the
subquery reads as what it is, and the plan is a single index lookup per session on a table
that holds tens of rows.

`ORDER BY updated DESC` is the whole point. The list is ordered by what the person touched
last, not by when the conversation was created.

## Loading one conversation

```go
SELECT role, content, tool_calls, tool_call_id, name
FROM messages WHERE session_id = ? ORDER BY ordinal
```

Every column for a message, in the order the conversation had them. `json.Unmarshal` on
the `tool_calls` column rebuilds the slice, and an empty string means no calls.

The `WHERE session_id = ?` is what keeps two conversations apart. Without it the store is
one long thread and every resume returns everything.

## Appending inside a transaction

```go
tx, err := s.db.BeginTx(ctx, nil)
defer func() { _ = tx.Rollback() }()

var start int
tx.QueryRowContext(ctx,
	`SELECT COALESCE(MAX(ordinal)+1, 0) FROM messages WHERE session_id = ?`, id).Scan(&start)
```

The starting ordinal is read inside the transaction, so it cannot be stale by the time the
inserts run. Reading it in the caller and passing it in is the bug that produces two
messages at the same position, and the unique constraint rejects the second after the first
has been committed.

`COALESCE(..., 0)` handles the first message of a new session, where `MAX` returns `NULL`.

`defer tx.Rollback()` after a successful commit returns an error that is ignored. That is
the standard idiom: the rollback is for the failure path, and committing first is
harmless.

## Touching `updated`

```go
_, err = tx.ExecContext(ctx, `UPDATE sessions SET updated = ? WHERE id = ?`,
	stamp(time.Now().UTC()), id)
```

In the same transaction as the messages, so the list order cannot disagree with the
contents. A session with five messages and an `updated` from yesterday sorts below one with
one message from a minute ago, which is correct: it is a recency-of-use ordering.

## `-sessions` before the prompt check

```go
if *listSess {
	...
	return 0, nil
}

if *prompt == "" {
	return 2, errors.New(`-p is required`)
}
```

Listing is read-only. It needs no prompt and no provider, so it is handled first. The
chapter's first draft validated `-p` first, and `-sessions` returned "a prompt is
required" forever.

The same rule applies to any new subcommand: decide what it actually needs before
inheriting the requirements of the main path.

## Checking an id exists

```go
ok, err := sessions.Exists(ctx, sessionID)
if !ok {
	return 1, fmt.Errorf("no such session: %s", sessionID)
}
```

`Messages` alone cannot tell "empty session" from "no such session", because both return
zero rows. The separate existence check is what turns a typo into a sentence.

## The fixes that matter

**1. No session column** (`two-sessions-do-not-mix`)

```diff
- CREATE TABLE messages (ordinal INTEGER, ...)
+ CREATE TABLE messages (session_id TEXT NOT NULL REFERENCES sessions(id), ordinal INTEGER, ...)
```

**2. Ordinal computed in the caller** (`resumes-by-id`)

```diff
- ordinal := len(prior) + i
+ SELECT COALESCE(MAX(ordinal)+1, 0) ... inside the transaction
```

**3. Seconds-resolution timestamps** (`continue-uses-latest`)

```diff
- now.Format(time.RFC3339)
+ now.Format("2006-01-02T15:04:05.000000Z07:00")
```

**4. Validating the prompt before `-sessions`** (`lists-sessions`)

```diff
- if *prompt == "" { return error } ... if *listSess { ... }
+ if *listSess { ... } ... if *prompt == "" { return error }
```

## What to compare against your own

- [ ] every message row carries its session id
- [ ] the ordinal is unique per session and dense from zero
- [ ] times have sub-second precision
- [ ] `-continue` picks the most recently updated session
- [ ] `-resume` with an unknown id fails and says so
- [ ] tool calls and tool results round-trip
- [ ] `-sessions` needs no prompt and no provider
- [ ] the file-per-session recorder still works

Everything outside that list is style.
