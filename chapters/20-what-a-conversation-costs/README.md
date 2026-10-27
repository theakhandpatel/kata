# 20 What a conversation costs

**Part VII · Store** · Difficulty: medium · 45 to 60 minutes · 7 graded cases

## What you are building

A report of what you have spent, and on what.

```sh
$ ./agent -usage
SESSION                TITLE                          MSGS     PROMPT      REPLY COST
20261027-091334-928e   why does the deploy fail           8      18400       2100 $0.004020
20261027-091334-5fb9   add the retry flag                 4       6100        700 $0.001335

total                                                     12      24500       2800 $0.005355
```

The counts come from the provider. The money comes from a rate table in your code. Both
are stored with the conversation, so the report survives the process that produced it.

You may edit `agent/usage.go`, `agent/store.go` and `agent/cmd/agent/main.go`.

## Where the numbers come from

The provider reports usage once, on the final message of the stream:

```json
{"prompt_tokens": 18400, "completion_tokens": 2100, "total_tokens": 20500}
```

The loop sums them across turns and returns the total in `Result.Usage`. That number is in
memory and disappears when the process exits, which is why the previous chapter's store is
where it belongs.

## Two records, not one

| Value | Where it lives | Why |
|---|---|---|
| token counts | with the message | a fact about that call |
| price per thousand | a table in your code | a policy that changes |

Keeping them apart means a price change does not require rewriting history, and a
conversation is not re-priced every time you look at it.

## Money in integers

```go
micros += int64(tokens) * rateMicros / 1000
```

`float64` accumulates error, and a cost report that is off by a cent is a report nobody
trusts. Micros are a millionth of a dollar: enough precision for a single call, exact in
addition, and formatted with six decimals at the edge.

```
  1000 prompt tokens at 0.15 per 1000   →  150 micros
  1000 reply tokens at 0.60 per 1000    →  600 micros
                                           750 micros   $0.000750
```

## Cached tokens

Some providers bill a cached prompt prefix at a tenth of the input rate, and report how
much was served from cache. When that figure is present, the arithmetic splits:

```go
uncached := c.PromptTokens - c.CacheReadTokens
micros += int64(uncached) * p.InputMicros / 1000
micros += int64(c.CacheReadTokens) * p.CacheMicros / 1000
```

When it is absent, `CacheReadTokens` is zero and this reduces to the plain input rate.

## The columns

```sql
prompt_tokens     INTEGER NOT NULL DEFAULT 0,
completion_tokens INTEGER NOT NULL DEFAULT 0,
cache_read_tokens INTEGER NOT NULL DEFAULT 0,
model             TEXT NOT NULL DEFAULT '',
```

Present on every message row and zero for prompts and tool results, because only the reply
costs anything. A separate `costs` table would need a foreign key and a join; four columns
on the row that produced them are simpler and cannot drift.

The `model` column matters: a session can switch models mid-conversation, and the rate
depends on which one answered.

## The report

Two queries. One for a single session, one grouped across all of them, ordered by what
they spent so the expensive conversation is at the top. Both join `sessions` for the
title, because a list of ids is not a report.

An empty store prints one sentence rather than a table of zeros. Zero rows is not an error,
and a table claiming you spent nothing across nothing is worse than saying so.

## What you need to build

```go
type MessageCost struct {
    PromptTokens     int
    CompletionTokens int
    CacheReadTokens  int
    Model            string
}

type Price struct {
    Model        string
    InputMicros  int64
    OutputMicros int64
    CacheMicros  int64
}

type CostReport struct {
    Sessions, Messages                     int
    PromptTokens, CompletionTokens, CacheReadTokens int
    Micros                                 int64
}

func PriceFor(model string) Price
func (p Price) Cost(c MessageCost) int64
func FormatMicros(micros int64) string
func (s *Store) AppendWithCost(ctx context.Context, id string, msgs []Message, costs []MessageCost) error
func (s *Store) UsageFor(ctx context.Context, id string) (CostReport, error)
func (s *Store) UsageAll(ctx context.Context) (CostReport, []SessionCost, error)
```

One flag:

```sh
agent -usage                 every session, plus a total
agent -usage -resume <id>    one session
```

## How you are checked

The verifier builds your program, runs it against a server that reports known token counts,
then reads the report and the store.

- `usage-is-recorded`: the counts the provider sent appear in the report.
- `usage-per-session`: `-usage -resume <id>` reports that session's totals.
- `usage-across-sessions`: two conversations both appear by title, and the prompt total is
  their sum.
- `cost-is-computed`: 1000 prompt and 1000 reply tokens at the default rates produce
  `$0.000750`.
- `usage-survives-a-resume`: two runs in one session report both runs' tokens.
- `usage-needs-no-provider`: `-usage` works with a dead endpoint.
- `empty-store-reports-nothing`: one sentence, exit 0.

## Notes

`-usage` is read-only. Handle it before the prompt check, the same way `-sessions` is, so
asking what you spent does not require a provider or a prompt.

Price the reply, not the prompt. The prompt's tokens are already counted on the reply that
consumed them.

## If you get stuck

<details><summary>First hint: the shape of the problem</summary>

Either the counts were never written, the query sums the wrong rows, or the money is
computed with the wrong units.
</details>

<details><summary>Second hint: what you will need</summary>

Four columns on the message row, `SUM` with `GROUP BY session_id`, `LEFT JOIN sessions` for
the title, and integer arithmetic in micros.
</details>

<details><summary>Third hint: the finished code</summary>

`AppendWithCost` writes the four cost columns alongside each message. `UsageFor` sums the
assistant rows for one session. `UsageAll` groups by session, joins the title, and orders by
the token total. `Price.Cost` splits cached from uncached prompt tokens, then adds
completion at the output rate, all in micros. `-usage` opens the store, prints, and returns
before any provider is built.

`kata hint 20 <case-id>` prints the same three hints.
</details>

## Done when

`kata submit 20` reports 7/7, and `./agent -usage` tells you which conversation is costing
you the most.

[example.md](example.md) has the finished code, after you pass.

---

Next: **21 Compaction**.
