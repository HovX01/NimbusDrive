package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(databaseURL string) (*Store, error) {
	path := strings.TrimPrefix(databaseURL, "sqlite://")
	if path == databaseURL {
		path = databaseURL
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  parent_id TEXT REFERENCES nodes(id),
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('file','folder')),
  mime_type TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL CHECK(status IN ('ready','pending','deleted')),
  channel_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id) WHERE status != 'deleted';
CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_parent_name
  ON nodes(parent_id, name) WHERE status != 'deleted' AND parent_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS file_parts (
  id TEXT PRIMARY KEY,
  file_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  part_no INTEGER NOT NULL,
  message_id INTEGER NOT NULL,
  size INTEGER NOT NULL,
  channel_id INTEGER NOT NULL,
  UNIQUE(file_id, part_no)
);
CREATE INDEX IF NOT EXISTS idx_parts_file ON file_parts(file_id);

CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
  name,
  path,
  id UNINDEXED,
  tokenize = 'unicode61 remove_diacritics 2'
);
`)
	if err != nil {
		return err
	}
	if err := s.migrateColumns(); err != nil {
		return err
	}
	if err := s.migrateShare(); err != nil {
		return err
	}
	return s.rebuildFTS(context.Background())
}

func (s *Store) migrateColumns() error {
	_, _ = s.db.Exec(`ALTER TABLE nodes ADD COLUMN deleted_at TEXT`)
	return nil
}

func (s *Store) rebuildFTS(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM nodes_fts`); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id FROM nodes WHERE status = 'ready' AND id != 'root'`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.upsertFTS(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pathForNode(ctx context.Context, id string) (crumbs []domain.PathCrumb, pathText string, err error) {
	var chain []domain.PathCrumb
	cur := id
	for i := 0; i < 64; i++ {
		n, getErr := s.Get(ctx, cur)
		if getErr != nil {
			return nil, "", getErr
		}
		name := n.Name
		if n.ID == "root" {
			name = "My Drive"
		}
		chain = append(chain, domain.PathCrumb{ID: n.ID, Name: name})
		if n.ParentID == nil || *n.ParentID == "" {
			break
		}
		cur = *n.ParentID
	}
	// reverse to root → leaf
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	parts := make([]string, len(chain))
	for i, c := range chain {
		parts[i] = c.Name
	}
	return chain, strings.Join(parts, " / "), nil
}

func (s *Store) upsertFTS(ctx context.Context, id string) error {
	if id == "" || id == "root" {
		return nil
	}
	n, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if n.Status != domain.StatusReady {
		return s.removeFTS(ctx, id)
	}
	_, pathText, err := s.pathForNode(ctx, id)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM nodes_fts WHERE id = ?`, id)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO nodes_fts(name, path, id) VALUES (?, ?, ?)`, n.Name, pathText, id)
	return err
}

func (s *Store) removeFTS(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM nodes_fts WHERE id = ?`, id)
	return err
}

// BuildFTSQuery turns user text into a safe FTS5 MATCH expression (prefix on last term).
func BuildFTSQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		var b strings.Builder
		for _, r := range f {
			switch r {
			case '"', '*', '(', ')', ':', '^':
				continue
			default:
				b.WriteRune(r)
			}
		}
		term := b.String()
		if term == "" {
			continue
		}
		if i == len(fields)-1 {
			term += "*"
		}
		out = append(out, term)
	}
	return strings.Join(out, " AND ")
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]domain.SearchHit, error) {
	q := BuildFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT n.id, n.parent_id, n.name, n.type, n.mime_type, n.size, n.status, n.channel_id, n.deleted_at, n.created_at, n.updated_at,
       bm25(nodes_fts) AS rank
FROM nodes_fts
JOIN nodes n ON n.id = nodes_fts.id
WHERE nodes_fts MATCH ? AND n.status = 'ready'
ORDER BY rank
LIMIT ?`, q, limit)
	if err != nil {
		return nil, err
	}

	type row struct {
		node domain.Node
		rank float64
	}
	var pending []row
	for rows.Next() {
		var n domain.Node
		var parent sql.NullString
		var deleted sql.NullString
		var created, updated string
		var rank float64
		if err := rows.Scan(
			&n.ID, &parent, &n.Name, &n.Type, &n.MimeType, &n.Size, &n.Status, &n.ChannelID, &deleted, &created, &updated, &rank,
		); err != nil {
			rows.Close()
			return nil, err
		}
		if parent.Valid {
			n.ParentID = &parent.String
		}
		if deleted.Valid {
			t, _ := time.Parse(time.RFC3339Nano, deleted.String)
			n.DeletedAt = &t
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		pending = append(pending, row{node: n, rank: rank})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(pending))
	for _, p := range pending {
		path, _, err := s.pathForNode(ctx, p.node.ID)
		if err != nil {
			return nil, err
		}
		hits = append(hits, domain.SearchHit{Node: p.node, Path: path, Rank: p.rank})
	}
	return hits, nil
}

func (s *Store) EnsureRoot(ctx context.Context) (domain.Node, error) {
	const rootID = "root"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO nodes (id, parent_id, name, type, mime_type, size, status, channel_id, created_at, updated_at)
VALUES (?, NULL, 'Root', 'folder', 'inode/directory', 0, 'ready', 0, ?, ?)
ON CONFLICT(id) DO NOTHING
`, rootID, now, now)
	if err != nil {
		return domain.Node{}, err
	}
	return s.Get(ctx, rootID)
}

