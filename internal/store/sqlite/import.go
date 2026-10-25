package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

// MergeDatabase merges a Nimbus SQLite snapshot into the live database.
// Existing rows win; folders merge by name, duplicate files (same content hash)
// are skipped, and conflicting file names are imported with a suffix.
func (s *Store) MergeDatabase(ctx context.Context, sourcePath string) (domain.ImportSummary, error) {
	var summary domain.ImportSummary
	if strings.TrimSpace(sourcePath) == "" {
		return summary, fmt.Errorf("%w: import file is required", domain.ErrValidation)
	}
	if s.path != "" {
		if srcInfo, err := os.Stat(sourcePath); err == nil {
			if targetInfo, err := os.Stat(s.path); err == nil && os.SameFile(srcInfo, targetInfo) {
				return summary, fmt.Errorf("%w: cannot import the live database into itself", domain.ErrValidation)
			}
		}
	}

	s.importMu.Lock()
	defer s.importMu.Unlock()

	src, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return summary, fmt.Errorf("open import database: %w", err)
	}
	defer src.Close()
	src.SetMaxOpenConns(1)
	_, _ = src.ExecContext(ctx, `PRAGMA query_only=ON`)

	if err := validateImportSource(ctx, src); err != nil {
		return summary, err
	}

	nodes, err := loadImportNodes(ctx, src)
	if err != nil {
		return summary, err
	}
	hashes, err := loadImportHashes(ctx, src)
	if err != nil {
		return summary, err
	}
	parts, err := loadImportParts(ctx, src)
	if err != nil {
		return summary, err
	}

	if _, err := s.EnsureRoot(ctx); err != nil {
		return summary, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return summary, err
	}
	defer func() { _ = tx.Rollback() }()

	idMap := map[string]string{"root": "root"}
	for _, n := range orderedImportNodes(nodes) {
		if n.ID == "root" {
			continue
		}
		if n.Status != string(domain.StatusReady) {
			summary.NodesSkipped++
			continue
		}
		if n.Type != string(domain.NodeFolder) && n.Type != string(domain.NodeFile) {
			summary.NodesSkipped++
			continue
		}

		parentID := "root"
		if n.ParentID != "" && n.ParentID != "root" {
			mapped, ok := idMap[n.ParentID]
			if !ok {
				addImportWarning(&summary, "skipped %q: parent was not imported", n.Name)
				summary.NodesSkipped++
				continue
			}
			parentID = mapped
		}

		if n.Type == string(domain.NodeFolder) {
			existingID, existingType, err := importChild(ctx, tx, parentID, n.Name)
			if err != nil {
				return summary, err
			}
			if existingID != "" && existingType == string(domain.NodeFolder) {
				idMap[n.ID] = existingID
				summary.FoldersMerged++
				continue
			}

			name := n.Name
			if existingID != "" {
				name = uniqueImportName(ctx, tx, parentID, n.Name, false)
			}
			newID := importNodeID(ctx, tx, n.ID)
			if err := insertImportNode(ctx, tx, newID, parentID, name, n); err != nil {
				return summary, err
			}
			idMap[n.ID] = newID
			summary.FoldersCreated++
			continue
		}

		hash := hashes[n.ID]
		if hash != "" {
			existingID, err := importHashNode(ctx, tx, hash)
			if err != nil {
				return summary, err
			}
			if existingID != "" {
				idMap[n.ID] = existingID
				summary.FilesSkipped++
				continue
			}
		}

		name := n.Name
		if existingID, _, err := importChild(ctx, tx, parentID, n.Name); err != nil {
			return summary, err
		} else if existingID != "" {
			name = uniqueImportName(ctx, tx, parentID, n.Name, true)
			summary.FilesRenamed++
		}

		newID := importNodeID(ctx, tx, n.ID)
		if err := insertImportNode(ctx, tx, newID, parentID, name, n); err != nil {
			return summary, err
		}
		idMap[n.ID] = newID
		summary.FilesImported++

		fileParts := parts[n.ID]
		var partBytes int64
		for _, p := range fileParts {
			partID := p.ID
			if partID == "" || importPartIDExists(ctx, tx, partID) {
				partID = uuid.NewString()
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO file_parts (id, file_id, part_no, message_id, size, channel_id)
VALUES (?, ?, ?, ?, ?, ?)`, partID, newID, p.PartNo, p.MessageID, p.Size, p.ChannelID); err != nil {
				return summary, err
			}
			partBytes += p.Size
			summary.PartsImported++
		}
		if n.Size > 0 && len(fileParts) == 0 {
			addImportWarning(&summary, "file %q has no parts; bytes will be unavailable", name)
		} else if len(fileParts) > 0 && partBytes != n.Size {
			addImportWarning(&summary, "file %q size is %d but parts total %d", name, n.Size, partBytes)
		}

		if hash != "" {
			res, err := tx.ExecContext(ctx, `
INSERT INTO file_hashes (hash, node_id) VALUES (?, ?)
ON CONFLICT(hash) DO UPDATE SET node_id = excluded.node_id`, hash, newID)
			if err != nil {
				return summary, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				summary.HashesImported++
			}
		}
	}

	if err := importShares(ctx, tx, src, idMap, &summary); err != nil {
		return summary, err
	}
	if err := importEditProjects(ctx, tx, src, idMap, &summary); err != nil {
		return summary, err
	}
	if err := importDataRows(ctx, tx, src, &summary); err != nil {
		return summary, err
	}
	if err := importBotGrants(ctx, tx, src, &summary); err != nil {
		return summary, err
	}
	if err := importSocial(ctx, tx, src, &summary); err != nil {
		return summary, err
	}

	if err := tx.Commit(); err != nil {
		return summary, err
	}
	if err := s.rebuildFTS(ctx); err != nil {
		addImportWarning(&summary, "search index rebuild failed: %v", err)
	}
	return summary, nil
}

type importNode struct {
	ID        string
	ParentID  string
	Name      string
	Type      string
	MimeType  string
	Size      int64
	Status    string
	ChannelID int64
	CreatedAt string
	UpdatedAt string
}

type importPart struct {
	ID        string
	FileID    string
	PartNo    int
	MessageID int
	Size      int64
	ChannelID int64
}

func validateImportSource(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("%w: cannot read import database", domain.ErrValidation)
	}
	ok := true
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(line), "ok") {
			ok = false
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: import database failed SQLite integrity check", domain.ErrValidation)
	}
	for _, table := range []string{"nodes", "file_parts"} {
		exists, err := importTableExists(ctx, db, table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: import is not a Nimbus database (missing %s)", domain.ErrValidation, table)
		}
	}
	return nil
}

func loadImportNodes(ctx context.Context, db *sql.DB) ([]importNode, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, COALESCE(parent_id, ''), name, type, mime_type, size, status, channel_id, created_at, updated_at
FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read nodes from import database", domain.ErrValidation)
	}
	defer rows.Close()
	var out []importNode
	for rows.Next() {
		var n importNode
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Name, &n.Type, &n.MimeType, &n.Size, &n.Status, &n.ChannelID, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func orderedImportNodes(nodes []importNode) []importNode {
	byID := make(map[string]importNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	depth := make(map[string]int, len(nodes))
	var walk func(string, map[string]bool) int
	walk = func(id string, seen map[string]bool) int {
		if d, ok := depth[id]; ok {
			return d
		}
		n, ok := byID[id]
		if !ok {
			return 0
		}
		if n.ParentID == "" || n.ParentID == "root" {
			depth[id] = 0
			return 0
		}
		if seen[id] {
			depth[id] = 1 << 20
			return depth[id]
		}
		seen[id] = true
		d := walk(n.ParentID, seen) + 1
		depth[id] = d
		delete(seen, id)
		return d
	}
	for _, n := range nodes {
		walk(n.ID, map[string]bool{})
	}
	out := append([]importNode(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool {
		return depth[out[i].ID] < depth[out[j].ID]
	})
	return out
}

func loadImportHashes(ctx context.Context, db *sql.DB) (map[string]string, error) {
	exists, err := importTableExists(ctx, db, "file_hashes")
	if err != nil || !exists {
		return map[string]string{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT node_id, hash FROM file_hashes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var nodeID, hash string
		if err := rows.Scan(&nodeID, &hash); err != nil {
			return nil, err
		}
		out[nodeID] = hash
	}
	return out, rows.Err()
}

func loadImportParts(ctx context.Context, db *sql.DB) (map[string][]importPart, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, file_id, part_no, message_id, size, channel_id
FROM file_parts ORDER BY file_id, part_no`)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read file parts from import database", domain.ErrValidation)
	}
	defer rows.Close()
	out := make(map[string][]importPart)
	for rows.Next() {
		var p importPart
		if err := rows.Scan(&p.ID, &p.FileID, &p.PartNo, &p.MessageID, &p.Size, &p.ChannelID); err != nil {
			return nil, err
		}
		out[p.FileID] = append(out[p.FileID], p)
	}
	return out, rows.Err()
}

func importTableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func importChild(ctx context.Context, tx *sql.Tx, parentID, name string) (string, string, error) {
	var id, typ string
	err := tx.QueryRowContext(ctx, `
SELECT id, type FROM nodes WHERE parent_id = ? AND name = ? AND status != 'deleted'`, parentID, name).Scan(&id, &typ)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return id, typ, err
}

func uniqueImportName(ctx context.Context, tx *sql.Tx, parentID, name string, isFile bool) string {
	base, ext := name, ""
	if isFile {
		ext = filepath.Ext(name)
		base = strings.TrimSuffix(name, ext)
	}
	for i := 1; i < 1000; i++ {
		suffix := " (imported)"
		if i > 1 {
			suffix = fmt.Sprintf(" (imported %d)", i)
		}
		candidate := base + suffix + ext
		id, _, err := importChild(ctx, tx, parentID, candidate)
		if err != nil || id == "" {
			return candidate
		}
	}
	return base + " (imported)" + ext
}

func importNodeID(ctx context.Context, tx *sql.Tx, preferred string) string {
	if preferred == "" || preferred == "root" {
		return uuid.NewString()
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE id = ?`, preferred).Scan(&n); err == nil && n == 0 {
		return preferred
	}
	return uuid.NewString()
}

func importHashNode(ctx context.Context, tx *sql.Tx, hash string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
SELECT n.id FROM file_hashes h JOIN nodes n ON n.id = h.node_id
WHERE h.hash = ? AND n.status = 'ready' LIMIT 1`, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func importPartIDExists(ctx context.Context, tx *sql.Tx, id string) bool {
	var n int
	return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_parts WHERE id = ?`, id).Scan(&n) == nil && n > 0
}

func insertImportNode(ctx context.Context, tx *sql.Tx, id, parentID, name string, n importNode) error {
	created, updated := n.CreatedAt, n.UpdatedAt
	if created == "" || updated == "" {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if created == "" {
			created = now
		}
		if updated == "" {
			updated = now
		}
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, parent_id, name, type, mime_type, size, status, channel_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'ready', ?, ?, ?)`,
		id, parentID, name, n.Type, n.MimeType, n.Size, n.ChannelID, created, updated)
	return err
}

func importShares(ctx context.Context, tx *sql.Tx, src *sql.DB, idMap map[string]string, summary *domain.ImportSummary) error {
	exists, err := importTableExists(ctx, src, "share_links")
	if err != nil || !exists {
		return err
	}
	rows, err := src.QueryContext(ctx, `
SELECT id, token, node_id, created_at, expires_at, revoked_at, download_count
FROM share_links`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, token, nodeID, created string
		var expires, revoked sql.NullString
		var count int
		if err := rows.Scan(&id, &token, &nodeID, &created, &expires, &revoked, &count); err != nil {
			return err
		}
		mapped, ok := idMap[nodeID]
		if !ok || strings.TrimSpace(token) == "" {
			continue
		}
		var taken int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM share_links WHERE token = ?`, token).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			continue
		}
		if id == "" || importRowIDExists(ctx, tx, "share_links", id) {
			id = uuid.NewString()
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO share_links (id, token, node_id, created_at, expires_at, revoked_at, download_count)
VALUES (?, ?, ?, ?, ?, ?, ?)`, id, token, mapped, created, nullString(expires), nullString(revoked), count)
		if err != nil {
			return err
		}
		summary.SharesImported++
	}
	return rows.Err()
}

