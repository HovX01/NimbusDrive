package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Store) migrateSocial() error {
	var oldSQL string
	_ = s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'social_connections'`).Scan(&oldSQL)
	if oldSQL != "" && !strings.Contains(oldSQL, "account_key") {
		if err := s.migrateSocialV2(); err != nil {
			return err
		}
	}
	_, _ = s.db.Exec(`ALTER TABLE social_connections ADD COLUMN auth_type TEXT NOT NULL DEFAULT 'oauth'`)
	_, _ = s.db.Exec(`ALTER TABLE social_connections ADD COLUMN cookie_path TEXT NOT NULL DEFAULT ''`)
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS social_connections (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  account_id TEXT NOT NULL DEFAULT '',
  account_key TEXT NOT NULL,
  auth_type TEXT NOT NULL DEFAULT 'oauth',
  display_name TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  scopes TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  cookie_path TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  active INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE(provider, account_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_social_active_provider
  ON social_connections(provider) WHERE active = 1;
CREATE INDEX IF NOT EXISTS idx_social_provider ON social_connections(provider);
CREATE TABLE IF NOT EXISTS social_oauth_configs (
  provider TEXT PRIMARY KEY,
  client_id TEXT NOT NULL DEFAULT '',
  client_secret TEXT NOT NULL DEFAULT '',
  public_base_url TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
`)
	return err
}

func (s *Store) migrateSocialV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`ALTER TABLE social_connections RENAME TO social_connections_v1`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
CREATE TABLE social_connections (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  account_id TEXT NOT NULL DEFAULT '',
  account_key TEXT NOT NULL,
  auth_type TEXT NOT NULL DEFAULT 'oauth',
  display_name TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  scopes TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  cookie_path TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  active INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE(provider, account_key)
)`); err != nil {
		return err
	}
	rows, err := tx.Query(`
SELECT provider, display_name, username, scopes, access_token, refresh_token, enabled, expires_at, updated_at
FROM social_connections_v1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var provider, displayName, username, scopes, accessToken, refreshToken, updated string
		var enabled int
		var expires sql.NullString
		if err := rows.Scan(&provider, &displayName, &username, &scopes, &accessToken, &refreshToken, &enabled, &expires, &updated); err != nil {
			return err
		}
		if displayName == "" && username == "" {
			displayName = "Account 1"
		}
		if _, err := tx.Exec(`
INSERT INTO social_connections (
  id, provider, account_id, account_key, auth_type, display_name, username, scopes, access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
) VALUES (?, ?, '', ?, 'oauth', ?, ?, ?, ?, ?, '', ?, 1, ?, ?)`,
			uuid.NewString(), provider, legacyAccountKey(provider, accessToken, refreshToken), displayName, username, scopes, accessToken, refreshToken,
			enabled, nullString(expires), updated); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE social_connections_v1`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListSocialConnections(ctx context.Context) ([]domain.SocialConnection, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, provider, account_id, account_key, auth_type, display_name, username, scopes, enabled, active, expires_at, updated_at
FROM social_connections ORDER BY provider, active DESC, updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SocialConnection
	for rows.Next() {
		c, err := scanSocialConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetActiveSocialConnection(ctx context.Context, provider domain.SocialProvider) (domain.SocialConnectionSecret, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, provider, account_id, account_key, auth_type, display_name, username, scopes, access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
FROM social_connections WHERE provider = ? AND active = 1`, provider)
	return scanSocialConnectionSecret(row)
}

