package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vrc/nimbus/internal/domain"
)

func (s *Store) migrateEditor() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS edit_projects (
  id TEXT PRIMARY KEY,
  source_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  timeline_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_edit_projects_source ON edit_projects(source_node_id);
`)
	return err
}

func (s *Store) CreateProject(ctx context.Context, p domain.EditProject) (domain.EditProject, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO edit_projects (id, source_node_id, name, timeline_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.SourceNodeID, p.Name, p.TimelineJSON, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.EditProject{}, domain.ErrConflict
		}
		return domain.EditProject{}, err
	}
	return p, nil
}

func (s *Store) GetProject(ctx context.Context, id string) (domain.EditProject, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, source_node_id, name, timeline_json, created_at, updated_at
FROM edit_projects WHERE id = ?`, id)
	return scanEditProject(row)
}

func (s *Store) GetProjectBySource(ctx context.Context, sourceNodeID string) (domain.EditProject, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, source_node_id, name, timeline_json, created_at, updated_at
FROM edit_projects WHERE source_node_id = ?
ORDER BY updated_at DESC LIMIT 1`, sourceNodeID)
	return scanEditProject(row)
}

func (s *Store) ListProjects(ctx context.Context) ([]domain.EditProject, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_node_id, name, timeline_json, created_at, updated_at
FROM edit_projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EditProject
	for rows.Next() {
		p, err := scanEditProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTimeline(ctx context.Context, id string, timelineJSON string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE edit_projects SET timeline_json = ?, updated_at = ? WHERE id = ?`,
		timelineJSON, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM edit_projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanEditProject(row scanner) (domain.EditProject, error) {
	var p domain.EditProject
	var created, updated string
	err := row.Scan(&p.ID, &p.SourceNodeID, &p.Name, &p.TimelineJSON, &created, &updated)
	if err == sql.ErrNoRows {
		return domain.EditProject{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.EditProject{}, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return p, nil
}
