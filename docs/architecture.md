# Architecture

Lathe has two halves. One is the harness you build. The other is the tool that
checks it. They share one interface, so the checks never look inside your code.

```mermaid
flowchart LR
    subgraph yours["harness/ (you edit this)"]
        P["provider<br/>speaks HTTP"]
        T["tools<br/>read, write, bash"]
        L["loop<br/>calls the model"]
        S["state<br/>prompt, session, budget"]
    end

    subgraph grader["internal/ (do not edit)"]
        F["fake<br/>scripted model server"]
        G["grader<br/>cases and reports"]
        H["hints<br/>three rungs"]
    end

    C["chapters/NN/checks<br/>what done means"]
    CLI["lathe<br/>the runner"]

    L --> P
    P <--> F
    L --> T
    L --> S
    C --> G
    G --> F
    CLI --> C
    CLI --> G
    G --> H
```

The fake server is a real HTTP server. It binds a socket, parses your request,
checks your headers, and writes Server-Sent Events with real chunk boundaries.
Only the content of the reply is scripted. That is the whole design: replace the
model, keep the wire.

## Repository layout

| Path | Role |
|---|---|
| `harness/` | The harness. The only directory a learner edits. |
| `starter/` | The initial state of `harness/`. Used to grade chapter 01 and by `lathe reset`. |
| `solutions/chNN/` | Overlay of the files as they must be after chapter NN. |
| `chapters/NN-slug/` | `README.md`, `overview.md`, `problem.md`, `example.md`, `checks/`. |
| `internal/fake/` | Scripted OpenAI-compatible server. The far side of the seam. |
| `internal/grader/` | Case DSL, bounded execution, report, diffs. |
| `internal/hints/` | The three-rung hint ladder, keyed by chapter and case. |
| `cmd/lathe/` | The CLI: `submit`, `read`, `hint`, `list`, `progress`, `verify`, `reset`. |
| `docs/` | Architecture, chapter format, writing style. |

## A submit run

```mermaid
sequenceDiagram
    actor You
    participant CLI as lathe submit
    participant Go as go build
    participant Test as chapter checks
    participant Fake as fake server
    participant Rep as report JSON

    You->>CLI: lathe submit 03
    CLI->>Go: build ./...
    alt does not compile
        Go-->>CLI: errors
        CLI-->>You: not ready, with the first error and its cause
    else compiles
        CLI->>Test: go test ./chapters/03-slug/checks/
        Test->>Fake: scripted conversation over HTTP
        Fake-->>Test: SSE bytes, recorded requests
        Test->>Rep: write verdict
        CLI->>Rep: read verdict
        CLI-->>You: cases, diffs, hints, next chapter
    end
```

A build error is reported as "not ready". It never counts as a failed case,
because a learner who mistyped an import has not got the logic wrong.

## How chapters relate

Chapters are cumulative. Chapter NN's code is a subset of chapter NN+1's. That
property is what makes "your earlier chapters stay green" a guarantee instead of
a hope.

```mermaid
flowchart LR
    S["starter/<br/>all stubs"] --> A1["solutions/ch01"]
    A1 --> A2["solutions/ch02"]
    A2 --> A3["solutions/ch03"]
    A3 --> A4["solutions/ch04"]
    A4 --> D["..."]

    A1 -. "before state for ch02" .-> A2
    A2 -. "before state for ch03" .-> A3
```

The state before chapter NN is derived by applying `solutions/ch01` through
`solutions/chNN-1`. It is not stored, so it cannot drift.

`lathe verify` grades every chapter twice: once against its after state, which
must pass, and once against its before state, which must fail at least one case.
A chapter whose starter passes grades nothing. A chapter whose reference fails
cannot be completed. Both are caught offline.

## The grading pipeline

```mermaid
flowchart TD
    A["lathe submit NN"] --> B{harness/ compiles?}
    B -- no --> C["not ready<br/>file:line, message, cause"]
    B -- yes --> D["run chapter checks"]
    D --> E["for each case:<br/>bounded, panic-proof"]
    E --> F{passed?}
    F -- yes --> G["next case"]
    F -- no --> H["why, expected, actual,<br/>first differing line, hint"]
    G --> I["report JSON<br/>cases, durations, errors"]
    H --> I
    I --> J{all passed?}
    J -- yes --> K["record progress<br/>print next chapter"]
    J -- no --> L["list failed case ids<br/>point at hints"]
```

Every case runs with a timeout and a panic barrier. A harness that blocks costs
one case with its name attached, not the whole chapter.

## The seams

The harness is built around six interfaces. Chapters 05 to 11 add implementations
of them; they do not rewrite the loop.

| Interface | Method | Implemented by |
|---|---|---|
| `Provider` | `Chat(ctx, Request) (Message, Usage, error)` | chapter 01, extended in 02 |
| `Tool` | `Name`, `Description`, `Schema`, `Execute` | chapter 03 |
| `Contributor` | `Contribute(ctx) (string, error)`, appended to the system prompt | chapter 05 (context files), 09 (skills) |
| `Trimmer` | `Fit([]Message, budget) ([]Message, error)` | chapter 07 |
| `Steering` | `Drain() []Message`, polled after each tool batch | chapter 08 |
| `Recorder` | `Append(Event) error`, `Load(id) ([]Message, error)` | chapter 06 |
| `ToolSource` | `Tools(ctx) ([]Tool, error)`, merged into the registry | chapter 10 (subagent), 11 (MCP) |

```mermaid
flowchart TD
    LOOP["loop"]
    LOOP -->|"request"| PROV["Provider"]
    LOOP -->|"dispatch"| REG["tool registry"]
    REG --> TOOLS["Tool: read, write, bash"]
    REG --> SRC["ToolSource: subagent, MCP"]
    LOOP -->|"system prompt"| CONTRIB["Contributor"]
    LOOP -->|"before send"| TRIM["Trimmer"]
    LOOP -->|"after tool batch"| STEER["Steering"]
    LOOP -->|"every event"| REC["Recorder"]
```

A change that makes a later chapter edit `loop.go` is a design error, not a
scheduling problem. The seams exist so the course stays additive.

## Determinism

| Layer | Faked | Proves |
|---|---|---|
| Transport | HTTP server speaking SSE | your wire code is correct |
| Behaviour | scripted model decisions | your loop, tools, and context logic |
| Output | full request traces, per case | nothing drifted silently |

There is no layer that proves the agent is good. Quality needs a real model,
money, and judgement. Lathe grades correctness of the harness, and says so.

## Adding a chapter

1. `chapters/NN-slug/checks/chNN_test.go` with cases in the `grader.Suite` DSL.
2. `internal/hints/hints.go` entries, three rungs each. The completeness test
   fails if a case has no ladder or a ladder has no case.
3. `solutions/chNN/` with only the files this chapter changes.
4. `overview.md`, `problem.md`, `example.md`, `README.md`.
5. `lathe verify`. It must be clean before the chapter counts as written.

## Glossary

| Term | Meaning |
|---|---|
| Harness | The code around a model that turns text into actions: provider, tools, loop, state. |
| Seam | An interface you can replace. The provider seam is the one the grader uses. |
| Scripted model | A fixture that replays a fixed reply. Not a mock of your code. |
| Case | One graded assertion with an id, a reason, and three hint rungs. |
| Before state | The harness with chapters NN and later removed. |
| After state | The harness with chapters 01 through NN applied. |
