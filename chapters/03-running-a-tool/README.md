# 03 Running a tool

**Part II · Act** · Difficulty: medium · 40 to 60 minutes · 6 graded cases

The model asks you to read a file. This chapter puts a real tool behind that request:
you open the file, send the contents back, and handle the three ways it can go wrong.

## What you are building

A program that does what the model asks, when what it asks for is a file:

```sh
$ echo "hello from a file" > notes.txt
$ ./agent -p "read notes.txt and tell me what it says"
The file says hello from a file.
```

Two requests happen here. The first asks the model what to do. It answers with a call
to `read`. You run the tool, send the result back, and the second request gets the
answer in words.

You may edit `agent/tool.go`, `agent/tools.go`, and `agent/cmd/agent/main.go`. The
registry and the wire format already exist from the Tool calling chapter. You are adding the part
that touches the disk.

## A tool is four things

**A tool is a name, a description, a schema, and a function.** The model reads the name
and description to decide whether to call it. The schema tells it what arguments to
write. Your function receives those arguments as JSON and returns text.

```
   model: "call read with {path: 'notes.txt'}"
     ↓
   your registry: look up "read" → ReadTool
     ↓
   ReadTool.Execute: resolve the path, open the file, read at most 20 KB
     ↓
   tool result: "hello from a file"
     ↓
   model: "The file says hello from a file."
```

**The model cannot see your filesystem.** Everything it knows about a file comes from
a result you sent it. That is why a truncation marker matters. Without one, the model
believes it read the whole file and will answer confidently about the part it never
saw.

**Three ways this goes wrong, and none of them should stop the program.**

The file does not exist. The model wrote a path, and the path is wrong. Return
`no such file: nope.txt` as the result. The model reads it and tries another path. If
you exit instead, the conversation ends and the model never gets the chance.

The path climbs out of your project. `../../etc/passwd` would read a file on your
machine that the model has no business seeing. Check the resolved path, not the string,
because `a/../../etc/passwd` contains `..` while `notes..txt` also does.

The file is enormous. A 50 MB log pasted into the conversation costs more than the
whole answer is worth. Read at most 20,000 bytes, and say that you stopped.

| Situation | What the model receives | What your program does |
|---|---|---|
| file missing | `no such file: nope.txt` | keeps going, exits 0 |
| path escapes | `path escapes the workspace: ../../etc/passwd` | keeps going, exits 0 |
| file is huge | the first 20,000 bytes and a marker | keeps going, exits 0 |
| no tool needed | nothing | one request, exits 0 |

## What you need to build

```go
type ReadTool struct {
    Root     string   // the workspace, absolute
    MaxBytes int      // 0 means 20000
}

func (t *ReadTool) Name() string                        // "read"
func (t *ReadTool) Description() string
func (t *ReadTool) Parameters() map[string]any          // JSON schema for {path}
func (t *ReadTool) Execute(ctx context.Context, args json.RawMessage) (string, error)
```

The `Tool` interface is already declared in `agent/types.go`. Register your tool in
`main` and the rest of the machine does its job.

The workspace root is the directory the program was started in, not your source tree.
The checks run the binary inside a temporary directory, and a path resolved against
anything else reads the wrong file.

## How you are checked

The verifier builds your program, creates a temporary workspace with files in it,
starts a server that asks for `read` on the first turn, and runs your binary with that
directory as its working directory. Then it looks at the second request and at what the
tool returned.

- `advertises-read`: the request advertises a tool named `read` whose parameters
  declare a `path`.
- `reads-a-file`: the second request carries a tool message with the file's contents,
  paired with the call id.
- `missing-file`: the result says `no such file`, and the program exits 0 after
  printing the model's answer.
- `refuses-escape`: `../../etc/passwd` produces a refusal naming the escape, and no
  file contents appear.
- `truncates-large-file`: a 50 KB file yields at most 25 KB and a marker containing
  `truncated`.
- `text-only-single-request`: a plain reply costs exactly one request, not two.

## If you get stuck

<details><summary>First hint: what kind of problem is this?</summary>

Either the model never sees your tool (the request is wrong), the tool runs but the
result does not reach the model (the message is wrong), or the tool fails and the
program dies (the error handling is wrong). The failing case id tells you which.
</details>

<details><summary>Second hint: the tools you will need</summary>

`filepath.Abs` and `filepath.Rel` for the workspace check. `os.Stat` to tell a missing
file from a directory. `os.Open` and `f.Read` with `limit+1` bytes to notice an
overflow. `strings.HasPrefix` on the relative path to detect an escape.
</details>

<details><summary>Third hint: what the finished tool does</summary>

Refuse an absolute path. Join the workspace root and the argument, take the absolute
result, and compare it with the root using `filepath.Rel`. If the result is `..` or
starts with `../`, refuse and name the escape.

Otherwise stat the file, open it, and read at most `limit+1` bytes. If you read more
than `limit`, return the first `limit` bytes followed by
`... [truncated: N of M bytes shown]`. Return an error for a missing file, and let the
registry turn that error into a result the model can read.

`miyagi hint 03 <case-id>` prints these same hints in the terminal.
</details>

## Done when

`miyagi submit 03` reports 6/6, and:

- asking the agent to read a file gets you the contents;
- a missing file produces an answer, not a crash;
- a path outside your project is refused.

When it passes, [example.md](example.md) shows the finished code.

---

Next: **04 Tools that touch disk**, where `write` and `bash` join `read`, and the hard
part becomes bounding what comes back.
