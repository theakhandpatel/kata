package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a conversation database.
//
// A file per conversation works until there are forty of them and you cannot remember
// which name you used. A database answers the question a person actually has: what did I
// talk about, and which one was it.
type Store struct {
	db *sql.DB
}

// Session is one conversation's identity and counters.
type Session struct {
	ID       string
	Title    string
	Created  time.Time
	Updated  time.Time
	Messages int
}

// stampLayout keeps microseconds. Two runs a second apart must order correctly, and two
// runs inside the same second must still have a definite order.
const stampLayout = "2006-01-02T15:04:05.000000Z07:00"

func stamp(t time.Time) string { return t.UTC().Format(stampLayout) }

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id       TEXT PRIMARY KEY,
	title    TEXT NOT NULL,
	created  TEXT NOT NULL,
	updated  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	ordinal      INTEGER NOT NULL,
	role         TEXT NOT NULL,
	content      TEXT NOT NULL,
	tool_calls   TEXT NOT NULL DEFAULT '',
	tool_call_id TEXT NOT NULL DEFAULT '',
	name         TEXT NOT NULL DEFAULT '',
	prompt_tokens     INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	cache_read_tokens INTEGER NOT NULL DEFAULT 0,
	model             TEXT NOT NULL DEFAULT '',
	UNIQUE(session_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated DESC);
`

// OpenStore opens or creates the database at path.
func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Create starts a session and returns its id.
//
// The id is derived from the clock and a short random suffix rather than being a
// counter, so two processes starting at the same moment do not collide.
func (s *Store) Create(ctx context.Context, title string) (string, error) {
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%04x", now.Format("20060102-150405"), now.UnixNano()&0xffff)

	if strings.TrimSpace(title) == "" {
		title = "untitled"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, created, updated) VALUES (?, ?, ?, ?)`,
		id, title, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return id, nil
}

// Latest returns the most recently updated session, or "" when there is none.
//
// This is what `--continue` needs: the session a person was last using, without having
// to remember an id.
func (s *Store) Latest(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM sessions ORDER BY updated DESC LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// List returns every session, most recently used first.
func (s *Store) List(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.created, s.updated,
		       (SELECT COUNT(*) FROM messages m WHERE m.session_id = s.id)
		FROM sessions s
		ORDER BY s.updated DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var (
			sess             Session
			created, updated string
		)
		if err := rows.Scan(&sess.ID, &sess.Title, &created, &updated, &sess.Messages); err != nil {
			return nil, err
		}
		sess.Created, _ = time.Parse(stampLayout, created)
		sess.Updated, _ = time.Parse(stampLayout, updated)
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Exists reports whether a session id is in the store.
func (s *Store) Exists(ctx context.Context, id string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Messages loads one session's conversation in order.
func (s *Store) Messages(ctx context.Context, id string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content, tool_calls, tool_call_id, name
		FROM messages WHERE session_id = ? ORDER BY ordinal`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var (
			m     Message
			calls string
		)
		if err := rows.Scan(&m.Role, &m.Content, &calls, &m.ToolCallID, &m.Name); err != nil {
			return nil, err
		}
		if calls != "" {
			if err := json.Unmarshal([]byte(calls), &m.ToolCalls); err != nil {
				return nil, fmt.Errorf("session %s: tool calls: %w", id, err)
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Append writes messages to a session, continuing from whatever is already stored.
//
// The ordinal is computed inside the transaction so two appends cannot both claim the
// same position. Deriving it from a count held in the caller is how a resumed session
// ends up with duplicate ordinals and half its history missing.
func (s *Store) Append(ctx context.Context, id string, msgs []Message) error {
	return s.AppendWithCost(ctx, id, msgs, nil)
}

// AppendWithCost stores messages and, for assistant messages, what they cost.
//
// costs may be nil, or the same length as msgs with zero values for messages that cost
// nothing. A prompt or a tool result has no price; the reply it produced does.
func (s *Store) AppendWithCost(ctx context.Context, id string, msgs []Message, costs []MessageCost) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var start int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(ordinal)+1, 0) FROM messages WHERE session_id = ?`, id).Scan(&start); err != nil {
		return err
	}

	for i, m := range msgs {
		calls := ""
		if len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err != nil {
				return err
			}
			calls = string(b)
		}
		var cost MessageCost
		if i < len(costs) {
			cost = costs[i]
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO messages (session_id, ordinal, role, content, tool_calls, tool_call_id, name,
			                      prompt_tokens, completion_tokens, cache_read_tokens, model)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, start+i, m.Role, m.Content, calls, m.ToolCallID, m.Name,
			cost.PromptTokens, cost.CompletionTokens, cost.CacheReadTokens, cost.Model); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `UPDATE sessions SET updated = ? WHERE id = ?`,
		stamp(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SetTitle replaces a session's title. Used once the first prompt is known.
func (s *Store) SetTitle(ctx context.Context, id, title string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET title = ? WHERE id = ?`, title, id)
	return err
}

// TitleFromPrompt makes a short title out of the first thing the person asked.
//
// A list of timestamps is unreadable. The first question is what a person recognises
// their own conversation by.
func TitleFromPrompt(prompt string, max int) string {
	if max <= 0 {
		max = 60
	}
	text := strings.TrimSpace(strings.Join(strings.Fields(prompt), " "))
	if text == "" {
		return "untitled"
	}
	if len([]rune(text)) <= max {
		return text
	}
	return strings.TrimSpace(string([]rune(text)[:max])) + "…"
}
