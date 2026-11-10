package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const indexPruneSchema = `
CREATE TABLE analysis_index_lives_v14 (
 generation INTEGER PRIMARY KEY AUTOINCREMENT, index_id TEXT NOT NULL UNIQUE, deleting INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE analysis_page_state_v14 (revision INTEGER NOT NULL);
INSERT INTO analysis_page_state_v14 VALUES(0);
PRAGMA user_version=14;
`

// 分析数据都能从仓库内容重新算出。清理按小批次提交，每批都在普通写入的等待时限内完成。
const (
	analysisDeleteRows = 200
	analysisDeleteKeys = 200
	// 全文索引删除一篇正文要重新分词，每批只删几篇。
	analysisDeleteText = 4
	compactMinimum     = 64 << 20
)

type IndexBuild struct {
	ID         string
	Generation int64
}

type IndexInfo struct {
	IndexBuild
	ProjectID, Tree, Variant, ParentID string
}

var ErrIndexRetired = errors.New("所用分析索引已撤下，本次分析未完成")

/**
 * BeginIndexBuild 让同一编号的并发构建共享代次。删除后重新构建会取得新代次，旧写入不能混入。
 */
func (s *Store) BeginIndexBuild(ctx context.Context, id string) (IndexBuild, error) {
	build := IndexBuild{ID: id}
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO analysis_index_lives_v14(index_id) VALUES(?) ON CONFLICT DO NOTHING", id); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT generation FROM analysis_index_lives_v14 WHERE index_id=? AND deleting=0", id).Scan(&build.Generation)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIndexRetired
		}
		return err
	})
	return build, err
}

func checkIndexGeneration(ctx context.Context, q queryer, build IndexBuild, published bool) error {
	query := "SELECT 1 FROM analysis_index_lives_v14 WHERE index_id=?1 AND generation=?2 AND deleting=0"
	if published {
		query += " AND EXISTS(SELECT 1 FROM analysis_indexes_v12 WHERE id=?1)"
	}
	var one int
	err := q.QueryRowContext(ctx, query, build.ID, build.Generation).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrIndexRetired
	}
	return err
}

func (s *Store) CheckPublishedIndex(ctx context.Context, build IndexBuild) error {
	return s.read(ctx, func(ctx context.Context, q queryer) error {
		return checkIndexGeneration(ctx, q, build, true)
	})
}

