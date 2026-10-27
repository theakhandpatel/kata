# 06 Problem

> **Your task:** record the conversation to a JSONL file and resume from it on the
> next run, without duplicating what is already there.

**You edit:** `agent/session.go`, `agent/cmd/agent/main.go`
**Given:** `agent/loop.go`, `agent/prompt.go`, `agent/types.go` (now with JSON tags)
**Time:** 60 to 90 minutes

---

## The contract

```go
// session.go
const SessionVersion = 1

type Recorder interface {
    Load() ([]Message, error)
    Append(Message) error
    Close() error
}

func OpenSession(path string) (*JSONLSession, error)
```

File format, one object per line:

```
{"type":"header","version":1,"created":"..."}
{"type":"message","message":{"role":"user","content":"..."}}
{"type":"message","message":{"role":"assistant","content":"..."}}
```

The program gains one flag:

```sh
agent -p "..." -session work.jsonl
```

Absent file means a new session, including missing parent directories.

## What the verifier does

It runs the binary **twice** against the same session path with different scripted
servers, then inspects the second run's request and the file itself. Some cases
add a tool call on the first run to check that the pairing survives.

## The verifier will assert that

- `creates-session-file`: the file exists and is not empty after one run.
- `session-is-jsonl`: every line parses as JSON with a `type`, and line 1 is a
  header carrying a version.
- `resumes-history`: the second request is `system`, `user`, `assistant`, `user`,
  in that order, with the first run's text intact.
- `appends-not-overwrites`: after two runs the file grew, and the first question
  appears exactly once.
- `no-duplicate-turns`: the first run's answer appears exactly once in the second
  request.
- `system-not-duplicated`: exactly one `system` message, at index 0, rebuilt from
  the current `AGENTS.md`.
- `tool-history-persisted`: the resumed request contains both the assistant's
  `tool_calls` entry and the matching `tool` result.
- `missing-session-is-created`: a path in a non-existent directory works, and the
  next run resumes it.

## Notes

- **Store conversation, rebuild policy.** The system message never goes in the
  file. Rebuilding it means `AGENTS.md` edits take effect on resume.
- **Save only what this run added.** `res.History[len(prior)+1:]` skips the system
  message and the loaded history. Without the slice, every resume doubles the
  conversation.
- **Persist on failure too.** A run that hit the step limit still produced turns
  worth keeping. Record them, then return the error.
- **The header is written once.** Only when the file did not exist.
- **A malformed line is an error.** Silently skipping corruption turns a broken
  session into a subtly different conversation.

## Hints

<details><summary>Rung 1, the mechanism</summary>

A session is an append-only log of messages. Loading gives you a prefix; saving
must write only the suffix you just created.
</details>

<details><summary>Rung 2, the APIs</summary>

`os.OpenFile` with `O_CREATE|O_WRONLY|O_APPEND`, `bufio.Scanner` over the file,
`encoding/json` per line, `errors.Is(err, os.ErrNotExist)` to detect a new file.
</details>

<details><summary>Rung 3, what the code does</summary>

`OpenSession` creates parent directories, checks whether the file exists, opens it
for append, and writes a header only when it is new. `Load` scans line by line,
unmarshals a probe containing `type` and `version`, skips the header, and collects
`message` records. `Append` marshals one record plus a newline. In `main`, load
prior messages, build `[system] + prior + [user]`, run the loop, then append
`res.History[len(prior)+1:]` before returning.

`kata hint 06 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 06` reports 8/8, and:

- a fact told in one run is recalled in the next;
- the session file grows by exactly the new turns, never by the old ones;
- you can explain what the `+1` in `len(prior)+1` skips.