func importEditProjects(ctx context.Context, tx *sql.Tx, src *sql.DB, idMap map[string]string, summary *domain.ImportSummary) error {
	exists, err := importTableExists(ctx, src, "edit_projects")
	if err != nil || !exists {
		return err
	}
	rows, err := src.QueryContext(ctx, `
SELECT id, source_node_id, name, timeline_json, created_at, updated_at
FROM edit_projects`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sourceID, name, timeline, created, updated string
		if err := rows.Scan(&id, &sourceID, &name, &timeline, &created, &updated); err != nil {
			return err
		}
		mapped, ok := idMap[sourceID]
		if !ok {
			continue
		}
		if id == "" {
			id = uuid.NewString()
		} else if importRowIDExists(ctx, tx, "edit_projects", id) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO edit_projects (id, source_node_id, name, timeline_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, id, mapped, name, remapTimelineIDs(timeline, idMap), created, updated); err != nil {
			return err
		}
		summary.EditProjectsImported++
	}
	return rows.Err()
}

func importDataRows(ctx context.Context, tx *sql.Tx, src *sql.DB, summary *domain.ImportSummary) error {
	exists, err := importTableExists(ctx, src, "data_rows")
	if err != nil || !exists {
		return err
	}
	rows, err := src.QueryContext(ctx, `SELECT id, collection, data, created_at, updated_at FROM data_rows`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, collection, data, created, updated string
		if err := rows.Scan(&id, &collection, &data, &created, &updated); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO data_rows (id, collection, data, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, id, collection, data, created, updated)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			summary.DataRowsImported++
		}
	}
	return rows.Err()
}