func (s *Store) Get(ctx context.Context, id string) (domain.Node, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, parent_id, name, type, mime_type, size, status, channel_id, deleted_at, created_at, updated_at
FROM nodes WHERE id = ? AND status != 'deleted'`, id)
	return scanNode(row)
}

func (s *Store) GetDeleted(ctx context.Context, id string) (domain.Node, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, parent_id, name, type, mime_type, size, status, channel_id, deleted_at, created_at, updated_at
FROM nodes WHERE id = ? AND status = 'deleted'`, id)
	return scanNode(row)
}

func (s *Store) ListChildren(ctx context.Context, parentID string) ([]domain.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, parent_id, name, type, mime_type, size, status, channel_id, deleted_at, created_at, updated_at
FROM nodes WHERE parent_id = ? AND status = 'ready' ORDER BY type DESC, name COLLATE NOCASE`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Create(ctx context.Context, node domain.Node) (domain.Node, error) {
	if node.ID == "" {
		node.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO nodes (id, parent_id, name, type, mime_type, size, status, channel_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, node.ParentID, node.Name, node.Type, node.MimeType, node.Size, node.Status, node.ChannelID,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.Node{}, domain.ErrConflict
		}
		return domain.Node{}, err
	}
	if node.Status == domain.StatusReady {
		_ = s.upsertFTS(ctx, node.ID)
	}
	return node, nil
}

func (s *Store) UpdateStatus(ctx context.Context, id string, status domain.NodeStatus) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE nodes SET status = ?, updated_at = ? WHERE id = ? AND status != 'deleted'`,
		status, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	switch status {
	case domain.StatusReady:
		_ = s.upsertFTS(ctx, id)
	case domain.StatusDeleted:
		_ = s.removeFTS(ctx, id)
	}
	return nil
}

func (s *Store) UpdateSize(ctx context.Context, id string, size int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE nodes SET size = ?, updated_at = ? WHERE id = ? AND status != 'deleted'`,
		size, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) Rename(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE nodes SET name = ?, updated_at = ? WHERE id = ? AND status != 'deleted'`,
		name, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	node, err := s.Get(ctx, id)
	if err != nil {
		return nil
	}
	if node.Type == domain.NodeFolder {
		_ = s.rebuildFTS(ctx)
	} else {
		_ = s.upsertFTS(ctx, id)
	}
	return nil
}

