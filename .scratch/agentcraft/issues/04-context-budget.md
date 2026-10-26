# Context budget

Type: grilling
Status: open
Blocked by: (nothing)

## Question

How does chapter 07 decide what to drop, and what is actually asserted?

Options: rough char/4 token estimate vs provider-reported usage; drop oldest
turns vs summarize; protect the last N turns vs protect tool pairs. The gradable
invariants are the interesting part: never exceed budget, never orphan a
tool_result, never drop the system prompt. Which invariants are required, and is
compaction (an inserted summary entry) in scope or pushed to the fog?
