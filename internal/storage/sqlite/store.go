package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

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
);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return s.ensureInboundColumns()
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
	return nil
}

func (s *Store) List(ctx context.Context) ([]inbound.Inbound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, remark, tag, listen, port, protocol, network, security, client_id, email, enabled, total_bytes, expiry_time, alter_id, sniffing, ws_path, tls_cert_file, tls_key_file, created_at FROM inbounds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []inbound.Inbound{}
	for rows.Next() {
		var item inbound.Inbound
		var createdAt string
		if err := rows.Scan(&item.ID, &item.Remark, &item.Tag, &item.Listen, &item.Port, &item.Protocol, &item.Network, &item.Security, &item.ClientID, &item.Email, &item.Enabled, &item.TotalBytes, &item.ExpiryTime, &item.AlterID, &item.Sniffing, &item.WSPath, &item.TLSCertFile, &item.TLSKeyFile, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Create(ctx context.Context, item inbound.Inbound) (inbound.Inbound, error) {
	item.CreatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO inbounds (remark, tag, listen, port, protocol, network, security, client_id, email, enabled, total_bytes, expiry_time, alter_id, sniffing, ws_path, tls_cert_file, tls_key_file, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Remark, "pending-"+item.ClientID, item.Listen, item.Port, item.Protocol, item.Network, item.Security, item.ClientID, item.Email, item.Enabled, item.TotalBytes, item.ExpiryTime, item.AlterID, item.Sniffing, item.WSPath, item.TLSCertFile, item.TLSKeyFile, item.CreatedAt.Format(time.RFC3339Nano))
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
