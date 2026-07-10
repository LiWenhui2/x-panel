package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xpanel/internal/auth"
	"xpanel/internal/inbound"
	"xpanel/internal/subscription"
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
);
CREATE TABLE IF NOT EXISTS subscriptions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  token_hint TEXT NOT NULL,
  token TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  total_bytes INTEGER NOT NULL DEFAULT 0,
  used_bytes INTEGER NOT NULL DEFAULT 0,
  expiry_time TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscription_inbounds (
  subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  inbound_id INTEGER NOT NULL REFERENCES inbounds(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (subscription_id, inbound_id)
);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	if err := s.ensureInboundColumns(); err != nil {
		return err
	}
	return s.ensureSubscriptionColumns()
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

func (s *Store) ensureSubscriptionColumns() error {
	rows, err := s.db.Query(`PRAGMA table_info(subscriptions)`)
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
		"total_bytes": "INTEGER NOT NULL DEFAULT 0",
		"used_bytes":  "INTEGER NOT NULL DEFAULT 0",
		"expiry_time": "TEXT NOT NULL DEFAULT ''",
		"token":       "TEXT NOT NULL DEFAULT ''",
	}
	for name, definition := range columns {
		if existing[name] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE subscriptions ADD COLUMN ` + name + ` ` + definition); err != nil {
			return fmt.Errorf("add subscription column %s: %w", name, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE subscriptions SET expiry_time = ? WHERE expiry_time = ''`, inbound.DefaultExpiryTime); err != nil {
		return fmt.Errorf("backfill subscription expiry time: %w", err)
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

func (s *Store) Update(ctx context.Context, id int64, item inbound.Inbound) (inbound.Inbound, error) {
	var usedBytes int64
	var createdAt string
	if err := s.db.QueryRowContext(ctx, `SELECT used_bytes, created_at FROM inbounds WHERE id = ?`, id).Scan(&usedBytes, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inbound.Inbound{}, inbound.ErrConflict
		}
		return inbound.Inbound{}, err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE inbounds SET remark = ?, listen = ?, port = ?, protocol = ?, network = ?, security = ?, client_id = ?, email = ?, enabled = ?, total_bytes = ?, expiry_time = ?, alter_id = ?, sniffing = ?, ws_path = ?, tls_cert_file = ?, tls_key_file = ? WHERE id = ?`,
		item.Remark, item.Listen, item.Port, item.Protocol, item.Network, item.Security, item.ClientID, item.Email, item.Enabled, item.TotalBytes, item.ExpiryTime, item.AlterID, item.Sniffing, item.WSPath, item.TLSCertFile, item.TLSKeyFile, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return inbound.Inbound{}, fmt.Errorf("%w: port or client ID already exists", inbound.ErrConflict)
		}
		return inbound.Inbound{}, err
	}
	item.ID = id
	item.Tag = fmt.Sprintf("inbound-%d", id)
	item.UsedBytes = usedBytes
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return item, nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM inbounds WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return inbound.ErrNotFound
	}
	return nil
}

func (s *Store) AddUsedBytes(ctx context.Context, id, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE inbounds SET used_bytes = used_bytes + ? WHERE id = ?`, delta, id)
	return err
}

func (s *Store) ListSubscriptions(ctx context.Context) ([]subscription.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, subscriptionSelect+` GROUP BY s.id ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []subscription.Subscription{}
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateSubscription(ctx context.Context, item subscription.Subscription, tokenHash string) (subscription.Subscription, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return subscription.Subscription{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO subscriptions (name, token_hash, token_hint, token, enabled, total_bytes, used_bytes, expiry_time, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Name, tokenHash, item.TokenHint, item.Token, item.Enabled, item.TotalBytes, item.UsedBytes, item.ExpiryTime, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return subscription.Subscription{}, err
	}
	item.ID, err = result.LastInsertId()
	if err != nil {
		return subscription.Subscription{}, err
	}
	if err = replaceSubscriptionInbounds(ctx, tx, item.ID, item.InboundIDs); err != nil {
		return subscription.Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return subscription.Subscription{}, err
	}
	item.CreatedAt, item.UpdatedAt = now, now
	return item, nil
}

func (s *Store) UpdateSubscription(ctx context.Context, id int64, input subscription.Input) (subscription.Subscription, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return subscription.Subscription{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE subscriptions SET name = ?, enabled = ?, total_bytes = ?, expiry_time = ?, updated_at = ? WHERE id = ?`,
		input.Name, input.Enabled, input.TotalBytes, input.ExpiryTime, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return subscription.Subscription{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	if err = replaceSubscriptionInbounds(ctx, tx, id, input.InboundIDs); err != nil {
		return subscription.Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return subscription.Subscription{}, err
	}
	return s.getSubscriptionByID(ctx, id)
}

func (s *Store) RotateSubscriptionToken(ctx context.Context, id int64, tokenHash, hint, token string) (subscription.Subscription, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE subscriptions SET token_hash = ?, token_hint = ?, token = ?, updated_at = ? WHERE id = ?`,
		tokenHash, hint, token, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return subscription.Subscription{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	return s.getSubscriptionByID(ctx, id)
}

func (s *Store) DeleteSubscription(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE inbounds SET enabled = 0 WHERE id IN (
SELECT inbound_id FROM subscription_inbounds WHERE subscription_id = ?
) AND id NOT IN (
SELECT inbound_id FROM subscription_inbounds WHERE subscription_id <> ?
)`, id, id)
	if err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return subscription.ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) FindSubscriptionByTokenHash(ctx context.Context, tokenHash string) (subscription.Subscription, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, subscriptionSelect+` WHERE s.token_hash = ? GROUP BY s.id`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	return item, err
}

func (s *Store) SubscriptionToken(ctx context.Context, id int64) (subscription.Subscription, string, error) {
	item, err := s.getSubscriptionByID(ctx, id)
	if err != nil {
		return subscription.Subscription{}, "", err
	}
	return item, item.Token, nil
}

const subscriptionSelect = `
SELECT s.id, s.name, s.enabled, s.token_hint, s.token, s.total_bytes, s.used_bytes, s.expiry_time, s.created_at, s.updated_at,
       COALESCE(GROUP_CONCAT(si.inbound_id, ','), '')
FROM subscriptions s
LEFT JOIN subscription_inbounds si ON si.subscription_id = s.id`

type rowScanner interface{ Scan(...any) error }

func scanSubscription(scanner rowScanner) (subscription.Subscription, error) {
	var item subscription.Subscription
	var createdAt, updatedAt, inboundIDs string
	if err := scanner.Scan(&item.ID, &item.Name, &item.Enabled, &item.TokenHint, &item.Token, &item.TotalBytes, &item.UsedBytes, &item.ExpiryTime, &createdAt, &updatedAt, &inboundIDs); err != nil {
		return subscription.Subscription{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	item.InboundIDs = []int64{}
	if inboundIDs != "" {
		for _, value := range strings.Split(inboundIDs, ",") {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return subscription.Subscription{}, err
			}
			item.InboundIDs = append(item.InboundIDs, id)
		}
	}
	if item.ExpiryTime == "" {
		item.ExpiryTime = inbound.DefaultExpiryTime
	}
	if item.TotalBytes <= 0 {
		item.RemainingBytes = 0
		return item, nil
	}
	item.RemainingBytes = item.TotalBytes - item.UsedBytes
	if item.RemainingBytes < 0 {
		item.RemainingBytes = 0
	}
	return item, nil
}

func (s *Store) getSubscriptionByID(ctx context.Context, id int64) (subscription.Subscription, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, subscriptionSelect+` WHERE s.id = ? GROUP BY s.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	return item, err
}

func replaceSubscriptionInbounds(ctx context.Context, tx *sql.Tx, subscriptionID int64, inboundIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_inbounds WHERE subscription_id = ?`, subscriptionID); err != nil {
		return err
	}
	for index, inboundID := range inboundIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_inbounds (subscription_id, inbound_id, sort_order) VALUES (?, ?, ?)`,
			subscriptionID, inboundID, index); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListSubscriptionBindings(ctx context.Context) ([]inbound.SubscriptionBinding, error) {
	items, err := s.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	bindings := make([]inbound.SubscriptionBinding, 0, len(items))
	for _, item := range items {
		bindings = append(bindings, inbound.SubscriptionBinding{
			ID: item.ID, Name: item.Name, Enabled: item.Enabled, InboundIDs: item.InboundIDs,
			TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes, ExpiryTime: item.ExpiryTime,
		})
	}
	return bindings, nil
}

func (s *Store) AddSubscriptionUsedBytes(ctx context.Context, id, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE subscriptions SET used_bytes = used_bytes + ?, updated_at = ? WHERE id = ?`,
		delta, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
