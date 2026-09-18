package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

func (s *Store) migrateBots() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS bot_grants (
  bot_id TEXT PRIMARY KEY,
  allowed INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
`)
	return err
}

func (s *Store) ListBotGrants(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT bot_id, allowed FROM bot_grants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]bool)
	for rows.Next() {
		var idStr string
		var allowed int
		if err := rows.Scan(&idStr, &allowed); err != nil {
			return nil, err
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		out[id] = allowed != 0
	}
	return out, rows.Err()
}

func (s *Store) SetBotAllowed(ctx context.Context, botID int64, allowed bool) error {
	if botID <= 0 {
		return fmt.Errorf("bot id required")
	}
	flag := 0
	if allowed {
		flag = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO bot_grants (bot_id, allowed, updated_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT(bot_id) DO UPDATE SET allowed = excluded.allowed, updated_at = excluded.updated_at`,
		strconv.FormatInt(botID, 10), flag)
	return err
}

func (s *Store) IsBotAllowed(ctx context.Context, botID int64) (bool, error) {
	var allowed int
	err := s.db.QueryRowContext(ctx, `SELECT allowed FROM bot_grants WHERE bot_id = ?`,
		strconv.FormatInt(botID, 10)).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // default deny
	}
	if err != nil {
		return false, err
	}
	return allowed != 0, nil
}
