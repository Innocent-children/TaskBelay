package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type AnalysisIndex struct {
	ID, ProjectID, Tree, Variant, ParentID string
	Data                                   json.RawMessage
}
type IndexRow struct{ Kind, Key, Member, Data string }

func (s *Store) FindIndex(ctx context.Context, project, tree, variant string) (AnalysisIndex, bool, error) {
	var out AnalysisIndex
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		query := "SELECT id,project_id,tree,variant,parent_id,data FROM analysis_indexes_v12 WHERE project_id=? AND variant=?"
		args := []any{project, variant}
		if tree != "" {
			query += " AND tree=?"
			args = append(args, tree)
		}
		query += " ORDER BY rowid DESC LIMIT 1"
		var raw string
		err := q.QueryRowContext(ctx, query, args...).Scan(&out.ID, &out.ProjectID, &out.Tree, &out.Variant, &out.ParentID, &raw)
		out.Data = json.RawMessage(raw)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	return out, err == nil, err
}

/**
 * 行按内容版本不可变；空正文遮蔽父索引条目。根记录最后发布，未完成的构建不会被读取。
 */
func (s *Store) SaveIndexRows(ctx context.Context, id string, rows []IndexRow) error {
	if len(rows) == 0 {
		return ctx.Err()
	}
	args := make([]any, 0, 5*len(rows))
	for _, r := range rows {
		args = append(args, id, r.Kind, r.Key, r.Member, r.Data)
	}
	query := "INSERT INTO analysis_index_rows_v12 VALUES " + strings.TrimSuffix(strings.Repeat("(?,?,?,?,?),", len(rows)), ",") + " ON CONFLICT(index_id,kind,key,member) DO NOTHING"
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}
func (s *Store) PublishIndex(ctx context.Context, r AnalysisIndex) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO analysis_indexes_v12 VALUES(?,?,?,?,?,?) ON CONFLICT DO NOTHING", r.ID, r.ProjectID, r.Tree, r.Variant, r.ParentID, string(r.Data))
		return err
	})
}

const indexLineage = `WITH RECURSIVE lineage(id,depth) AS (
 SELECT ?,0 UNION ALL SELECT i.parent_id,l.depth+1 FROM analysis_indexes_v12 i JOIN lineage l ON i.id=l.id WHERE i.parent_id<>''
) `

func (s *Store) IndexRows(ctx context.Context, id, kind, key string) ([]IndexRow, error) {
	var out []IndexRow
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		args := []any{id, kind, key}
		query := indexLineage + `SELECT kind,key,member,data FROM (
 SELECT r.*,row_number() OVER(PARTITION BY r.key,r.member ORDER BY l.depth) AS rank
 FROM lineage l CROSS JOIN analysis_index_rows_v12 r WHERE r.index_id=l.id AND r.kind=? AND r.key=?) WHERE rank=1 AND data<>'' ORDER BY key,member`
		rows, err := q.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r IndexRow
			if err := rows.Scan(&r.Kind, &r.Key, &r.Member, &r.Data); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

/**
 * 统计重算按主键分页读取不可变版本，在事务外合并遮蔽关系。
 * 不在每页对全部剩余记录排序；正文达到批量目标时提前结束当前页。
 */
func (s *Store) IndexPrefixRows(ctx context.Context, id, kind, prefix string, pageRows, pageBytes int) ([]IndexRow, error) {
	var lineage []string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, indexLineage+`SELECT id FROM lineage ORDER BY depth`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			lineage = append(lineage, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	var out []IndexRow
	seen := map[[2]string]bool{}
	for _, layer := range lineage {
		var last *IndexRow
		for {
			var batch []IndexRow
			err := s.read(ctx, func(ctx context.Context, q queryer) error {
				query := `SELECT kind,key,member,data FROM analysis_index_rows_v12 WHERE index_id=? AND kind=? AND key<?`
				args := []any{layer, kind, prefix + "\U0010ffff"}
				if last != nil {
					query += ` AND (key,member)>(?,?)`
					args = append(args, last.Key, last.Member)
				} else {
					query += ` AND key>=?`
					args = append(args, prefix)
				}
				query += ` ORDER BY key,member LIMIT ?`
				args = append(args, pageRows)
				rows, err := q.QueryContext(ctx, query, args...)
				if err != nil {
					return err
				}
				defer rows.Close()
				bytes := 0
				for rows.Next() {
					var row IndexRow
					if err := rows.Scan(&row.Kind, &row.Key, &row.Member, &row.Data); err != nil {
						return err
					}
					batch = append(batch, row)
					bytes += len(row.Key) + len(row.Member) + len(row.Data)
					if bytes >= pageBytes {
						break
					}
				}
				return rows.Err()
			})
			if err != nil {
				return nil, fmt.Errorf("读取索引摘要 %s/%s: %w", layer, kind, err)
			}
			if len(batch) == 0 {
				break
			}
			last = &batch[len(batch)-1]
			for _, row := range batch {
				key := [2]string{row.Key, row.Member}
				if !seen[key] && row.Data != "" {
					out = append(out, row)
				}
				seen[key] = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key || out[i].Key == out[j].Key && out[i].Member < out[j].Member
	})
	return out, nil
}

func (s *Store) SaveIndexText(ctx context.Context, id, path, body string) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_text_paths_v12(index_id,path) VALUES(?,?) ON CONFLICT DO NOTHING`, id, path); err != nil {
			return err
		}
		var rowID int64
		if err := tx.QueryRowContext(ctx, `SELECT row_id FROM analysis_text_paths_v12 WHERE index_id=? AND path=?`, id, path).Scan(&rowID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_text_v12 WHERE rowid=?)`, rowID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO analysis_text_v12(rowid,index_id,path,body) VALUES(?,?,?,?)", rowID, id, path, body)
		return err
	})
}

func (s *Store) IndexTextMatches(ctx context.Context, id, pattern string) ([]string, error) {
	var out []string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, indexLineage+`SELECT DISTINCT t.path FROM analysis_text_v12 t JOIN lineage l ON t.index_id=l.id WHERE body MATCH ?`, id, `"`+strings.ReplaceAll(pattern, `"`, `""`)+`"`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}
