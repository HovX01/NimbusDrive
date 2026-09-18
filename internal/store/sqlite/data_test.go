package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vrc/nimbus/internal/domain"
)

func TestDataCRUD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st, err := Open("sqlite://" + filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	row, err := st.Insert(ctx, "products", map[string]any{"name": "Shirt", "price": 29.99})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRow(ctx, "products", row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data["name"] != "Shirt" {
		t.Fatalf("got %+v", got.Data)
	}

	rows, err := st.Query(ctx, "products", domain.DataQuery{
		Filters: []domain.DataFilter{{Field: "name", Op: "eq", Value: "Shirt"}},
		Limit:   10,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("query: %d rows err=%v", len(rows), err)
	}

	updated, err := st.Update(ctx, "products", row.ID, map[string]any{"price": 39})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Data["price"] != 39 && updated.Data["price"] != 39.0 && updated.Data["price"] != int(39) {
		t.Fatalf("price not updated: %v (%T)", updated.Data["price"], updated.Data["price"])
	}

	if err := st.Delete(ctx, "products", row.ID); err != nil {
		t.Fatal(err)
	}
}