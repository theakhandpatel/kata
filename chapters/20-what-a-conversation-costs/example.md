# 20 Example

*The finished implementation, taken apart. Open this after `miyagi submit 20` passes.*

The file it describes is `reference/ch20/agent/usage.go`, plus four columns in
`reference/ch20/agent/store.go`.

---

## Storing the cost beside the reply

```go
costs := make([]agent.MessageCost, len(newMessages))
for i := len(newMessages) - 1; i >= 0; i-- {
	if newMessages[i].Role == agent.RoleAssistant {
		costs[i] = agent.MessageCost{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			Model:            model,
		}
		break
	}
}
```

Walking backwards to the last assistant message, because that is the one the final request
produced. `Result.Usage` is a total across every turn of the run, so it belongs on the
final reply rather than being divided among them.

Dividing would look more precise and be a guess. The provider reports a count per request,
and a run makes several; only the last one is still in hand when the run ends. Earlier
turns are recorded as zero, and the report is honest about what it knows.

## Money in micros

```go
func (p Price) Cost(c MessageCost) int64 {
	uncached := c.PromptTokens - c.CacheReadTokens
	if uncached < 0 {
		uncached = 0
	}
	micros := int64(0)
	micros += int64(uncached) * p.InputMicros / 1000
	micros += int64(c.CacheReadTokens) * p.CacheMicros / 1000
	micros += int64(c.CompletionTokens) * p.OutputMicros / 1000
	return micros
}
```

Three terms, integer arithmetic throughout. `float64` would accumulate error across
thousands of messages, and a report that disagrees with the invoice by a cent teaches people
to ignore the report.

The `uncached < 0` guard exists because a provider that reports a cache figure larger than
the prompt count is buggy, and the resulting negative charge is worse than a zero.

```go
func FormatMicros(micros int64) string {
	return fmt.Sprintf("%s$%d.%06d", sign, micros/1_000_000, micros%1_000_000)
}
```

Converted at the edge, once, for display.

## The grouped query

```go
SELECT m.session_id, COALESCE(s.title, ''), s.updated,
       COALESCE(SUM(m.prompt_tokens), 0),
       COALESCE(SUM(m.completion_tokens), 0),
       COALESCE(SUM(m.cache_read_tokens), 0),
       COUNT(*)
FROM messages m
LEFT JOIN sessions s ON s.id = m.session_id
WHERE m.role = 'assistant'
GROUP BY m.session_id
ORDER BY SUM(m.prompt_tokens) + SUM(m.completion_tokens) DESC
```

`WHERE m.role = 'assistant'` is what makes the totals right. Counting every row would double
the reply tokens, because a tool call and its result both carry the prompt count in some
providers' accounting, and only the reply is priced.

`LEFT JOIN` rather than `JOIN`: a session with messages but no row in `sessions` should still
appear, with an empty title, instead of vanishing from the report.

`COALESCE` because `SUM` over zero rows returns `NULL`, and scanning `NULL` into an `int`
fails.

`ORDER BY` the total, so the expensive conversation is the first line. That is the question
the report exists to answer.

## Two shapes for one function

```go
func printUsage(total agent.CostReport, per []agent.SessionCost) {
	if total.Messages == 0 {
		fmt.Println("no usage recorded yet")
		return
	}
	if per == nil {
		// one session
	}
	// every session
}
```

`per == nil` distinguishes "report one session" from "report all". The alternative is two
functions and duplicated formatting.

The empty check comes first for both shapes. A table of zeros is technically accurate and
practically useless.

## Read-only flags before the prompt check

```go
if *showUsage {
	... open store, print, return 0
}
if *listSess {
	... same
}
if *prompt == "" {
	return 2, errors.New(`-p is required`)
}
```

Both read-only commands are handled before anything that needs a prompt or a provider.
Asking what you spent must not require a model, a deadline, or a network.

## The fixes that matter

**1. Reporting no title** (`usage-across-sessions`)

```diff
- SELECT session_id, SUM(...) FROM messages
+ SELECT m.session_id, s.title, ... LEFT JOIN sessions s
```

**2. A zero table on an empty store** (`empty-store-reports-nothing`)

```diff
- fmt.Printf("%-12s ...", "MESSAGES", ...)
+ if total.Messages == 0 { fmt.Println("no usage recorded yet"); return }
```

**3. Pricing every row** (`usage-per-session`)

```diff
- FROM messages WHERE session_id = ?
+ FROM messages WHERE session_id = ? AND role = 'assistant'
```

**4. Floats for money** (`cost-is-computed`)

```diff
- cost += float64(tokens) * rate / 1000
+ micros += int64(tokens) * rateMicros / 1000
```

## What to compare against your own

- [ ] counts written on the assistant row, not summed in memory only
- [ ] the session's totals include earlier runs
- [ ] the report joins session titles
- [ ] only assistant rows are priced
- [ ] money arithmetic is integer
- [ ] an empty store prints a sentence
- [ ] `-usage` works with no provider

Everything outside that list is style.
