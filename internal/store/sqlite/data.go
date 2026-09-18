package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Store) migrateData() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS data_rows (
  id TEXT PRIMARY KEY,
  collection TEXT NOT NULL,
  data TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_data_rows_collection ON data_rows(collection);
`)
	return err
}

func (s *Store) Insert(ctx context.Context, collection string, data map[string]any) (domain.DataRow, error) {
	if data == nil {
		data = map[string]any{}
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	raw, err := json.Marshal(data)
	if err != nil {
		return domain.DataRow{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO data_rows (id, collection, data, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		id, collection, string(raw), formatTime(now), formatTime(now))
	if err != nil {
		return domain.DataRow{}, err
	}
	return domain.DataRow{
		ID:         id,
		Collection: collection,
		Data:       data,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (s *Store) GetRow(ctx context.Context, collection, id string) (domain.DataRow, error) {
	var raw, created, updated string
	err := s.db.QueryRowContext(ctx, `
SELECT data, created_at, updated_at FROM data_rows WHERE collection = ? AND id = ?`,
		collection, id).Scan(&raw, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DataRow{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DataRow{}, err
	}
	data, err := decodeData(raw)
	if err != nil {
		return domain.DataRow{}, err
	}
	return domain.DataRow{
		ID:         id,
		Collection: collection,
		Data:       data,
		CreatedAt:  parseTime(created),
		UpdatedAt:  parseTime(updated),
	}, nil
}

func (s *Store) Query(ctx context.Context, collection string, q domain.DataQuery) ([]domain.DataRow, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	var (
		args   []any
		where  = []string{"collection = ?"}
	)
	args = append(args, collection)

	for _, f := range q.Filters {
		expr, val, err := filterExpr(f)
		if err != nil {
			return nil, err
		}
		where = append(where, expr)
		args = append(args, val...)
	}

	order := "created_at DESC"
	if q.OrderBy != "" {
		col, err := orderExpr(q.OrderBy)
		if err != nil {
			return nil, err
		}
		dir := "ASC"
		if q.Desc {
			dir = "DESC"
		}
		order = col + " " + dir
	}

	query := fmt.Sprintf(`
SELECT id, data, created_at, updated_at FROM data_rows
WHERE %s
ORDER BY %s
LIMIT ? OFFSET ?`, strings.Join(where, " AND "), order)
	args = append(args, limit, q.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.DataRow
	for rows.Next() {
		var id, raw, created, updated string
		if err := rows.Scan(&id, &raw, &created, &updated); err != nil {
			return nil, err
		}
		data, err := decodeData(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.DataRow{
			ID:         id,
			Collection: collection,
			Data:       data,
			CreatedAt:  parseTime(created),
			UpdatedAt:  parseTime(updated),
		})
	}
	return out, rows.Err()
}

func (s *Store) Update(ctx context.Context, collection, id string, patch map[string]any) (domain.DataRow, error) {
	row, err := s.GetRow(ctx, collection, id)
	if err != nil {
		return domain.DataRow{}, err
	}
	for k, v := range patch {
		if k == "id" || k == "collection" || k == "created_at" || k == "updated_at" {
			continue
		}
		row.Data[k] = v
	}
	now := time.Now().UTC()
	raw, err := json.Marshal(row.Data)
	if err != nil {
		return domain.DataRow{}, err
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE data_rows SET data = ?, updated_at = ? WHERE collection = ? AND id = ?`,
		string(raw), formatTime(now), collection, id)
	if err != nil {
		return domain.DataRow{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.DataRow{}, domain.ErrNotFound
	}
	row.UpdatedAt = now
	return row, nil
}

func (s *Store) Delete(ctx context.Context, collection, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM data_rows WHERE collection = ? AND id = ?`, collection, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListCollections(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT collection FROM data_rows ORDER BY collection`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func filterExpr(f domain.DataFilter) (string, []any, error) {
	if f.Field == "id" || f.Field == "created_at" || f.Field == "updated_at" {
		col := f.Field
		switch f.Op {
		case "eq":
			return col + " = ?", []any{f.Value}, nil
		case "like":
			return col + " LIKE ?", []any{f.Value}, nil
		default:
			return "", nil, fmt.Errorf("%w: operator %s not supported on %s", domain.ErrValidation, f.Op, f.Field)
		}
	}
	path := "$." + f.Field
	expr := "json_extract(data, ?)"
	switch f.Op {
	case "eq":
		return expr + " = ?", []any{path, f.Value}, nil
	case "gt":
		return expr + " > ?", []any{path, f.Value}, nil
	case "gte":
		return expr + " >= ?", []any{path, f.Value}, nil
	case "lt":
		return expr + " < ?", []any{path, f.Value}, nil
	case "lte":
		return expr + " <= ?", []any{path, f.Value}, nil
	case "like":
		return expr + " LIKE ?", []any{path, f.Value}, nil
	default:
		return "", nil, fmt.Errorf("%w: unknown operator", domain.ErrValidation)
	}
}

func orderExpr(field string) (string, error) {
	switch field {
	case "id", "created_at", "updated_at":
		return field, nil
	default:
		return "json_extract(data, '$." + field + "')", nil
	}
}

func decodeData(raw string) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
