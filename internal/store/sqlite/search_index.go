package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

const searchIndexReady = `SELECT EXISTS(SELECT 1 FROM nodes_fts_state WHERE id = 1 AND version = 3)`

// PrepareSearch repairs the index in batches so API requests can use SQLite between commits.
func (s *Store) PrepareSearch(ctx context.Context) error {
	for {
		done, err := s.prepareSearchBatch(ctx)
		if done || err != nil {
			return err
		}
		t := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (s *Store) prepareSearchBatch(ctx context.Context) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var reset bool
	if err := tx.QueryRowContext(ctx, `SELECT
  NOT EXISTS(SELECT 1 FROM nodes_fts_state WHERE id = 1 AND version IN (2, 3))
  OR EXISTS(SELECT 1 FROM nodes_fts_dirty WHERE id = 'root')`).Scan(&reset); err != nil {
		return false, err
	}
	if reset {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM nodes_fts;
DELETE FROM nodes_fts_rows;
DELETE FROM nodes_fts_dirty;
DELETE FROM nodes_fts_backfill;
INSERT INTO nodes_fts_backfill(id) SELECT id FROM nodes WHERE status = 'ready' AND id != 'root';
INSERT OR REPLACE INTO nodes_fts_state(id, version) VALUES (1, 2);`); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}

	batch, err := searchWork(ctx, tx, `
SELECT id FROM nodes_fts_dirty
UNION ALL
SELECT id FROM nodes_fts_backfill WHERE NOT EXISTS(SELECT 1 FROM nodes_fts_dirty d WHERE d.id = nodes_fts_backfill.id)
LIMIT 128`)
	if err != nil {
		return false, err
	}
	if len(batch) == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE nodes_fts_state SET version = 3 WHERE id = 1 AND version = 2`); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if err := refreshFTS(ctx, tx, batch); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) reindexTree(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ready bool
	if err := tx.QueryRowContext(ctx, searchIndexReady).Scan(&ready); err != nil || !ready {
		return err
	}
	ids, err := searchWork(ctx, tx, `
WITH RECURSIVE subtree(id) AS (
  VALUES (?)
  UNION
  SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id WHERE n.status != 'deleted'
)
SELECT id FROM subtree WHERE id != 'root'`, id)
	if err != nil {
		return err
	}
	if err := refreshFTS(ctx, tx, ids); err != nil {
		return err
	}
	return tx.Commit()
}

func searchWork(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func refreshFTS(ctx context.Context, tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		if err := deleteFTS(ctx, tx, id); err != nil {
			return err
		}
		n, err := getNode(ctx, tx, id)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err == nil && n.Status == domain.StatusReady {
			if err := insertFTS(ctx, tx, n); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}
		}
		if err := clearFTSWork(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func deleteFTS(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts WHERE rowid = (SELECT rowid FROM nodes_fts_rows WHERE id = ?)`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts_rows WHERE id = ?`, id)
	return err
}

func insertFTS(ctx context.Context, tx *sql.Tx, node domain.Node) error {
	_, path, err := pathForNode(ctx, tx, node.ID)
	if err != nil {
		return err
	}
	var rowID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO nodes_fts_rows(id) VALUES (?) RETURNING rowid`, node.ID).Scan(&rowID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO nodes_fts(rowid, name, path, id) VALUES (?, ?, ?, ?)`, rowID, node.Name, path, node.ID)
	return err
}

func clearFTSWork(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts_backfill WHERE id = ?`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts_dirty WHERE id = ?`, id)
	return err
}
