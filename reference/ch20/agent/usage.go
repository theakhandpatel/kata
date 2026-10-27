package agent

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MessageCost is what one model call cost, recorded beside the assistant message that
// carried the reply.
//
// The provider reports the counts; the caller decides what they are worth. Keeping the
// two apart means a price change does not require re-reading old conversations.
type MessageCost struct {
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	Model            string
}

// Price is the rate for one model, in micros per thousand tokens.
//
// Integer micros rather than floats: money arithmetic in float64 drifts, and a report
// that is off by a cent is a report nobody trusts.
type Price struct {
	Model        string
	InputMicros  int64 // per 1000 prompt tokens
	OutputMicros int64 // per 1000 completion tokens
	CacheMicros  int64 // per 1000 cached prompt tokens, when the provider reports them
}

// DefaultPrices is a small table, not a live feed. The rates are illustrative and the
// command prints them alongside the total so nobody mistakes them for an invoice.
var DefaultPrices = []Price{
	{Model: "default", InputMicros: 150, OutputMicros: 600, CacheMicros: 15},
}

// PriceFor finds the rate for a model, falling back to the default entry.
func PriceFor(model string) Price {
	for _, p := range DefaultPrices {
		if p.Model == model {
			return p
		}
	}
	return DefaultPrices[0]
}

// Cost computes the money for one message, in micros.
//
// Cached prompt tokens are billed at the cache rate and the rest at the input rate.
// When a provider reports no cache figure, CacheReadTokens is zero and this reduces to
// the plain input rate.
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

// FormatMicros renders micros as a currency amount with six decimals.
func FormatMicros(micros int64) string {
	sign := ""
	if micros < 0 {
		sign = "-"
		micros = -micros
	}
	return fmt.Sprintf("%s$%d.%06d", sign, micros/1_000_000, micros%1_000_000)
}

// CostReport is the usage of one session, or of all of them.
type CostReport struct {
	Sessions         int
	Messages         int
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	Micros           int64
}

// Add accumulates one message.
func (r *CostReport) Add(c MessageCost) {
	r.Messages++
	r.PromptTokens += c.PromptTokens
	r.CompletionTokens += c.CompletionTokens
	r.CacheReadTokens += c.CacheReadTokens
	r.Micros += PriceFor(c.Model).Cost(c)
}

// Merge accumulates another report.
func (r *CostReport) Merge(other CostReport) {
	r.Sessions += other.Sessions
	r.Messages += other.Messages
	r.PromptTokens += other.PromptTokens
	r.CompletionTokens += other.CompletionTokens
	r.CacheReadTokens += other.CacheReadTokens
	r.Micros += other.Micros
}

// UsageFor sums the cost of one session.
func (s *Store) UsageFor(ctx context.Context, id string) (CostReport, error) {
	var report CostReport
	report.Sessions = 1

	rows, err := s.db.QueryContext(ctx, `
		SELECT prompt_tokens, completion_tokens, cache_read_tokens, model
		FROM messages WHERE session_id = ? AND role = 'assistant'`, id)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	for rows.Next() {
		var c MessageCost
		if err := rows.Scan(&c.PromptTokens, &c.CompletionTokens, &c.CacheReadTokens, &c.Model); err != nil {
			return report, err
		}
		report.Add(c)
	}
	return report, rows.Err()
}

// UsageAll sums every session, and reports each one's total alongside.
func (s *Store) UsageAll(ctx context.Context) (CostReport, []SessionCost, error) {
	var total CostReport

	rows, err := s.db.QueryContext(ctx, `
		SELECT m.session_id, COALESCE(s.title, ''), s.updated,
		       COALESCE(SUM(m.prompt_tokens), 0),
		       COALESCE(SUM(m.completion_tokens), 0),
		       COALESCE(SUM(m.cache_read_tokens), 0),
		       COUNT(*)
		FROM messages m
		LEFT JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'assistant'
		GROUP BY m.session_id
		ORDER BY SUM(m.prompt_tokens) + SUM(m.completion_tokens) DESC`)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()

	var perSession []SessionCost
	for rows.Next() {
		var (
			sc SessionCost
			c  MessageCost
		)
		var updated string
		if err := rows.Scan(&sc.ID, &sc.Title, &updated, &c.PromptTokens, &c.CompletionTokens,
			&c.CacheReadTokens, &sc.Messages); err != nil {
			return total, nil, err
		}
		sc.Updated, _ = time.Parse(stampLayout, updated)
		sc.PromptTokens = c.PromptTokens
		sc.CompletionTokens = c.CompletionTokens
		sc.CacheReadTokens = c.CacheReadTokens
		sc.Micros = PriceFor("default").Cost(c)
		perSession = append(perSession, sc)
		total.Merge(CostReport{
			Sessions:         1,
			Messages:         sc.Messages,
			PromptTokens:     sc.PromptTokens,
			CompletionTokens: sc.CompletionTokens,
			CacheReadTokens:  sc.CacheReadTokens,
			Micros:           sc.Micros,
		})
	}
	return total, perSession, rows.Err()
}

// SessionCost is one row of the usage report.
type SessionCost struct {
	ID               string
	Title            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	Messages         int
	Micros           int64
	Updated          time.Time
}

// ErrNoUsage means there is nothing recorded yet.
var ErrNoUsage = sql.ErrNoRows
