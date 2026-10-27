# 19 Session storage

**Part VII · Store** · Difficulty: medium-hard · 60 to 90 minutes · 8 graded cases

## What you are building

A place where conversations live, so you can go back to one.

```sh
$ ./agent -p "why does the deploy fail on fridays"
The deploy runs at 23:50 UTC, so it lands on Saturday for the audit job...

$ ./agent -sessions
ID                           TITLE                                       MSGS  LAST USED
20261027-121501-9a3f         why does the deploy fail on fridays            6  2026-10-27 12:15
20261026-183004-1c02         add the retry flag                             4  2026-10-26 18:31

$ ./agent -continue -p "and what about the audit job?"
...
```

Three commands: start a conversation, list what you have, pick up where you left off.

You edit `agent/cmd/agent/main.go`. You also create one new file, `agent/store.go`, holding the schema, the queries and `TitleFromPrompt`.
## A file per conversation is not enough

The previous chapters wrote one JSONL file that you named yourself. That works for a
single transcript and fails as soon as there are twenty, because:

| Problem | Consequence |
|---|---|
| no identity | you cannot resume without the exact filename |
| no list | you cannot find a conversation you had last week |
| no title | even a directory listing is unreadable |
| no index | finding anything means `grep` across files |

A database answers the question a person actually has: what did I talk about, and which
one was it.

## The schema

```sql
CREATE TABLE sessions (
    id       TEXT PRIMARY KEY,
    title    TEXT NOT NULL,
    created  TEXT NOT NULL,
    updated  TEXT NOT NULL
);

CREATE TABLE messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal      INTEGER NOT NULL,
    role         TEXT NOT NULL,
    content      TEXT NOT NULL,
    tool_calls   TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    UNIQUE(session_id, ordinal)
);
```

Four decisions worth defending:

**`session_id` on every message.** Without it a store is one long thread, and every
conversation bleeds into every other.

**`ordinal`, not `id`.** Auto-increment ids are global; the order within a conversation
is what the provider needs. `UNIQUE(session_id, ordinal)` makes a duplicate position
impossible.

**`tool_calls` as JSON text.** A tool call is a small nested structure, and SQLite has no
array type. Storing it as JSON keeps the column count down and the round trip exact.

**`updated` on the session.** `-continue` means "the one I was last using", which is an
ordering question, not a recency-of-creation question.

## Ordinals are computed inside the transaction

```go
var start int
tx.QueryRowContext(ctx,
	`SELECT COALESCE(MAX(ordinal)+1, 0) FROM messages WHERE session_id = ?`, id).Scan(&start)
```

The obvious version counts messages in the caller and passes the number in. That is how a
resumed session ends up with two messages at position 5, and the unique constraint rejects
the second one after the first has already been written.

## Titles come from the first prompt

A list of timestamps is unreadable. The first thing the person asked is what they
recognise their own conversation by:

```
why does the deploy fail on fridays
add the retry flag
```

Trimmed to sixty characters, whitespace collapsed, with an ellipsis when it is longer.

## Resuming a session that does not exist

```sh
$ ./agent -resume typo-id -p "hello"
agent: no such session: typo-id
```

Silently starting a new conversation under a mistyped id loses the thread the person
thought they were rejoining, and they only notice two turns later.

## Timestamps need sub-second precision

`time.RFC3339` drops the fraction, so two runs in the same second order arbitrarily and
`-continue` picks the wrong one. Microseconds in a fixed layout:

```go
const stampLayout = "2006-01-02T15:04:05.000000Z07:00"
```

## What you need to build

```go
type Store struct{ /* ... */ }
type Session struct {
    ID       string
    Title    string
    Created  time.Time
    Updated  time.Time
    Messages int
}

func OpenStore(path string) (*Store, error)
func (s *Store) Create(ctx context.Context, title string) (string, error)
func (s *Store) List(ctx context.Context) ([]Session, error)
func (s *Store) Latest(ctx context.Context) (string, error)
func (s *Store) Exists(ctx context.Context, id string) (bool, error)
func (s *Store) Messages(ctx context.Context, id string) ([]Message, error)
func (s *Store) Append(ctx context.Context, id string, msgs []Message) error
func (s *Store) SetTitle(ctx context.Context, id, title string) error
func TitleFromPrompt(prompt string, max int) string
```

Four flags:

```sh
agent -p "..."                    start a new conversation
agent -sessions                   list them
agent -resume <id> -p "..."       continue one by id
agent -continue -p "..."          continue the most recent
```

The database defaults to `<workspace>/.agent/sessions.db`.

## How you are checked

The verifier builds your program and runs it several times against the same store,
reading stdout, stderr, the request trace, and the database file.

- `creates-a-session`: the database exists after the first run.
- `lists-sessions`: two conversations both appear in `-sessions` output, by title.
- `title-from-first-prompt`: the title is the first prompt, trimmed.
- `resumes-by-id`: `-resume` puts the earlier prompt back in the request.
- `continue-uses-latest`: `-continue` picks the most recent and does not bring in the
  older one.
- `tool-calls-round-trip`: a tool call and its result both survive a resume.
- `two-sessions-do-not-mix`: resuming one conversation never shows the other's messages.
- `unknown-id-is-reported`: `-resume` with a wrong id exits non-zero and names it.

## Notes

`-sessions` is read-only. It needs no prompt and no provider, so handle it before
validating either.

The store lives in the workspace by default, so a conversation about a project stays with
that project.

Keep `-session <file>` working. A single throwaway transcript is still the right tool for
piping and for `tail -f`, and the database is not a replacement for it.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either the rows are not keyed by session, the order is not stored, or the resume path
loads the wrong set.
</details>

<details><summary>Second hint: what you will need</summary>

`database/sql` with the `modernc.org/sqlite` driver, `CREATE TABLE IF NOT EXISTS`,
`BeginTx` for the append, and `json.Marshal` for the tool calls column.
</details>

<details><summary>Third hint: the finished store</summary>

`OpenStore` opens the database, sets `foreign_keys` and a busy timeout, and runs the
schema. `Create` inserts an id built from the clock and a random suffix. `List` selects the
sessions with a message count, ordered by `updated` descending. `Messages` selects by
`session_id` ordered by `ordinal`. `Append` opens a transaction, reads
`COALESCE(MAX(ordinal)+1, 0)`, inserts each message, and touches `updated`.

`kata hint 19 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 19` reports 8/8, and you can start a conversation, quit, list it, and pick it
up again tomorrow.

[example.md](example.md) has the finished code, after you pass.