func (s *Store) UpsertSocialConnection(ctx context.Context, c domain.SocialConnectionSecret) (domain.SocialConnection, error) {
	now := time.Now().UTC()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.AccountKey == "" {
		c.AccountKey = legacyAccountKey(string(c.Provider), c.AccessToken, c.RefreshToken)
	}
	if c.AuthType == "" {
		c.AuthType = "oauth"
	}
	if c.DisplayName == "" && c.Username == "" {
		c.DisplayName = "Connected account"
	}
	c.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if c.Active {
		if _, err := tx.ExecContext(ctx, `UPDATE social_connections SET active = 0 WHERE provider = ?`, c.Provider); err != nil {
			return domain.SocialConnection{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO social_connections (
  id, provider, account_id, account_key, auth_type, display_name, username, scopes, access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider, account_key) DO UPDATE SET
  account_id = excluded.account_id,
  auth_type = excluded.auth_type,
  display_name = excluded.display_name,
  username = excluded.username,
  scopes = excluded.scopes,
  access_token = excluded.access_token,
  refresh_token = CASE WHEN excluded.refresh_token = '' THEN social_connections.refresh_token ELSE excluded.refresh_token END,
  cookie_path = CASE WHEN excluded.cookie_path = '' THEN social_connections.cookie_path ELSE excluded.cookie_path END,
  enabled = excluded.enabled,
  active = excluded.active,
  expires_at = excluded.expires_at,
  updated_at = excluded.updated_at`,
		c.ID, c.Provider, c.AccountID, c.AccountKey, c.AuthType, c.DisplayName, c.Username, strings.Join(c.Scopes, ","), c.AccessToken, c.RefreshToken, c.CookiePath,
		boolInt(c.Enabled), boolInt(c.Active), timePtrString(c.ExpiresAt), now.Format(time.RFC3339Nano))
	if err != nil {
		return domain.SocialConnection{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SocialConnection{}, err
	}
	secret, err := s.getBySocialAccountKey(ctx, c.Provider, c.AccountKey)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	return secret.SocialConnection, nil
}

func (s *Store) getBySocialAccountKey(ctx context.Context, provider domain.SocialProvider, accountKey string) (domain.SocialConnectionSecret, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, provider, account_id, account_key, auth_type, display_name, username, scopes, access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
FROM social_connections WHERE provider = ? AND account_key = ?`, provider, accountKey)
	return scanSocialConnectionSecret(row)
}

func (s *Store) SetSocialConnectionEnabled(ctx context.Context, provider domain.SocialProvider, enabled bool) (domain.SocialConnection, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE social_connections SET enabled = ?, updated_at = ? WHERE provider = ? AND active = 1`,
		boolInt(enabled), time.Now().UTC().Format(time.RFC3339Nano), provider)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.SocialConnection{}, domain.ErrNotFound
	}
	secret, err := s.GetActiveSocialConnection(ctx, provider)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	return secret.SocialConnection, nil
}

func (s *Store) SetActiveSocialConnection(ctx context.Context, provider domain.SocialProvider, id string) (domain.SocialConnection, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM social_connections WHERE provider = ? AND id = ?`, provider, id).Scan(&exists); err != nil {
		return domain.SocialConnection{}, err
	}
	if exists == 0 {
		return domain.SocialConnection{}, domain.ErrNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE social_connections SET active = 0 WHERE provider = ?`, provider); err != nil {
		return domain.SocialConnection{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE social_connections SET active = 1, enabled = 1, updated_at = ? WHERE provider = ? AND id = ?`, now, provider, id); err != nil {
		return domain.SocialConnection{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SocialConnection{}, err
	}
	secret, err := s.GetActiveSocialConnection(ctx, provider)
	if err != nil {
		return domain.SocialConnection{}, err
	}
	return secret.SocialConnection, nil
}

func (s *Store) DeleteSocialConnection(ctx context.Context, provider domain.SocialProvider, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM social_connections WHERE provider = ? AND id = ?`, provider, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE social_connections
SET active = 1, enabled = 1, updated_at = ?
WHERE id = (
  SELECT id FROM social_connections WHERE provider = ? ORDER BY updated_at DESC LIMIT 1
) AND NOT EXISTS (SELECT 1 FROM social_connections WHERE provider = ? AND active = 1)`,
		time.Now().UTC().Format(time.RFC3339Nano), provider, provider); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSocialConnection(row scanner) (domain.SocialConnection, error) {
	var c domain.SocialConnection
	var scopes string
	var enabled, active int
	var expires sql.NullString
	var updated string
	if err := row.Scan(&c.ID, &c.Provider, &c.AccountID, &c.AccountKey, &c.AuthType, &c.DisplayName, &c.Username, &scopes, &enabled, &active, &expires, &updated); err != nil {
		if err == sql.ErrNoRows {
			return domain.SocialConnection{}, domain.ErrNotFound
		}
		return domain.SocialConnection{}, err
	}
	c.Scopes = splitScopes(scopes)
	c.Connected = true
	c.Enabled = enabled != 0
	c.Active = active != 0
	if expires.Valid {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		if !t.IsZero() {
			c.ExpiresAt = &t
		}
	}
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, nil
}

func scanSocialConnectionSecret(row scanner) (domain.SocialConnectionSecret, error) {
	var c domain.SocialConnectionSecret
	var scopes string
	var enabled, active int
	var expires sql.NullString
	var updated string
	if err := row.Scan(&c.ID, &c.Provider, &c.AccountID, &c.AccountKey, &c.AuthType, &c.DisplayName, &c.Username, &scopes, &c.AccessToken, &c.RefreshToken, &c.CookiePath, &enabled, &active, &expires, &updated); err != nil {
		if err == sql.ErrNoRows {
			return domain.SocialConnectionSecret{}, domain.ErrNotFound
		}
		return domain.SocialConnectionSecret{}, err
	}
	c.Scopes = splitScopes(scopes)
	c.Connected = true
	c.Enabled = enabled != 0
	c.Active = active != 0
	if expires.Valid {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		if !t.IsZero() {
			c.ExpiresAt = &t
		}
	}
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, nil
}

func splitScopes(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func timePtrString(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func nullString(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}

func legacyAccountKey(provider, accessToken, refreshToken string) string {
	h := sha256.Sum256([]byte(provider + ":" + accessToken + ":" + refreshToken))
	return hex.EncodeToString(h[:])
}

func (s *Store) ListSocialOAuthConfigs(ctx context.Context) ([]domain.SocialOAuthAppConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT provider, client_id, client_secret, public_base_url, updated_at
FROM social_oauth_configs ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SocialOAuthAppConfig
	for rows.Next() {
		c, err := scanSocialOAuthConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetSocialOAuthConfig(ctx context.Context, provider domain.SocialProvider) (domain.SocialOAuthAppConfig, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT provider, client_id, client_secret, public_base_url, updated_at
FROM social_oauth_configs WHERE provider = ?`, provider)
	return scanSocialOAuthConfig(row)
}

func (s *Store) UpsertSocialOAuthConfig(ctx context.Context, c domain.SocialOAuthAppConfig) (domain.SocialOAuthAppConfig, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO social_oauth_configs (provider, client_id, client_secret, public_base_url, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(provider) DO UPDATE SET
  client_id = excluded.client_id,
  client_secret = excluded.client_secret,
  public_base_url = excluded.public_base_url,
  updated_at = excluded.updated_at`,
		c.Provider, strings.TrimSpace(c.ClientID), strings.TrimSpace(c.ClientSecret),
		strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/"), now.Format(time.RFC3339Nano))
	if err != nil {
		return domain.SocialOAuthAppConfig{}, err
	}
	return s.GetSocialOAuthConfig(ctx, c.Provider)
}

func scanSocialOAuthConfig(row scanner) (domain.SocialOAuthAppConfig, error) {
	var c domain.SocialOAuthAppConfig
	var updated string
	if err := row.Scan(&c.Provider, &c.ClientID, &c.ClientSecret, &c.PublicBaseURL, &updated); err != nil {
		if err == sql.ErrNoRows {
			return domain.SocialOAuthAppConfig{}, domain.ErrNotFound
		}
		return domain.SocialOAuthAppConfig{}, err
	}
	c.Configured = c.ClientID != "" && c.ClientSecret != ""
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, nil
}

var (
	_ domain.SocialConnectionRepository  = (*Store)(nil)
	_ domain.SocialOAuthConfigRepository = (*Store)(nil)
)
