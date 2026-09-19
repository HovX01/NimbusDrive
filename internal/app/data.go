package app

import (
	"context"
	"fmt"

	"github.com/vrc/nimbus/internal/dataquery"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Services) ListCollections(ctx context.Context) ([]string, error) {
	if s.Data == nil {
		return nil, domain.ErrNotConfigured
	}
	return s.Data.ListCollections(ctx)
}

func (s *Services) InsertData(ctx context.Context, collection string, data map[string]any) (domain.DataRow, error) {
	if s.Data == nil {
		return domain.DataRow{}, domain.ErrNotConfigured
	}
	if !dataquery.ValidCollection(collection) {
		return domain.DataRow{}, fmt.Errorf("%w: invalid collection name", domain.ErrValidation)
	}
	return s.Data.Insert(ctx, collection, data)
}

func (s *Services) GetData(ctx context.Context, collection, id string) (domain.DataRow, error) {
	if s.Data == nil {
		return domain.DataRow{}, domain.ErrNotConfigured
	}
	if !dataquery.ValidCollection(collection) {
		return domain.DataRow{}, fmt.Errorf("%w: invalid collection name", domain.ErrValidation)
	}
	return s.Data.GetRow(ctx, collection, id)
}

func (s *Services) QueryData(ctx context.Context, collection string, q domain.DataQuery) ([]domain.DataRow, error) {
	if s.Data == nil {
		return nil, domain.ErrNotConfigured
	}
	if !dataquery.ValidCollection(collection) {
		return nil, fmt.Errorf("%w: invalid collection name", domain.ErrValidation)
	}
	return s.Data.Query(ctx, collection, q)
}

func (s *Services) UpdateData(ctx context.Context, collection, id string, patch map[string]any) (domain.DataRow, error) {
	if s.Data == nil {
		return domain.DataRow{}, domain.ErrNotConfigured
	}
	if !dataquery.ValidCollection(collection) {
		return domain.DataRow{}, fmt.Errorf("%w: invalid collection name", domain.ErrValidation)
	}
	return s.Data.Update(ctx, collection, id, patch)
}

func (s *Services) DeleteData(ctx context.Context, collection, id string) error {
	if s.Data == nil {
		return domain.ErrNotConfigured
	}
	if !dataquery.ValidCollection(collection) {
		return fmt.Errorf("%w: invalid collection name", domain.ErrValidation)
	}
	return s.Data.Delete(ctx, collection, id)
}

// DataRowJSON flattens a row for API responses (Supabase-style).
func DataRowJSON(row domain.DataRow) map[string]any {
	out := map[string]any{
		"id":         row.ID,
		"collection": row.Collection,
		"created_at": row.CreatedAt,
		"updated_at": row.UpdatedAt,
	}
	for k, v := range row.Data {
		out[k] = v
	}
	return out
}

func DataRowsJSON(rows []domain.DataRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = DataRowJSON(row)
	}
	return out
}

// StorageStats returns aggregated drive consumption for the dashboard.
func (s *Services) StorageStats(ctx context.Context, days int) (domain.StorageStats, error) {
	if s.Nodes == nil {
		return domain.StorageStats{}, domain.ErrNotConfigured
	}
	return s.Nodes.StorageStats(ctx, days)
}
