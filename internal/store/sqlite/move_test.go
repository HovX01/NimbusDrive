package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vrc/nimbus/internal/domain"
)

func TestMoveNode(t *testing.T) {
	dir := t.TempDir()
	st, err := Open("sqlite://" + filepath.Join(dir, "move.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := st.Create(ctx, domain.Node{
		ParentID: &root.ID,
		Name:     "Docs",
		Type:     domain.NodeFolder,
		MimeType: "inode/directory",
		Status:   domain.StatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := st.Create(ctx, domain.Node{
		ParentID: &root.ID,
		Name:     "note.txt",
		Type:     domain.NodeFile,
		MimeType: "text/plain",
		Status:   domain.StatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Move(ctx, file.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	moved, err := st.Get(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ParentID == nil || *moved.ParentID != folder.ID {
		t.Fatalf("parent=%v want %s", moved.ParentID, folder.ID)
	}
}

func TestMoveFolderIntoSelfFails(t *testing.T) {
	dir := t.TempDir()
	st, err := Open("sqlite://" + filepath.Join(dir, "cycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := st.Create(ctx, domain.Node{
		ParentID: &root.ID,
		Name:     "Parent",
		Type:     domain.NodeFolder,
		Status:   domain.StatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.Create(ctx, domain.Node{
		ParentID: &folder.ID,
		Name:     "Child",
		Type:     domain.NodeFolder,
		Status:   domain.StatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Move(ctx, folder.ID, child.ID); err == nil {
		t.Fatal("expected cycle error")
	}
}
