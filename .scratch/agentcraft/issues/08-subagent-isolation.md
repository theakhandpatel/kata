# Subagent isolation

Type: grilling
Status: open
Blocked by: (nothing)

## Question

What does "isolated context" mean for chapter 10, testably?

A child loop must not see the parent's history but must return a result the
parent can place. The decision: is isolation asserted by inspecting the child's
requests (empty history but same system prompt), by the child's tool set being a
subset, by a budget, or by all three? Also: does the parent's tool_use/tool_result
pairing survive a failed child?
