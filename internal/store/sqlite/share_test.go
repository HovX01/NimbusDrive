package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/vrc/nimbus/internal/domain"

	_ "modernc.org/sqlite"
)

func TestHardDeleteNodeWithShareLinks(t *testing.T) {
	dir := t.TempDir()
	st, err := Open("sqlite://" + filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	file, err := st.Create(ctx, domain.Node{
		ParentID: &root.ID,
		Name:     "shared.txt",
		Type:     domain.NodeFile,
		MimeType: "text/plain",
		Status:   domain.StatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDelete(ctx, file.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateShare(ctx, domain.ShareLink{
		Token:  "abc123",
		NodeID: file.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteByNode(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.HardDelete(ctx, file.ID); err != nil {
		t.Fatalf("hard delete after share cleanup: %v", err)
	}
}

func TestMigrateShareCascadeFromLegacySchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE nodes (
  id TEXT PRIMARY KEY,
  parent_id TEXT REFERENCES nodes(id),
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  channel_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE share_links (
  id TEXT PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  node_id TEXT NOT NULL REFERENCES nodes(id),
  created_at TEXT NOT NULL,
  expires_at TEXT,
  revoked_at TEXT,
  download_count INTEGER NOT NULL DEFAULT 0
);`); err != nil {
		t.Fatal(err)
	}
	st := &Store{db: db}
	t.Cleanup(func() { _ = db.Close() })
	if err := st.migrateShareCascade(); err != nil {
		t.Fatal(err)
	}
	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='share_links'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if ddl == "" || !contains(ddl, "ON DELETE CASCADE") {
		t.Fatalf("expected CASCADE FK after migration, got: %q", ddl)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