// AnalysisIndexes 按发布顺序返回全部索引版本。
func (s *Store) AnalysisIndexes(ctx context.Context) ([]IndexInfo, error) {
	var out []IndexInfo
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT i.id,i.project_id,i.tree,i.variant,i.parent_id,COALESCE(l.generation,0)
FROM analysis_indexes_v12 i LEFT JOIN analysis_index_lives_v14 l ON l.index_id=i.id ORDER BY i.rowid`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x IndexInfo
			if err := rows.Scan(&x.ID, &x.ProjectID, &x.Tree, &x.Variant, &x.ParentID, &x.Generation); err != nil {
				return err
			}
			out = append(out, x)
		}
		return rows.Err()
	})
	return out, err
}

// TaskOrigins 返回每个项目里仍有任务使用的起点 tree。
func (s *Store) TaskOrigins(ctx context.Context) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT DISTINCT w.project_id,json_extract(t.data,'$.origin_tree') FROM tasks_v12 t JOIN workspaces_v12 w ON w.id=t.workspace_id")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var project string
			var tree sql.NullString
			if err := rows.Scan(&project, &tree); err != nil {
				return err
			}
			if out[project] == nil {
				out[project] = map[string]bool{}
			}
			out[project][tree.String] = true
		}
		return rows.Err()
	})
	return out, err
}

/**
 * RetireIndex 在同一事务中撤下仍是所选代次的根并登记删除。若新发布的子索引依赖它，继续保留。
 */
func (s *Store) RetireIndex(ctx context.Context, build IndexBuild) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM analysis_indexes_v12 WHERE id=?1
AND COALESCE((SELECT generation FROM analysis_index_lives_v14 WHERE index_id=?1),0)=?2
AND NOT EXISTS(SELECT 1 FROM analysis_indexes_v12 WHERE parent_id=?1)`, build.ID, build.Generation)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO analysis_index_lives_v14(index_id,deleting) VALUES(?,1)
ON CONFLICT(index_id) DO UPDATE SET deleting=1`, build.ID)
		return err
	})
}

func (s *Store) PendingIndexDeletions(ctx context.Context) ([]IndexBuild, error) {
	var builds []IndexBuild
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT index_id,generation FROM analysis_index_lives_v14 WHERE deleting=1 ORDER BY generation")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b IndexBuild
			if err := rows.Scan(&b.ID, &b.Generation); err != nil {
				return err
			}
			builds = append(builds, b)
		}
		return rows.Err()
	})
	return builds, err
}

/**
 * DeleteIndexBatch 只处理已撤下且代次相符的索引。目录、旧行和引用全文均按实际索引编号分批删除，
 * 最后一批在同一事务内清除标记。中断后可从标记续删，不扫描或删除正在构建的版本。
 */
func (s *Store) DeleteIndexBatch(ctx context.Context, build IndexBuild) (done bool, err error) {
	err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var marked bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM analysis_index_lives_v14 WHERE index_id=? AND generation=? AND deleting=1)", build.ID, build.Generation).Scan(&marked); err != nil {
			return err
		}
		if !marked {
			done = true
			return nil
		}
		const texts = "SELECT row_id FROM analysis_text_paths_v12 WHERE index_id=? ORDER BY row_id LIMIT ?"
		removed := int64(0)
		for _, batch := range []struct {
			query string
			limit int
		}{
			{"DELETE FROM analysis_index_pages_v13 WHERE seq IN (SELECT seq FROM analysis_index_pages_v13 WHERE index_id=? LIMIT ?)", analysisDeleteRows},
			{"DELETE FROM analysis_index_rows_v12 WHERE rowid IN (SELECT rowid FROM analysis_index_rows_v12 WHERE index_id=? LIMIT ?)", analysisDeleteRows},
			{"DELETE FROM analysis_text_v12 WHERE rowid IN (" + texts + ")", analysisDeleteText},
			{"DELETE FROM analysis_text_paths_v12 WHERE row_id IN (" + texts + ")", analysisDeleteText},
		} {
			result, err := tx.ExecContext(ctx, batch.query, build.ID, batch.limit)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			removed += n
		}
		done = removed == 0
		if done {
			_, err := tx.ExecContext(ctx, "DELETE FROM analysis_index_lives_v14 WHERE index_id=?", build.ID)
			return err
		}
		return nil
	})
	return done, err
}

/**
 * PurgeCacheBatch 在计算格式版本变化后清掉旧版本留下的缓存。旧缓存的键里含旧版本，
 * 新版本不会再读到它们。首次遇到新版本时记下当时的最大行号，只删除在那之前写入的缓存。
 */
func (s *Store) PurgeCacheBatch(ctx context.Context, algorithm string) (done bool, err error) {
	err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		prefix := "purge:" + algorithm + ":"
		var marker sql.NullString
		if err := tx.QueryRowContext(ctx, "SELECT MAX(cache_key) FROM analysis_cache_v12 WHERE kind='meta' AND cache_key LIKE 'purge:%'").Scan(&marker); err != nil {
			return err
		}
		set := func(upto int64) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM analysis_cache_v12 WHERE kind='meta' AND cache_key LIKE 'purge:%'"); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, "INSERT INTO analysis_cache_v12(kind,cache_key,encoding,raw_bytes,sha256,body) VALUES('meta',?,'',0,'',x'')", prefix+strconv.FormatInt(upto, 10))
			return err
		}
		var upto int64
		if rest, ok := strings.CutPrefix(marker.String, prefix); ok {
			if upto, err = strconv.ParseInt(rest, 10, 64); err != nil {
				return fmt.Errorf("缓存清理标记无效")
			}
		} else {
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(rowid),0) FROM analysis_cache_v12").Scan(&upto); err != nil {
				return err
			}
			if err := set(upto); err != nil {
				return err
			}
		}
		if upto == 0 {
			done = true
			return nil
		}
		rows, err := tx.QueryContext(ctx, "SELECT DISTINCT kind FROM analysis_cache_v12")
		if err != nil {
			return err
		}
		var kinds []string
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				rows.Close()
				return err
			}
			// 当前格式的词流可能在第一次维护前已经建好，随后检查还要复用。
			if kind != "meta" && kind != indexPageKind && !strings.HasPrefix(kind, algorithm+":") {
				kinds = append(kinds, kind)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		budget := int64(analysisDeleteKeys)
		for _, kind := range kinds {
			if budget <= 0 {
				return nil
			}
			result, err := tx.ExecContext(ctx, "DELETE FROM analysis_cache_v12 WHERE rowid IN (SELECT rowid FROM analysis_cache_v12 WHERE kind=? AND rowid<=? LIMIT ?)", kind, upto, budget)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			budget -= n
		}
		if budget > 0 {
			done = true
			return set(0)
		}
		return nil
	})
	return done, err
}

/**
 * PageState 返回索引页目录引用的全部页，以及最近一次登记页的修订号。
 * packed 是可能内嵌分块引用的页：单条记录的 file 和 learning 页。
 */
func (s *Store) PageState(ctx context.Context) (referenced map[string]bool, packed []string, revision int64, err error) {
	referenced = map[string]bool{}
	err = s.read(ctx, func(ctx context.Context, q queryer) error {
		if err := q.QueryRowContext(ctx, "SELECT revision FROM analysis_page_state_v14").Scan(&revision); err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, "SELECT DISTINCT sha256,kind IN ('file','learning') AND first_key=last_key FROM analysis_index_pages_v13")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hash string
			var single bool
			if err := rows.Scan(&hash, &single); err != nil {
				return err
			}
			if single && !referenced[hash] {
				packed = append(packed, hash)
			}
			referenced[hash] = true
		}
		return rows.Err()
	})
	return referenced, packed, revision, err
}

// PagePart 返回一页内嵌引用的分块编号；没有分块时为空。
func (s *Store) PagePart(ctx context.Context, hash string) (string, error) {
	raw, err := s.readPageBody(ctx, hash)
	if err != nil {
		return "", err
	}
	if len(raw) > 64 && raw[0] == 1 {
		return string(raw[1:65]), nil
	}
	return "", nil
}

// CachedPages 返回缓存里保存的全部索引页编号。
func (s *Store) CachedPages(ctx context.Context) ([]string, error) {
	var out []string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT cache_key FROM analysis_cache_v12 WHERE kind=?", indexPageKind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			out = append(out, key)
		}
		return rows.Err()
	})
	return out, err
}

/**
 * DeletePages 删除一批不再被引用的索引页。目录在核对之后有新页登记时放弃这一批：
 * 新登记的页可能正好复用了其中的内容。
 */
func (s *Store) DeletePages(ctx context.Context, hashes []string, revision int64) (bool, error) {
	deleted := false
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var current int64
		if err := tx.QueryRowContext(ctx, "SELECT revision FROM analysis_page_state_v14").Scan(&current); err != nil {
			return err
		}
		if current != revision {
			return nil
		}
		for _, hash := range hashes {
			if _, err := tx.ExecContext(ctx, "DELETE FROM analysis_cache_v12 WHERE kind=? AND cache_key=?", indexPageKind, hash); err != nil {
				return err
			}
		}
		deleted = true
		return nil
	})
	return deleted, err
}

/**
 * Shrink 在空闲页超过文件一半且不少于 64 MiB 时收缩账本文件。
 * 收缩期间其他进程的写入会等待到各自的时限后放弃，hook 按自身故障处理、不影响 Agent。
 */
func (s *Store) Shrink(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var free, total, size int64
	for query, target := range map[string]*int64{"PRAGMA freelist_count": &free, "PRAGMA page_count": &total, "PRAGMA page_size": &size} {
		if err := conn.QueryRowContext(ctx, query).Scan(target); err != nil {
			return err
		}
	}
	if free*size < compactMinimum || free*2 < total {
		return nil
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 2000"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
