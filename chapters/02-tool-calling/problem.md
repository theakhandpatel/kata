# 02 Problem

> **Your task:** let the agent run a tool. Advertise `read`, execute it when the
> model asks, and send the result back so the model can answer.

**You edit:** `agent/provider_openai.go`, `agent/tool.go`, `agent/tools.go`,
`agent/cmd/agent/main.go`
**Given:** `agent/types.go` (the `Tool` interface and `ToolResult`)
**Time:** 60 to 90 minutes

---

## The contract

Four pieces, all in the `agent` package:

```go
// tool.go -- the registry
func NewRegistry(tools ...Tool) *Registry
func (r *Registry) Lookup(name string) (Tool, bool)
func (r *Registry) Defs() []ToolDef
func (r *Registry) Run(ctx context.Context, call ToolCall) ToolResult

// tools.go -- one tool
type ReadTool struct {
    Root     string // workspace root, absolute
    MaxBytes int    // 0 means 20000
}
```

`Tool` is already declared in `agent/types.go`. The program in
`agent/cmd/agent/main.go` currently sends no tools; wire the registry into it.

The provider must also learn one thing: when a message contains tool calls, or is
a tool result, that has to be serialised in the wire format.

## What the verifier does

It builds `./agent/cmd/agent`, creates a temporary workspace, starts a scripted
server that asks for a tool on the first turn, and runs your binary with that
directory as its working directory. Then it inspects:

1. the **first** request: does it advertise `read` with a `path` parameter;
2. the **second** request: is there a `tool` message whose content is the file,
   paired with the call id;
3. stdout, stderr and the exit code.

## The verifier will assert that

- `advertises-read`: the first request carries a tool named `read` whose
  parameters declare a `path` property.
- `reads-a-file`: the second request contains a `tool` message with the file
  contents and `tool_call_id` equal to the call id, and stdout is the model's
  follow-up answer.
- `folds-arguments`: arguments split across four fragments, with the id present
  only on the first, still resolve to the right file.
- `missing-file`: the tool result says `no such file`, and the program exits 0
  after printing the model's answer.
- `refuses-escape`: `../../etc/passwd` produces a refusal naming the escape, and
  no file contents appear in the result.
- `truncates-large-file`: a 50 KB file yields at most 25 KB of tool content and
  a marker containing `truncated`.
- `invalid-arguments`: malformed JSON produces a result saying the arguments are
  not valid JSON; the program exits 0.
- `text-only-single-request`: a plain reply costs exactly one request.

## Notes

- **Order matters.** Tool results go back in the same order as the calls, and
  each carries its call id. Providers validate this pairing.
- **The workspace root is the process working directory.** The checks start the
  binary inside a temp dir; do not resolve paths against the source tree.
- **Absolute paths are refused.** So are `..` segments that escape after
  resolution. Check the resolved path, not the string.
- **Deliberate omission:** no `write`, no `bash`, no parallel execution. Chapter
  03 adds the other tools; chapter 04 adds the loop.
- **Output contract:** the answer on stdout, diagnostics on stderr, exit 0 unless
  the provider request itself fails.

## Hints

<details><summary>Rung 1, the mechanism</summary>

A call and its result are two halves of one message pair. The id is what ties
them together, and the index is what ties the streamed fragments together.
</details>

<details><summary>Rung 2, the APIs</summary>

`json.Valid` before `json.Unmarshal` on model-supplied argument text.
`filepath.Abs` and `filepath.Rel` for the workspace check. `os.Stat` to tell a
missing file from a directory. `f.Read` with `limit+1` bytes to detect an
overflow.
</details>

<details><summary>Rung 3, what the code does</summary>

Fold tool-call fragments into a map keyed by `index`, appending
`function.arguments`; emit the calls sorted by index. Serialise an assistant
message with `tool_calls`, and a tool message with `role: "tool"`,
`tool_call_id` and `content`. `Registry.Run` returns an `IsError` result for an
unknown tool, invalid JSON, or a tool error. `ReadTool` resolves the path inside
`Root`, rejects escapes, reads `limit+1` bytes, and appends a truncation marker
when the extra byte exists.

`kata hint 02 <case-id>` prints these same rungs from the CLI.
</details>

## Done when

`kata submit 02` reports 8/8, and:

- a real endpoint reads a real file for you;
- you can explain why folding by id fails and folding by index does not;
- you know why a missing file must not exit the program.
