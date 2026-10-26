# Chapter format

Every chapter is one directory with **three reader-facing documents**, one
grader, and one reference solution. The three documents answer three different
questions and are read at three different times; merging any two of them produces
a page that is simultaneously too long to work from and too thin to learn from.

```
chapters/NN-slug/
  README.md      one-screen index: goal, time, difficulty, edit list, doc links
  overview.md    how the mechanism works, in general. Concept + diagrams.
  problem.md     what you must build, what exactly is graded, and the hints.
  example.md     a complete, annotated implementation. Implementation-heavy.
  checks/        Go test suite — the only thing that defines "done"
solutions/chNN/  reference implementation, cumulative, separate Go module
```

## The three documents

| | `overview.md` | `problem.md` | `example.md` |
|---|---|---|---|
| Question | *how does this work?* | *what must I do?* | *what does a full one look like?* |
| Read when | before you start | at the keyboard | after a real attempt |
| Shape | prose, diagrams, worked byte-level examples | tables, claims, hints | code, diffs, annotations |
| Length | long, discursive | short, scannable | as long as the code |
| Spoilers | none — concept only | none | total, by design |
| Ends with | self-check questions | done-when | comparison checklist |

The rule that keeps them honest: **`overview.md` may not contain a working
implementation, and `example.md` may not contain new concepts.** If an idea only
appears in the example, it belongs in the overview first.

### `overview.md` — the general doc

Free-form, but must contain:

1. **Why it exists** — the mechanism in the domain's terms, not Go's.
2. **A diagram** — `mermaid` for GitHub/HTML, plus a short ASCII version when the
   diagram is load-bearing, because the terminal cannot render mermaid.
3. **Real bytes** — an actual SSE chunk, an actual JSON body, an actual file.
   Not "the model returns chunks"; the chunk.
4. **A failure map** — the ways this goes wrong, drawn as a tree, each branch
   labelled with the case id that catches it. This is the link between teaching
   and grading.
5. **Where the real implementations go further** — pi/crush/PiG file paths, and
   what they add that we skipped.
6. **Self-check** — 3–5 questions answerable only if the concept landed, each
   pointing at the section that answers it.

### `problem.md` — the contract

Fixed sections, in this order. No freestyle.

1. **Your task** — one sentence, naming a capability, never a function.
2. **The contract** — the signature, the wire shape, the file format. Tables.
3. **What the verifier does** — what it runs, and *that it inspects requests*,
   not only output.
4. **The verifier will assert that** — one bullet per graded case, phrased as a
   promise, named by case id. Must match `checks/` exactly; if they diverge the
   checks are truth and this page is a bug.
5. **Notes** — edge cases you must handle; deliberate omissions with a link to
   the real implementation; the output contract (stdout vs stderr, exit codes).
6. **Hints** — three rungs, spoiler-collapsed, identical to `internal/hints`.
7. **Done when** — `lathe submit NN` green, plus one self-check sentence.

### `example.md` — the implementation doc

The full annotated answer, structured as the *thinking order*, not the file
order:

1. **The shape** — the function split, before any code.
2. **Steps** — one section per decision, each with the code and *why*.
3. **The diffs** — wrong vs right for each trap, labelled with the case id.
4. **The complete file** — or a pointer to `solutions/chNN/`.
5. **Production delta** — a table of what pi/crush add and why we skipped it.
6. **Comparison checklist** — differences that are bugs vs differences that are
   style.

## Indexing

CodeCrafters indexes stages globally with sections as headers. We do not: **a
chapter is the unit**, `NN` is its identity, and cases inside a chapter are
addressable only by case id (for hints). Inserting a chapter never renumbers a
learner's progress, and the arc stays editable while it is being written.

## Grouping into parts

| Part | Chapters | The leap |
|---|---|---|
| I. Wire | 01–02 | talk to a model, survive its output |
| II. Act | 03–04 | tools, then the loop that drives them |
| III. Remember | 05–07 | prompt, sessions, budget |
| IV. Extend | 08–09 | steering, skills |
| V. Compose | 10–11 | subagents, MCP |

Part boundaries are where the mental model changes, and where a learner can stop
and still have something useful.
