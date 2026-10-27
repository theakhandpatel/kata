# Chapter format

Every chapter is one directory with **one page to work from** and **one page to compare
against**, plus the checks and a title.

```
chapters/NN-slug/
  README.md      the task, how it works, the graded claims, three hints
  example.md     the finished implementation, annotated
  title          one line, the chapter's name, used by `kata list` and the site
  checks/        the Go test suite: the only thing that defines "done"
```

`reference/chNN/` holds the code, and only the files that chapter changes.

## Why two pages and not three

There were three for most of this course's life: `overview.md` for the concept,
`problem.md` for the task, and `example.md` for the answer. It was a mistake, and the reason
is worth recording.

A reader at the keyboard needs the task and the idea together. Splitting them means holding
two windows open, or scrolling between two files to answer one question. The merged page
also has a natural order that the split fought against: what you are building, then how it
works, then what is checked, then hints.

`example.md` stays separate, because it is spoilers. That distinction is real and the
others were not.

## `README.md`: the page to work from

Sections in this order. A chapter missing one of them is incomplete.

1. **What you are building.** A command and its output, in a shell block. If the reader
   cannot picture the result, nothing after this helps.
2. **The mechanism.** How it works, in the domain's terms. Diagrams and real bytes.
3. **The claims.** A table or list of every case id and what it asserts.
4. **What you need to build.** Signatures and types, and which files you may edit.
5. **Notes.** The traps. What is deliberately left out.
6. **If you get stuck.** Three collapsible hints: the shape of the problem, the tools you
   will need, and what the finished code does.
7. **Done when.** The number of cases, and one sentence of self-check.

## `example.md`: the page to compare against

1. **The shape**, before any code: the function split.
2. **Steps**, in the order you would think them through.
3. **The fixes.** Wrong version and right version, side by side, each labelled with the case
   id that catches it.
4. **What to compare against your own.** A checklist. Differences inside it are bugs,
   outside it is style.

## Rules

**Every claim names a case id.** A claim with no case is prose, and a case with no claim is
a test the reader was not told about. Both are bugs, and `cmd/kata/docs_test.go` fails on
both.

**No course commentary.** No "this chapter will teach you", no defence of the fictional
server, no warnings about what a later chapter does. State the mechanism and the check. A
reader who understands the mechanism will recognise the trap when it arrives.

**No forward references by number.** Chapters move. Say "the Steer and abort chapter", not
"chapter 08".

**The example is not a copy of the reference.** It is the reference taken apart in the order
the thinking happens, which is almost never the order the lines appear.

## Checks

Every case is one claim, with an id, a one-line title, a `Why` naming the mechanism, three
hints, and a function that returns an error or nil.

```go
{
	ID:    "missing-file",
	Title: "reports a missing file as a result, not a crash",
	Why:   "A failed tool is information the model can act on.",
	Hints: hints.Get("03", "missing-file"),
	Run: func(t *testing.T) error { ... },
}
```

Cases do not stop each other, each is bounded by a timeout, and a panic fails one case
rather than the chapter.

## Drivers

Almost every case is black box: build the binary, run it, read stdout, the exit code, and
the requests it sent. That is the only way to prove the program works when invoked, and it
leaves the internal design free.

## Adding a chapter

1. Create `chapters/NN-slug/` with `README.md`, `example.md`, `title`, `checks/chNN_test.go`.
2. Add an `"NN"` entry to `internal/hints/hints.go`, three hints per case.
3. Add `reference/chNN/` with only the files the chapter changes.
4. `kata verify`. It must be clean: the reference passes every case and the state before the
   chapter fails at least one.

Nothing else needs editing. The chapter list, the site sidebar, the part grouping and the
grading order all come from the directory names.