func importBotGrants(ctx context.Context, tx *sql.Tx, src *sql.DB, summary *domain.ImportSummary) error {
	exists, err := importTableExists(ctx, src, "bot_grants")
	if err != nil || !exists {
		return err
	}
	rows, err := src.QueryContext(ctx, `SELECT bot_id, allowed, updated_at FROM bot_grants`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var botID, updated string
		var allowed int
		if err := rows.Scan(&botID, &allowed, &updated); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO bot_grants (bot_id, allowed, updated_at) VALUES (?, ?, ?)`, botID, allowed, updated)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			summary.BotGrantsImported++
		}
	}
	return rows.Err()
}

func importSocial(ctx context.Context, tx *sql.Tx, src *sql.DB, summary *domain.ImportSummary) error {
	exists, err := importTableExists(ctx, src, "social_connections")
	if err != nil || !exists {
		return err
	}
	rows, err := src.QueryContext(ctx, `
SELECT id, provider, account_id, account_key, auth_type, display_name, username, scopes,
       access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
FROM social_connections`)
	if err != nil {
		addImportWarning(summary, "social connections were not imported: %v", err)
		return nil
	}
	for rows.Next() {
		var id, provider, accountID, accountKey, authType, display, username, scopes string
		var accessToken, refreshToken, cookiePath, updated string
		var enabled, active int
		var expires sql.NullString
		if err := rows.Scan(&id, &provider, &accountID, &accountKey, &authType, &display, &username, &scopes,
			&accessToken, &refreshToken, &cookiePath, &enabled, &active, &expires, &updated); err != nil {
			rows.Close()
			return err
		}
		if accountKey == "" {
			continue
		}
		var taken int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM social_connections WHERE provider = ? AND account_key = ?`, provider, accountKey).Scan(&taken); err != nil {
			rows.Close()
			return err
		}
		if taken > 0 {
			continue
		}
		if active != 0 {
			var activeCount int
			if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM social_connections WHERE provider = ? AND active = 1`, provider).Scan(&activeCount); err != nil {
				rows.Close()
				return err
			}
			if activeCount > 0 {
				active = 0
			}
		}
		if id == "" || importRowIDExists(ctx, tx, "social_connections", id) {
			id = uuid.NewString()
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO social_connections (
  id, provider, account_id, account_key, auth_type, display_name, username, scopes,
  access_token, refresh_token, cookie_path, enabled, active, expires_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, provider, accountID, accountKey, authType, display, username, scopes,
			accessToken, refreshToken, cookiePath, enabled, active, nullString(expires), updated); err != nil {
			rows.Close()
			return err
		}
		summary.SocialConnectionsAdded++
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	exists, err = importTableExists(ctx, src, "social_oauth_configs")
	if err != nil || !exists {
		return err
	}
	rows, err = src.QueryContext(ctx, `
SELECT provider, client_id, client_secret, public_base_url, updated_at
FROM social_oauth_configs`)
	if err != nil {
		addImportWarning(summary, "social OAuth configs were not imported: %v", err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var provider, clientID, clientSecret, baseURL, updated string
		if err := rows.Scan(&provider, &clientID, &clientSecret, &baseURL, &updated); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO social_oauth_configs (provider, client_id, client_secret, public_base_url, updated_at)
VALUES (?, ?, ?, ?, ?)`, provider, clientID, clientSecret, baseURL, updated)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			summary.SocialOAuthConfigsAdded++
		}
	}
	return rows.Err()
}

func remapTimelineIDs(raw string, idMap map[string]string) string {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	remapNodeRefs(value, idMap)
	out, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(out)
}

func remapNodeRefs(value any, idMap map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "sourceNodeId" || key == "source_node_id" {
				if id, ok := item.(string); ok {
					if mapped, found := idMap[id]; found {
						v[key] = mapped
					}
				}
				continue
			}
			remapNodeRefs(item, idMap)
		}
	case []any:
		for _, item := range v {
			remapNodeRefs(item, idMap)
		}
	}
}

func importRowIDExists(ctx context.Context, tx *sql.Tx, table, id string) bool {
	var n int
	return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n) == nil && n > 0
}

func addImportWarning(summary *domain.ImportSummary, format string, args ...any) {
	if len(summary.Warnings) >= 20 {
		return
	}
	summary.Warnings = append(summary.Warnings, fmt.Sprintf(format, args...))
}
