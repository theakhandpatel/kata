# Design audit: interfaces and stub-ability

Two questions. Is the code shaped so a piece can be replaced by a stub, and does that shape
teach a practice worth learning? And is the grading sequential?

## Sequential grading

Yes, and it is the default now.

```
$ kata submit          # grades from the frontier, stops at the first failure
$ kata submit 07       # grades chapter 07 only
$ kata submit all      # grades everything, does not stop
```

`kata list` shows where you are:

```
  I. Wire
    [x] 01  Talk to the model            9/9 cases passed
    [!] 02  Tool calling                 failed: 2/7 cases passed
    [-] 03  Running a tool               blocked
```

`x` passed, `!` failed, `-` blocked because something before it has not passed, blank ready.
The first ready chapter is the frontier, and that is the only one worth grading.

**The suites themselves do not depend on each other.** No check file imports `kata/agent`;
every case builds the binary and runs it. So a broken chapter 06 does not stop chapter 07 from
compiling or from being graded, and `kata submit all` really does grade everything. The ladder
is a choice about feedback, not a technical constraint. It exists because a later chapter's
failures are mostly noise when an earlier one is red.

Verified by hand: with only chapter 01's provider in place, `kata submit` passed 01, advanced
to 02, failed 02, and stopped without touching 03. A second run resumed at 02.

## Interfaces

Seven interfaces, and they are not there for decoration. The test of a real seam is whether
something else implements it.

| Interface | Where | Implementations | Stub-able |
|---|---|---|---|
| `Provider` | `agent/types.go` | `OpenAICompat`, plus `budgetedProvider` wrapping it | yes, and it is how the checks work |
| `StreamingProvider` | `agent/loop.go` | the same, plus the budget decorator | yes |
| `Tool` | `agent/types.go` | **8**: `ReadTool`, `WriteTool`, `BashTool`, `StubTool`, `TaskTool`, `ReadResultTool`, `ToolLoader`, `mcpTool` (plus `spillTool`, which wraps another `Tool`) | yes, and `StubTool` exists purely to prove it |
| `Contributor` | `agent/prompt.go` | `AgentsFile`, `Catalog`, `ScopedCatalog` | yes |
| `Recorder` | `agent/session.go` | `JSONLSession` | yes |

Two more that are functions rather than interfaces, deliberately: `ToolSource` and
`Trimmer` were in an earlier design and were removed because one implementation each made them
noise. A one-implementation interface is a typedef with extra steps.

### What the shape teaches

- **`Provider` has one method.** Streaming was added by declaring a wider interface
  (`StreamingProvider`) beside it rather than changing it, so a provider that cannot stream is
  still a provider. That is interface segregation shown, not asserted.
- **Decorators, not edits.** The budget is a `Provider` that wraps a `Provider`. Retries will
  be the same shape. Later chapters add a file rather than editing the loop.
- **`StubTool` is a teaching device.** Chapter 02 needs a registered tool it cannot fail to
  run, so the reference ships one that returns `"stub"`. It is honest about being a placeholder
  and it is replaced in chapter 03.
- **Errors are values at the boundary, text inside.** `Tool.Execute` returns `(string, error)`;
  the registry converts an error into an `isError` result. That distinction is what lets a
  later chapter tell "the tool failed" from "the tool returned a failure message".

### Where the shape is weak

**`Recorder` has one implementation.** It is used as an interface in `main` and by the loop's
absence of knowledge, but nothing else implements it. If a second consumer never arrives, it is
a candidate for removal in favour of the concrete type.

**`Provider.Request` carries twelve fields.** A narrower request type would be more stub-friendly,
but it would also mean a conversion at every call site. The chosen trade is documented at the
type.

**The checks do not exercise the interfaces directly.** They grade the program, which is the
right call for proving behaviour, but it means a learner never sees an interface mocked for
them. That is intentional: the fake server is the only stub the course ships, and it stands in
for the model rather than for the learner's code.
