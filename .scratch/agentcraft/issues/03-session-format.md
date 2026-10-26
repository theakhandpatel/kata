# Session format

Type: grilling
Status: open
Blocked by: (nothing)

## Question

Session file format: JSONL tree or flat append-only log?

pi uses a tree (each entry points at a parent; branching, forking, compaction
summaries). A flat log is far simpler and still teaches replay, resume, and
idempotence. Which one is worth the chapter, given the course explicitly skips
the TUI that motivates tree navigation?

Affects chapters 06 and 07 directly.
