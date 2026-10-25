package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	target, err := Open(filepath.Join(dir, "target.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if _, err := target.EnsureRoot(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := target.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('docs','root','Docs','folder','inode/directory',0,'ready',0,?,?)`, now, now)
	exec(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('current-keep','root','keep.txt','file','text/plain',3,'ready',7,?,?)`, now, now)
	exec(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('p-current-keep','current-keep',0,900,3,7)`)
	exec(`INSERT INTO file_hashes (hash,node_id) VALUES ('hash-current-keep','current-keep')`)
	exec(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('existing-dup','root','existing.bin','file','application/octet-stream',5,'ready',7,?,?)`, now, now)
	exec(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('p-existing-dup','existing-dup',0,901,5,7)`)
	exec(`INSERT INTO file_hashes (hash,node_id) VALUES ('hash-duplicate','existing-dup')`)

	source, err := Open(filepath.Join(dir, "source.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "source.sqlite")
	if _, err := source.EnsureRoot(ctx); err != nil {
		t.Fatal(err)
	}
	execSrc := func(q string, args ...any) {
		t.Helper()
		if _, err := source.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	execSrc(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('src-docs','root','Docs','folder','inode/directory',0,'ready',0,?,?)`, now, now)
	execSrc(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('src-old','root','Old','folder','inode/directory',0,'ready',0,?,?)`, now, now)
	execSrc(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('src-old-file','src-old','old.bin','file','application/octet-stream',10,'ready',7,?,?)`, now, now)
	execSrc(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('sp1','src-old-file',0,100,4,7)`)
	execSrc(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('sp2','src-old-file',1,101,6,7)`)
	execSrc(`INSERT INTO file_hashes (hash,node_id) VALUES ('hash-old','src-old-file')`)
	execSrc(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('src-dup','root','duplicate.bin','file','application/octet-stream',5,'ready',7,?,?)`, now, now)
	execSrc(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('sp-dup','src-dup',0,102,5,7)`)
	execSrc(`INSERT INTO file_hashes (hash,node_id) VALUES ('hash-duplicate','src-dup')`)
	execSrc(`INSERT INTO nodes (id,parent_id,name,type,mime_type,size,status,channel_id,created_at,updated_at)
VALUES ('src-keep','root','keep.txt','file','text/plain',3,'ready',7,?,?)`, now, now)
	execSrc(`INSERT INTO file_parts (id,file_id,part_no,message_id,size,channel_id) VALUES ('sp-keep','src-keep',0,103,3,7)`)
	execSrc(`INSERT INTO file_hashes (hash,node_id) VALUES ('hash-imported-keep','src-keep')`)
	execSrc(`INSERT INTO share_links (id,token,node_id,created_at,expires_at,revoked_at,download_count)
VALUES ('share-old','tok-old','src-old-file',?,NULL,NULL,2)`, now)
	execSrc(`INSERT INTO edit_projects (id,source_node_id,name,timeline_json,created_at,updated_at)
VALUES ('project-old','src-old-file','Old edit','{}',?,?)`, now, now)
	execSrc(`INSERT INTO data_rows (id,collection,data,created_at,updated_at) VALUES ('row-old','notes','{"text":"hello"}',?,?)`, now, now)
	execSrc(`INSERT INTO bot_grants (bot_id,allowed,updated_at) VALUES ('123',1,?)`, now)
	execSrc(`INSERT INTO social_connections (id,provider,account_id,account_key,auth_type,display_name,username,scopes,access_token,refresh_token,cookie_path,enabled,active,expires_at,updated_at)
VALUES ('social-old','tiktok','acct','key','oauth','Old','olduser','user.info','access','refresh','',1,1,NULL,?)`, now)
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	summary, err := target.MergeDatabase(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FoldersMerged != 1 || summary.FoldersCreated != 1 {
		t.Fatalf("folders summary = %+v", summary)
	}
	if summary.FilesImported != 2 || summary.FilesSkipped != 1 || summary.FilesRenamed != 1 {
		t.Fatalf("files summary = %+v", summary)
	}
	if summary.PartsImported != 3 || summary.SharesImported != 1 || summary.EditProjectsImported != 1 {
		t.Fatalf("related summary = %+v", summary)
	}
	if summary.DataRowsImported != 1 || summary.BotGrantsImported != 1 || summary.SocialConnectionsAdded != 1 {
		t.Fatalf("app data summary = %+v", summary)
	}

	var docs int
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE parent_id='root' AND name='Docs'`).Scan(&docs); err != nil {
		t.Fatal(err)
	}
	if docs != 1 {
		t.Fatalf("Docs count = %d", docs)
	}
	var oldID string
	if err := target.db.QueryRowContext(ctx, `SELECT id FROM nodes WHERE parent_id='root' AND name='Old'`).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	var parts int
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_parts WHERE file_id=?`, oldID).Scan(&parts); err != nil {
		t.Fatal(err)
	}
	var oldFileID string
	if err := target.db.QueryRowContext(ctx, `SELECT id FROM nodes WHERE parent_id=? AND name='old.bin'`, oldID).Scan(&oldFileID); err != nil {
		t.Fatal(err)
	}
	if parts != 0 {
		t.Fatalf("parts attached to folder = %d", parts)
	}
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_parts WHERE file_id=?`, oldFileID).Scan(&parts); err != nil {
		t.Fatal(err)
	}
	if parts != 2 {
		t.Fatalf("old.bin parts = %d", parts)
	}
	var keepCount int
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE parent_id='root' AND name='keep (imported).txt'`).Scan(&keepCount); err != nil {
		t.Fatal(err)
	}
	if keepCount != 1 {
		t.Fatalf("renamed keep count = %d", keepCount)
	}
	var dupCount int
	if err := target.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE name='duplicate.bin'`).Scan(&dupCount); err != nil {
		t.Fatal(err)
	}
	if dupCount != 0 {
		t.Fatalf("duplicate file was imported")
	}

	second, err := target.MergeDatabase(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if second.FilesImported != 0 || second.FilesSkipped != 3 {
		t.Fatalf("second import summary = %+v", second)
	}
	if second.SharesImported != 0 || second.EditProjectsImported != 0 {
		t.Fatalf("second import duplicated related rows = %+v", second)
	}
}
