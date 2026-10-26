# CodeCrafters stage anatomy

Source: 14 saved stage pages from *Build your own Claude Code* (Go track),
captured 2026-10-27. 13 stages + local setup. This is the format we copy, with
the parts that only make sense for a hosted product removed.

## Page skeleton

```
┌ sidebar ──────────┬ main ─────────────────────────────────────────────┐
│ Introduction      │  #wd2  Pass arguments to a skill    [Pending]     │
│ Local Setup       │  Instructions | Code Examples | Forum             │
│ Communicate w/ LLM│  ─────────────────────────────────────────────    │
│ Advertise read    │  [Previous steps incomplete]                      │
│ …                 │  Your Task          ← one sentence                │
│ ▸ Skills (group)  │  <concept explainer, tables, worked examples>     │
│   Advertise skills│  Tests              ← literal expected transcript │
│   Invoke by name  │  The tester will verify that:                     │
│   Pass arguments  │    - bullet                                        │
│   Stack skills    │  Notes              ← edge cases + scope limits   │
│   Run script      │  How to pass this stage  (1 write, 2 submit)      │
│   Model chooses   │  Hints (community, filtered by language)          │
│   Skill/subagent  │                                                   │
└───────────────────┴───────────────────────────────────────────────────┘
```

Header carries difficulty (`Very easy` / `Easy` / `Medium` / `Hard`) and status
(`Pending` / `In-progress` / `Complete`). Stages are grouped into sections;
`Skills` is a section header with six children.

## The six sections, and what each is actually for

### 1. Your Task — one sentence

> In this stage, you'll add support for passing arguments to a skill.

Verb is always "add support for". It names the *capability*, never the
mechanism. Never "implement `splitArgs`".

### 2. Concept — teach the model of the domain, not the code

Stage `Pass arguments to a skill` opens with the *problem*, then a table of the
two placeholder kinds:

| Placeholder | Replaced with |
| --- | --- |
| `$ARGUMENTS` | Everything after the skill name, as one string |
| `$ARGUMENTS[0]`, `$ARGUMENTS[1]`, … | A single argument, by zero-based position |
| `$0`, `$1`, `$2`, … | Shorthand for sending single arguments |

Then a **worked example with a literal program invocation**:

```
$ ./your_program.sh -p "/apple staging eu-west"
```

> `$ARGUMENTS` becomes `staging eu-west`, `$0` becomes `staging`, and `$1`
> becomes `eu-west`.

So a body of `Deploy to $0 in region $1. Full request was: $ARGUMENTS` is sent as
`Deploy to staging in region eu-west. Full request was: staging eu-west`.

For the agent loop stage, the concept is **pseudocode**, then a section-per-line
walkthrough:

```
messages = [{role:"user", content: prompt}]
loop:
  response = call_api(messages)
  append response message to messages
  if response has no tool_calls: print response.content; exit
  for each tool_call in response.tool_calls:
    result = execute_tool(tool_call)
    append {role:"tool", tool_call_id: tool_call.id, content: result} to messages
```

That is the single most effective teaching device on the page: **the algorithm in
eight lines, before any Go.**

### 3. Tests — a literal transcript, then the claims

> The tester will create two skills and run your program twice.
>
> ```
> $ ./your_program.sh -p "/apple 4127"
> 4127
> ```
>
> The tester will verify that:
> - The `$ARGUMENTS` substitution reproduces the full argument string
> - The positional substitutions resolve to the right words, in the order the body asks for

Two moves to steal:

- **Show the exact invocation and the exact expected output.** Randomized
  values are described ("a random number"), so the learner can't hardcode.
- **"The tester will verify that:"** is the graded claim, in prose, *before*
  you start. Not a list of assertions — a list of promises.

And in the subagent stage, the strongest one:

> The tester will watch the requests your program sends, and verify that:
> - One of them carries the skill's instructions **without** the question that triggered them
> - The subagent's answer appears in your program's output

