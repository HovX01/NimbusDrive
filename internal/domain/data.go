package domain

import "time"

// DataRow is one document in a named collection (Supabase-style table row).
type DataRow struct {
	ID         string         `json:"id"`
	Collection string         `json:"collection"`
	Data       map[string]any   `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// DataFilter is one query predicate on a JSON field or system column.
type DataFilter struct {
	Field string
	Op    string // eq, gt, gte, lt, lte, like
	Value string
}

// DataQuery lists rows in a collection with optional filters.
type DataQuery struct {
	Filters []DataFilter
	OrderBy string // field name
	Desc    bool
	Limit   int
	Offset  int
}
