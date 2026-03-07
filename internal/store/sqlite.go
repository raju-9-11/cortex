package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteStore implements the Store interface using SQLite via modernc.org/sqlite.
type SQLiteStore struct {
	writer *sql.DB // MaxOpenConns=1 (serialized writes)
	reader *sql.DB // MaxOpenConns=4 (concurrent reads)
}

// NewSQLiteStore opens a SQLite database at the given path and returns a
// new SQLiteStore with separate writer and reader connection pools.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode%%3DWAL&_pragma=busy_timeout%%3D5000&_pragma=foreign_keys%%3D1",
		path,
	)

	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("opening sqlite reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	return &SQLiteStore{writer: writer, reader: reader}, nil
}

// Close shuts down both the reader and writer connection pools.
func (s *SQLiteStore) Close() error {
	s.reader.Close()
	return s.writer.Close()
}

// Ping verifies database connectivity.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.reader.PingContext(ctx)
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

// Migrate reads embedded SQL migration files and applies any that have not
// yet been recorded in the _migrations table.
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	// Ensure the _migrations table exists before checking applied versions.
	_, err := s.writer.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`)
	if err != nil {
		return fmt.Errorf("creating _migrations table: %w", err)
	}

	// Read all migration files from the embedded filesystem.
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	// Collect and sort file names lexicographically.
	var fileNames []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		version := strings.TrimSuffix(name, ".sql")

		// Check if this migration has already been applied.
		var exists int
		err := s.writer.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM _migrations WHERE version = ?`, version,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if exists > 0 {
			continue
		}

		// Read the migration SQL.
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		// Apply the migration within a transaction.
		tx, err := s.writer.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning transaction for %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO _migrations (version) VALUES (?)`, version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.writer.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, title, model, system_prompt, status,
		                      token_count, message_count, created_at, updated_at, last_access)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.Title, sess.Model, sess.SystemPrompt, sess.Status,
		sess.TokenCount, sess.MessageCount,
		sess.CreatedAt.UTC().Format(timeFormat),
		sess.UpdatedAt.UTC().Format(timeFormat),
		sess.LastAccess.UTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.reader.QueryRowContext(ctx, `
		SELECT id, user_id, title, model, system_prompt, status,
		       token_count, message_count, created_at, updated_at, last_access
		FROM sessions WHERE id = ?`, id)

	sess, err := scanSession(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return sess, nil
}

func (s *SQLiteStore) ListSessions(ctx context.Context, params SessionListParams) ([]Session, error) {
	query := `SELECT id, user_id, title, model, system_prompt, status,
	                 token_count, message_count, created_at, updated_at, last_access
	          FROM sessions WHERE 1=1`
	var args []any

	if params.UserID != "" {
		query += ` AND user_id = ?`
		args = append(args, params.UserID)
	}
	if params.Status != "" {
		query += ` AND status = ?`
		args = append(args, params.Status)
	}

	query += ` ORDER BY last_access DESC`

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	if params.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, params.Offset)
	}

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, *sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}
	return sessions, nil
}

func (s *SQLiteStore) UpdateSession(ctx context.Context, sess *Session) error {
	result, err := s.writer.ExecContext(ctx, `
		UPDATE sessions
		SET title = ?, model = ?, system_prompt = ?, status = ?,
		    token_count = ?, message_count = ?,
		    updated_at = ?, last_access = ?
		WHERE id = ?`,
		sess.Title, sess.Model, sess.SystemPrompt, sess.Status,
		sess.TokenCount, sess.MessageCount,
		sess.UpdatedAt.UTC().Format(timeFormat),
		sess.LastAccess.UTC().Format(timeFormat),
		sess.ID,
	)
	if err != nil {
		return fmt.Errorf("updating session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, id string) error {
	result, err := s.writer.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateMessage(ctx context.Context, msg *Message) error {
	var parentID sql.NullString
	if msg.ParentID != nil {
		parentID = sql.NullString{String: *msg.ParentID, Valid: true}
	}

	var model sql.NullString
	if msg.Model != "" {
		model = sql.NullString{String: msg.Model, Valid: true}
	}

	var metadata sql.NullString
	if msg.Metadata != nil {
		metadata = sql.NullString{String: *msg.Metadata, Valid: true}
	}

	_, err := s.writer.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, parent_id, role, content,
		                      token_count, is_active, pinned, model, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, parentID, msg.Role, msg.Content,
		msg.TokenCount, boolToInt(msg.IsActive), boolToInt(msg.Pinned),
		model, metadata,
		msg.CreatedAt.UTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("creating message: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMessage(ctx context.Context, id string) (*Message, error) {
	row := s.reader.QueryRowContext(ctx, `
		SELECT id, session_id, parent_id, role, content,
		       token_count, is_active, pinned, model, metadata, created_at
		FROM messages WHERE id = ?`, id)

	msg, err := scanMessage(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting message: %w", err)
	}
	return msg, nil
}

func (s *SQLiteStore) ListMessages(ctx context.Context, params MessageListParams) ([]Message, error) {
	query := `SELECT id, session_id, parent_id, role, content,
	                 token_count, is_active, pinned, model, metadata, created_at
	          FROM messages WHERE session_id = ?`
	args := []any{params.SessionID}

	if params.ActiveOnly {
		query += ` AND is_active = 1`
	}

	query += ` ORDER BY created_at ASC`

	if params.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, params.Limit)
	}
	if params.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, params.Offset)
	}

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		msg, err := scanMessageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating messages: %w", err)
	}
	return messages, nil
}

func (s *SQLiteStore) DeactivateMessages(ctx context.Context, sessionID string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Build parameterized placeholder list: (?, ?, ?)
	placeholders := make([]string, len(messageIDs))
	args := make([]any, 0, len(messageIDs)+1)
	args = append(args, sessionID)
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		UPDATE messages SET is_active = 0
		WHERE session_id = ? AND id IN (%s)`,
		strings.Join(placeholders, ", "),
	)

	_, err := s.writer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("deactivating messages: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

func (s *SQLiteStore) UpsertProvider(ctx context.Context, p *Provider) error {
	_, err := s.writer.ExecContext(ctx, `
		INSERT INTO providers (id, type, base_url, api_key, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		    type = excluded.type,
		    base_url = excluded.base_url,
		    api_key = excluded.api_key,
		    enabled = excluded.enabled`,
		p.ID, p.Type, p.BaseURL, nullString(p.APIKey), boolToInt(p.Enabled),
		p.CreatedAt.UTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("upserting provider: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetProvider(ctx context.Context, id string) (*Provider, error) {
	row := s.reader.QueryRowContext(ctx, `
		SELECT id, type, base_url, api_key, enabled, created_at
		FROM providers WHERE id = ?`, id)

	p, err := scanProvider(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting provider: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.reader.QueryContext(ctx, `
		SELECT id, type, base_url, api_key, enabled, created_at
		FROM providers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing providers: %w", err)
	}
	defer rows.Close()

	var providers []Provider
	for rows.Next() {
		p, err := scanProviderRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning provider: %w", err)
		}
		providers = append(providers, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating providers: %w", err)
	}
	return providers, nil
}

func (s *SQLiteStore) DeleteProvider(ctx context.Context, id string) error {
	result, err := s.writer.ExecContext(ctx, `DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting provider: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSessionFromScanner(sc scanner) (*Session, error) {
	var sess Session
	var createdAt, updatedAt, lastAccess string

	err := sc.Scan(
		&sess.ID, &sess.UserID, &sess.Title, &sess.Model,
		&sess.SystemPrompt, &sess.Status,
		&sess.TokenCount, &sess.MessageCount,
		&createdAt, &updatedAt, &lastAccess,
	)
	if err != nil {
		return nil, err
	}

	sess.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	sess.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
	sess.LastAccess, _ = time.Parse(timeFormat, lastAccess)

	return &sess, nil
}

func scanSession(row *sql.Row) (*Session, error) {
	return scanSessionFromScanner(row)
}

func scanSessionRow(rows *sql.Rows) (*Session, error) {
	return scanSessionFromScanner(rows)
}

func scanMessageFromScanner(sc scanner) (*Message, error) {
	var msg Message
	var parentID, model, metadata sql.NullString
	var isActive, pinned int
	var createdAt string

	err := sc.Scan(
		&msg.ID, &msg.SessionID, &parentID, &msg.Role, &msg.Content,
		&msg.TokenCount, &isActive, &pinned, &model, &metadata, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		msg.ParentID = &parentID.String
	}
	msg.IsActive = isActive != 0
	msg.Pinned = pinned != 0
	if model.Valid {
		msg.Model = model.String
	}
	if metadata.Valid {
		msg.Metadata = &metadata.String
	}
	msg.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	return &msg, nil
}

func scanMessage(row *sql.Row) (*Message, error) {
	return scanMessageFromScanner(row)
}

func scanMessageRow(rows *sql.Rows) (*Message, error) {
	return scanMessageFromScanner(rows)
}

func scanProviderFromScanner(sc scanner) (*Provider, error) {
	var p Provider
	var apiKey sql.NullString
	var enabled int
	var createdAt string

	err := sc.Scan(&p.ID, &p.Type, &p.BaseURL, &apiKey, &enabled, &createdAt)
	if err != nil {
		return nil, err
	}

	if apiKey.Valid {
		p.APIKey = apiKey.String
	}
	p.Enabled = enabled != 0
	p.CreatedAt, _ = time.Parse(timeFormat, createdAt)

	return &p, nil
}

func scanProvider(row *sql.Row) (*Provider, error) {
	return scanProviderFromScanner(row)
}

func scanProviderRow(rows *sql.Rows) (*Provider, error) {
	return scanProviderFromScanner(rows)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
