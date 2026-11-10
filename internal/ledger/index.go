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

// LatestDerivedIndex 返回在 parent 之上最近发布的索引版本。
func (s *Store) LatestDerivedIndex(ctx context.Context, project, parent string) (AnalysisIndex, bool, error) {
	var out AnalysisIndex
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var raw string
		err := q.QueryRowContext(ctx, "SELECT id,project_id,tree,variant,parent_id,data FROM analysis_indexes_v12 WHERE project_id=? AND parent_id=? AND variant<>(SELECT variant FROM analysis_indexes_v12 WHERE id=?) ORDER BY rowid DESC LIMIT 1", project, parent, parent).Scan(&out.ID, &out.ProjectID, &out.Tree, &out.Variant, &out.ParentID, &raw)
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
func (s *Store) SaveIndexRows(ctx context.Context, build IndexBuild, rows []IndexRow) error {
	if len(rows) == 0 {
		return ctx.Err()
	}
	pages, err := prepareIndexPages(ctx, rows)
	if err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := checkIndexGeneration(ctx, tx, build, false); err != nil {
			return err
		}
		return saveIndexPages(ctx, tx, build.ID, pages, false)
	})
}
func (s *Store) PublishIndex(ctx context.Context, r AnalysisIndex, generation, parentGeneration int64) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := checkIndexGeneration(ctx, tx, IndexBuild{r.ID, generation}, false); err != nil {
			return err
		}
		if r.ParentID != "" {
			if err := checkIndexGeneration(ctx, tx, IndexBuild{r.ParentID, parentGeneration}, true); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO analysis_indexes_v12 VALUES(?,?,?,?,?,?) ON CONFLICT DO NOTHING", r.ID, r.ProjectID, r.Tree, r.Variant, r.ParentID, string(r.Data))
		if err == nil {
			// 同一逻辑版本已由另一父层的构建先发布时，自己的批次已确定可以清理。
			_, err = tx.ExecContext(ctx, `UPDATE analysis_index_lives_v14 SET deleting=1 WHERE index_id=?1
AND NOT EXISTS(SELECT 1 FROM analysis_indexes_v12 WHERE id=?1)`, r.ID)
		}
		return err
	})
}

const indexLineage = `WITH RECURSIVE lineage(id,depth) AS (
 SELECT ?,0 UNION ALL SELECT i.parent_id,l.depth+1 FROM analysis_indexes_v12 i JOIN lineage l ON i.id=l.id WHERE i.parent_id<>''
) `

func (s *Store) IndexRows(ctx context.Context, id, kind, key string) ([]IndexRow, error) {
	type match struct {
		member, data, page string
		emptyMembers       bool
	}
	var matches []match
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		query := indexLineage + `SELECT member,data,page,empty_members FROM (
 SELECT l.depth,0 AS priority,0 AS seq,r.member,r.data,'' AS page,0 AS empty_members FROM lineage l CROSS JOIN analysis_index_rows_v12 r
 WHERE r.index_id=l.id AND r.kind=?2 AND r.key=?3
 UNION ALL
 SELECT l.depth,2-p.legacy,p.seq,'','',p.sha256,p.empty_members FROM lineage l CROSS JOIN analysis_index_pages_v13 p
 WHERE p.index_id=l.id AND p.kind=?2 AND p.last_key>=?3 AND p.first_key<=?3)
 ORDER BY depth,priority,seq`
		rows, err := q.QueryContext(ctx, query, id, kind, key)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m match
			if err := rows.Scan(&m.member, &m.data, &m.page, &m.emptyMembers); err != nil {
				return err
			}
			matches = append(matches, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	var out []IndexRow
	seen := map[string]bool{}
	add := func(member, data string) {
		if !seen[member] && data != "" {
			out = append(out, IndexRow{kind, key, member, data})
		}
		seen[member] = true
	}
	for _, m := range matches {
		if m.page == "" {
			add(m.member, m.data)
			continue
		}
		if m.emptyMembers && seen[""] {
			continue
		}
		rows, err := s.indexPageRows(ctx, m.page)
		if err != nil {
			return nil, err
		}
		start := sort.Search(len(rows), func(i int) bool { return rows[i].Key >= key })
		for _, row := range rows[start:] {
			if row.Key != key {
				break
			}
			add(row.Member, row.Data)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Member < out[j].Member })
	return out, ctx.Err()
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
	add := func(row IndexRow) {
		key := [2]string{row.Key, row.Member}
		if !seen[key] && row.Data != "" {
			out = append(out, row)
		}
		seen[key] = true
	}
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
				add(row)
			}
		}
		// 迁移只从旧表移入不可变页。先读旧行、再取页目录，跨批迁移也不会漏读。
		type pageRef struct {
			hash      string
			singleKey sql.NullString
		}
		var pages []pageRef
		end := prefix + "\U0010ffff"
		err = s.read(ctx, func(ctx context.Context, q queryer) error {
			rows, err := q.QueryContext(ctx, `SELECT sha256,CASE WHEN empty_members AND first_key=last_key THEN first_key END FROM analysis_index_pages_v13 WHERE index_id=? AND kind=?
 AND last_key>=? AND first_key<? ORDER BY legacy DESC,seq`, layer, kind, prefix, end)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var page pageRef
				if err := rows.Scan(&page.hash, &page.singleKey); err != nil {
					return err
				}
				pages = append(pages, page)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
		for _, page := range pages {
			if page.singleKey.Valid && seen[[2]string{page.singleKey.String, ""}] {
				continue
			}
			rows, err := s.indexPageRows(ctx, page.hash)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.Key >= prefix && row.Key < end {
					row.Kind = kind
					add(row)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key || out[i].Key == out[j].Key && out[i].Member < out[j].Member
	})
	return out, nil
}

func (s *Store) SaveIndexText(ctx context.Context, build IndexBuild, path, body string) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := checkIndexGeneration(ctx, tx, build, false); err != nil {
			return err
		}
		id := build.ID
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