They grade the **request trace**, not the final string. That is exactly the
`internal/fake` seam this course uses, and it validates the design.

### 4. Notes — edge cases and explicit scope limits

> Invoking a skill with no arguments should still work. Replace the placeholders
> with empty strings.

> Claude Code supports several more substitutions, including named arguments and
> session variables. We won't deal with them in this extension.

> Do not print raw file contents when executing the Read tool. This differs from
> the behavior in the "Execute the read tool" stage.

> You can print intermediate debugging information to stderr. Print to stdout
> only when the final result is ready to be printed.

Three note kinds, all worth copying:

- **Edge case you must handle** (empty arguments).
- **Deliberate omission with a link to the real thing** — this is how they keep
  scope small without pretending the omitted feature doesn't exist.
- **Output contract** (stdout vs stderr, exit code 0).

### 5. How to pass — exactly two steps

1. Write code to pass this stage.
2. Submit: `codecrafters submit`.

No options, no alternatives.

### 6. Hints — community comments filtered by language

Not curated hints. Threaded comments with markdown. Upvote-sorted. Useful at
scale, useless at 1 learner. **We replace this with a curated three-rung
ladder** (`lathe hint`), which is strictly better for a solo learner and strictly
worse for a cohort.

## The 13 stages, so we can see the escalation curve

| # | Stage | Difficulty | Stated time | New concept |
|---|---|---|---|---|
| 1 | Communicate with the LLM | Very easy | <5 min | OpenAI-compatible API; provided solution, just uncomment |
| 2 | Advertise the read tool | Easy | 5–10 min | tool *schema* in the request |
| 3 | Execute the read tool | Easy | 5–10 min | tool result message |
| 4 | Implement the agent loop | Medium | 30–60 min | loop until no tool calls |
| 5 | Implement the write tool | Easy | 5–10 min | second tool, same seam |
| 6 | Implement the bash tool | Easy | 5–10 min | exec + stdout capture |
| — | **Skills** (section) | | | |
| 7 | Advertise skills to the LLM | Medium | 30–60 min | discover on disk, list in prompt |
| 8 | Invoke a skill by name | Easy | 5–10 min | slash-command routing |
| 9 | Pass arguments to a skill | Easy | 5–10 min | placeholder substitution |
| 10 | Stack multiple skills | Medium | 30–60 min | multiple expansion |
| 11 | Run a script bundled with a skill | Medium | 30–60 min | scripts as tools |
| 12 | Let the model choose a skill | Medium | 30–60 min | catalog in system prompt |
| 13 | Run a skill in a subagent | Hard | >1 hour | isolated child context |

Shape of the curve: **one conceptual leap per 2–3 mechanical stages.** Leaps are
stages 1, 4, 7, 13. Everything between is "same seam, new payload". Our chapters
are the leaps; our cases are the mechanical stages inside each.

## What we copy, and what we drop

| Copy | Drop | Replace with |
|---|---|---|
| "Your Task" one-liner | Leaderboard, points, rank | — |
| Pseudocode before code | Git-push submit loop | `lathe submit` locally |
| Literal transcripts | Hosted tester/Docker | `internal/fake` + `internal/grader` |
| "The tester will verify that:" | Community hints | Curated 3-rung ladder |
| Notes with explicit omissions | "Previous steps incomplete" gate | Regression: prior chapters stay green |
| Difficulty + time estimate | Provided solution to uncomment | — (we keep a reference impl, but not handed to you) |
| Section grouping | Account/login | — |

## Two corrections to our own design, from reading these

1. **They grade the request trace, publicly stated.** Our `checks/` already do
   this, but our chapter pages don't *say so*. Every chapter page must have a
   "the verifier will inspect your requests and assert:" section.
2. **They name the output contract.** stdout vs stderr, exit code. We never
   specified it. It belongs in chapter 01's Notes, because a learner piping debug
   output to stdout will fail later chapters for a reason that looks unrelated.
