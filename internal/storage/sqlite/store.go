package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xpanel/internal/auth"
	"xpanel/internal/inbound"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS inbounds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  remark TEXT NOT NULL,
  tag TEXT NOT NULL UNIQUE,
  listen TEXT NOT NULL,
  port INTEGER NOT NULL UNIQUE CHECK(port BETWEEN 1 AND 65535),
  protocol TEXT NOT NULL,
  network TEXT NOT NULL,
  security TEXT NOT NULL,
  client_id TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS config_revisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sha256 TEXT NOT NULL,
  content TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  salt TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return s.ensureInboundColumns()
}

func (s *Store) HasUser(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) ReplaceAdministrator(ctx context.Context, username, passwordHash, salt string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM users`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO users (username, password_hash, salt, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		username, passwordHash, salt, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	var user auth.User
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, salt, created_at FROM users WHERE username = ?`, username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Salt, &createdAt)
	if err != nil {
		return auth.User{}, err
	}
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, expiresAt.UTC().Format(time.RFC3339Nano), now)
	return err
}

func (s *Store) FindSession(ctx context.Context, token string) (auth.User, error) {
	var user auth.User
	var createdAt string
	var expiresAt string
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.username, u.password_hash, u.salt, u.created_at, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = ?`, token).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Salt, &createdAt, &expiresAt)
	if err != nil {
		return auth.User{}, err
	}
	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || time.Now().UTC().After(expiry) {
		_ = s.DeleteSession(ctx, token)
		return auth.User{}, sql.ErrNoRows
	}
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *Store) ensureInboundColumns() error {
	rows, err := s.db.Query(`PRAGMA table_info(inbounds)`)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	columns := map[string]string{
		"total_bytes":   "INTEGER NOT NULL DEFAULT 0",
		"used_bytes":    "INTEGER NOT NULL DEFAULT 0",
		"expiry_time":   "TEXT NOT NULL DEFAULT ''",
		"alter_id":      "INTEGER NOT NULL DEFAULT 0",
		"sniffing":      "INTEGER NOT NULL DEFAULT 1",
		"ws_path":       "TEXT NOT NULL DEFAULT '/xpanel'",
		"tls_cert_file": "TEXT NOT NULL DEFAULT ''",
		"tls_key_file":  "TEXT NOT NULL DEFAULT ''",
	}
	for name, definition := range columns {
		if existing[name] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE inbounds ADD COLUMN ` + name + ` ` + definition); err != nil {
			return fmt.Errorf("add inbound column %s: %w", name, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE inbounds SET expiry_time = ? WHERE expiry_time = ''`, inbound.DefaultExpiryTime); err != nil {
		return fmt.Errorf("backfill inbound expiry time: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]inbound.Inbound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, remark, tag, listen, port, protocol, network, security, client_id, email, enabled, total_bytes, used_bytes, expiry_time, alter_id, sniffing, ws_path, tls_cert_file, tls_key_file, created_at FROM inbounds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []inbound.Inbound{}
	for rows.Next() {
		var item inbound.Inbound
		var createdAt string
		if err := rows.Scan(&item.ID, &item.Remark, &item.Tag, &item.Listen, &item.Port, &item.Protocol, &item.Network, &item.Security, &item.ClientID, &item.Email, &item.Enabled, &item.TotalBytes, &item.UsedBytes, &item.ExpiryTime, &item.AlterID, &item.Sniffing, &item.WSPath, &item.TLSCertFile, &item.TLSKeyFile, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Create(ctx context.Context, item inbound.Inbound) (inbound.Inbound, error) {
	item.CreatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO inbounds (remark, tag, listen, port, protocol, network, security, client_id, email, enabled, total_bytes, used_bytes, expiry_time, alter_id, sniffing, ws_path, tls_cert_file, tls_key_file, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Remark, "pending-"+item.ClientID, item.Listen, item.Port, item.Protocol, item.Network, item.Security, item.ClientID, item.Email, item.Enabled, item.TotalBytes, item.UsedBytes, item.ExpiryTime, item.AlterID, item.Sniffing, item.WSPath, item.TLSCertFile, item.TLSKeyFile, item.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return inbound.Inbound{}, fmt.Errorf("%w: port or client ID already exists", inbound.ErrConflict)
		}
		return inbound.Inbound{}, err
	}
	item.ID, err = result.LastInsertId()
	if err != nil {
		return inbound.Inbound{}, err
	}
	item.Tag = fmt.Sprintf("inbound-%d", item.ID)
	if _, err = s.db.ExecContext(ctx, `UPDATE inbounds SET tag = ? WHERE id = ?`, item.Tag, item.ID); err != nil {
		return inbound.Inbound{}, err
	}
	return item, nil
}

func (s *Store) AddUsedBytes(ctx context.Context, id, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE inbounds SET used_bytes = used_bytes + ? WHERE id = ?`, delta, id)
	return err
}
