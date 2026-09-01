package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Store) CreateShare(ctx context.Context, link domain.ShareLink) (domain.ShareLink, error) {
	if link.ID == "" {
		link.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	var expires any
	if link.ExpiresAt != nil {
		expires = link.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO share_links (id, token, node_id, created_at, expires_at, revoked_at, download_count)
VALUES (?, ?, ?, ?, ?, NULL, 0)`,
		link.ID, link.Token, link.NodeID, now.Format(time.RFC3339Nano), expires,
	)
	if err != nil {
		return domain.ShareLink{}, err
	}
	return link, nil
}

func (s *Store) GetByToken(ctx context.Context, token string) (domain.ShareLink, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, token, node_id, created_at, expires_at, revoked_at, download_count
FROM share_links WHERE token = ?`, token)
	return scanShare(row)
}

func (s *Store) ListByNode(ctx context.Context, nodeID string) ([]domain.ShareLink, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, token, node_id, created_at, expires_at, revoked_at, download_count
FROM share_links WHERE node_id = ? AND revoked_at IS NULL
ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ShareLink
	for rows.Next() {
		sh, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (s *Store) DeleteByNode(ctx context.Context, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM share_links WHERE node_id = ?`, nodeID)
	return err
}

func (s *Store) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `
UPDATE share_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) IncrementDownload(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE share_links SET download_count = download_count + 1 WHERE id = ?`, id)
	return err
}

func scanShare(row scanner) (domain.ShareLink, error) {
	var sh domain.ShareLink
	var created string
	var expires, revoked sql.NullString
	err := row.Scan(&sh.ID, &sh.Token, &sh.NodeID, &created, &expires, &revoked, &sh.DownloadCount)
	if err == sql.ErrNoRows {
		return domain.ShareLink{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ShareLink{}, err
	}
	sh.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if expires.Valid {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		sh.ExpiresAt = &t
	}
	if revoked.Valid {
		t, _ := time.Parse(time.RFC3339Nano, revoked.String)
		sh.RevokedAt = &t
	}
	return sh, nil
}

var _ domain.ShareRepository = (*Store)(nil)

func (s *Store) migrateShare() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS share_links (
  id TEXT PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  revoked_at TEXT,
  download_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_share_links_node ON share_links(node_id);
CREATE INDEX IF NOT EXISTS idx_share_links_token ON share_links(token);
`)
	if err != nil {
		return err
	}
	return s.migrateShareCascade()
}

// migrateShareCascade rebuilds share_links with ON DELETE CASCADE for DBs created before that FK rule.
func (s *Store) migrateShareCascade() error {
	var ddl string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='share_links'`).Scan(&ddl); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if strings.Contains(ddl, "ON DELETE CASCADE") {
		return nil
	}
	_, err := s.db.Exec(`
PRAGMA foreign_keys=OFF;
BEGIN;
CREATE TABLE share_links_new (
  id TEXT PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  revoked_at TEXT,
  download_count INTEGER NOT NULL DEFAULT 0
);
INSERT INTO share_links_new SELECT * FROM share_links;
DROP TABLE share_links;
ALTER TABLE share_links_new RENAME TO share_links;
CREATE INDEX IF NOT EXISTS idx_share_links_node ON share_links(node_id);
CREATE INDEX IF NOT EXISTS idx_share_links_token ON share_links(token);
COMMIT;
PRAGMA foreign_keys=ON;
`)
	return err
}

func shareActive(sh domain.ShareLink) bool {
	if sh.RevokedAt != nil {
		return false
	}
	if sh.ExpiresAt != nil && time.Now().After(*sh.ExpiresAt) {
		return false
	}
	return strings.TrimSpace(sh.Token) != ""
}