func (s *Store) Move(ctx context.Context, id, parentID string) error {
	if id == "root" {
		return domain.ErrValidation
	}
	if parentID == "" {
		parentID = "root"
	}
	node, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	curParent := "root"
	if node.ParentID != nil && *node.ParentID != "" {
		curParent = *node.ParentID
	}
	if curParent == parentID {
		return nil
	}
	if id == parentID {
		return fmt.Errorf("%w: cannot move into itself", domain.ErrValidation)
	}
	parent, err := s.Get(ctx, parentID)
	if err != nil {
		return err
	}
	if parent.Type != domain.NodeFolder {
		return fmt.Errorf("%w: destination is not a folder", domain.ErrValidation)
	}
	if node.Type == domain.NodeFolder {
		subtree, err := s.folderSubtreeIDs(ctx, id)
		if err != nil {
			return err
		}
		if subtree[parentID] {
			return fmt.Errorf("%w: cannot move folder into itself or a subfolder", domain.ErrValidation)
		}
	}
	if existing, err := s.FindChildByName(ctx, parentID, node.Name); err == nil && existing.ID != id {
		return domain.ErrConflict
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `
UPDATE nodes SET parent_id = ?, updated_at = ? WHERE id = ? AND status != 'deleted'`,
		nullIfRoot(parentID), now, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrConflict
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if node.Type == domain.NodeFolder {
		return s.rebuildFTS(ctx)
	}
	return s.upsertFTS(ctx, id)
}

func nullIfRoot(parentID string) any {
	if parentID == "root" {
		return "root"
	}
	return parentID
}

func (s *Store) folderSubtreeIDs(ctx context.Context, folderID string) (map[string]bool, error) {
	out := map[string]bool{folderID: true}
	queue := []string{folderID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		children, err := s.ListChildren(ctx, cur)
		if err != nil {
			return nil, err
		}
		for _, c := range children {
			out[c.ID] = true
			if c.Type == domain.NodeFolder {
				queue = append(queue, c.ID)
			}
		}
	}
	return out, nil
}

func (s *Store) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `
UPDATE nodes SET status = 'deleted', deleted_at = ?, updated_at = ? WHERE id = ? AND status != 'deleted'`,
		now, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	_ = s.removeFTS(ctx, id)
	return nil
}

func (s *Store) ListTrashRoots(ctx context.Context, limit int) ([]domain.Node, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT n.id, n.parent_id, n.name, n.type, n.mime_type, n.size, n.status, n.channel_id, n.deleted_at, n.created_at, n.updated_at
FROM nodes n
WHERE n.status = 'deleted' AND n.id != 'root'
  AND (n.parent_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM nodes p WHERE p.id = n.parent_id AND p.status = 'deleted'
  ))
ORDER BY COALESCE(n.deleted_at, n.updated_at) DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListDeletedChildren(ctx context.Context, parentID string) ([]domain.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, parent_id, name, type, mime_type, size, status, channel_id, deleted_at, created_at, updated_at
FROM nodes WHERE parent_id = ? AND status = 'deleted' ORDER BY type DESC, name COLLATE NOCASE`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) HardDelete(ctx context.Context, id string) error {
	if id == "root" {
		return domain.ErrValidation
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	_ = s.removeFTS(ctx, id)
	return nil
}

func (s *Store) FindChildByName(ctx context.Context, parentID, name string) (domain.Node, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, parent_id, name, type, mime_type, size, status, channel_id, deleted_at, created_at, updated_at
FROM nodes WHERE parent_id = ? AND name = ? AND status != 'deleted'`, parentID, name)
	return scanNode(row)
}

func (s *Store) ReplaceParts(ctx context.Context, fileID string, parts []domain.FilePart) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM file_parts WHERE file_id = ?`, fileID); err != nil {
		return err
	}
	for _, p := range parts {
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO file_parts (id, file_id, part_no, message_id, size, channel_id)
VALUES (?, ?, ?, ?, ?, ?)`, p.ID, fileID, p.PartNo, p.MessageID, p.Size, p.ChannelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListByFile(ctx context.Context, fileID string) ([]domain.FilePart, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, file_id, part_no, message_id, size, channel_id
FROM file_parts WHERE file_id = ? ORDER BY part_no`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FilePart
	for rows.Next() {
		var p domain.FilePart
		if err := rows.Scan(&p.ID, &p.FileID, &p.PartNo, &p.MessageID, &p.Size, &p.ChannelID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteByFile(ctx context.Context, fileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM file_parts WHERE file_id = ?`, fileID)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNode(row scanner) (domain.Node, error) {
	var n domain.Node
	var parent sql.NullString
	var deleted sql.NullString
	var created, updated string
	err := row.Scan(&n.ID, &parent, &n.Name, &n.Type, &n.MimeType, &n.Size, &n.Status, &n.ChannelID, &deleted, &created, &updated)
	if err == sql.ErrNoRows {
		return domain.Node{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Node{}, err
	}
	if parent.Valid {
		n.ParentID = &parent.String
	}
	if deleted.Valid {
		t, _ := time.Parse(time.RFC3339Nano, deleted.String)
		n.DeletedAt = &t
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	n.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return n, nil
}

// Ensure Store implements ports.
var (
	_ domain.NodeRepository = (*Store)(nil)
	_ domain.PartRepository = (*Store)(nil)
)

func (s *Store) String() string { return fmt.Sprintf("sqlite.Store(%p)", s) }
