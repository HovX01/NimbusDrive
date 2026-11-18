package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/vrc/nimbus/internal/domain"
)

const searchIndexReady = `SELECT EXISTS(SELECT 1 FROM nodes_fts_state WHERE id = 1 AND version = 1)`

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
  NOT EXISTS(SELECT 1 FROM nodes_fts_state WHERE id = 1 AND version IN (0, 1))
  OR EXISTS(SELECT 1 FROM nodes_fts_dirty WHERE id = 'root')`).Scan(&reset); err != nil {
		return false, err
	}
	if reset {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM nodes_fts;
DELETE FROM nodes_fts_dirty;
DELETE FROM nodes_fts_backfill;
INSERT INTO nodes_fts_backfill(id) SELECT id FROM nodes WHERE status = 'ready' AND id != 'root';
INSERT OR REPLACE INTO nodes_fts_state(id, version) VALUES (1, 0);`); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}

	rows, err := tx.QueryContext(ctx, `
SELECT id, NOT EXISTS(SELECT 1 FROM nodes_fts_backfill b WHERE b.id = work.id)
FROM (
  SELECT id FROM nodes_fts_dirty
  UNION ALL
  SELECT id FROM nodes_fts_backfill WHERE NOT EXISTS(SELECT 1 FROM nodes_fts_dirty d WHERE d.id = nodes_fts_backfill.id)
  LIMIT 128
) work`)
	if err != nil {
		return false, err
	}
	type entry struct {
		id        string
		hasOldRow bool
	}
	var batch []entry
	var stale []any
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.hasOldRow); err != nil {
			rows.Close()
			return false, err
		}
		batch = append(batch, e)
		if e.hasOldRow {
			stale = append(stale, e.id)
		}
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(batch) == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE nodes_fts_state SET version = 1 WHERE id = 1 AND version = 0`); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}

	// The FTS id column is unindexed, so scan it at most once per repair batch.
	if len(stale) != 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts WHERE id IN (?`+strings.Repeat(",?", len(stale)-1)+")", stale...); err != nil {
			return false, err
		}
	}
	for _, e := range batch {
		n, err := getNode(ctx, tx, e.id)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return false, err
		}
		if err == nil && n.Status == domain.StatusReady {
			if err := insertFTS(ctx, tx, n); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return false, err
			}
		}
		if err := clearFTSWork(ctx, tx, e.id); err != nil {
			return false, err
		}
	}
	return false, tx.Commit()
}

func insertFTS(ctx context.Context, tx *sql.Tx, node domain.Node) error {
	_, path, err := pathForNode(ctx, tx, node.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO nodes_fts(name, path, id) VALUES (?, ?, ?)`, node.Name, path, node.ID)
	return err
}

func clearFTSWork(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts_backfill WHERE id = ?`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM nodes_fts_dirty WHERE id = ?`, id)
	return err
}
