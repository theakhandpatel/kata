# Loop grammar

Type: prototype
Status: open
Blocked by: 01

## Question

What does the chapter-04 event-log grammar look like in concrete terms?

Take the graded event subset from `01-pi-event-taxonomy` and write the actual
grammar a learner's loop must satisfy, e.g.:

    agent_start turn_start (text_delta* | tool_use tool_result)* turn_end agent_end

Then prototype the failure message for the most likely violation (a tool_use
with no tool_result) and check that it reads as a sentence a learner can act on.
This decides whether chapter 04 is a sequence matcher or a set of invariants.
