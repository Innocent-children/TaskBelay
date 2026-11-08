package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

/**
 * schema 13 的写入方只追加不可变页。两个整理进程可能选中同一批旧行，
 * 取得写锁后核对实际删除的行数，页和旧行的删除一起提交。
 */
func (s *Store) compactIndexBatch(ctx context.Context) (bool, error) {
	var id, kind string
	var batch []IndexRow
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		err := q.QueryRowContext(ctx, `SELECT index_id,kind FROM analysis_index_rows_v12
 ORDER BY index_id,kind,key,member LIMIT 1`).Scan(&id, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `SELECT key,member,data FROM analysis_index_rows_v12 WHERE index_id=? AND kind=?
 ORDER BY key,member LIMIT 1024`, id, kind)
		if err != nil {
			return err
		}
		defer rows.Close()
		size := 0
		for rows.Next() {
			row := IndexRow{Kind: kind}
			if err := rows.Scan(&row.Key, &row.Member, &row.Data); err != nil {
				return err
			}
			batch = append(batch, row)
			size += len(row.Key) + len(row.Member) + len(row.Data)
			if size >= 512*1024 {
				break
			}
		}
		return rows.Err()
	})
	if err != nil || len(batch) == 0 {
		return false, err
	}
	pages, err := prepareIndexPages(ctx, batch)
	if err != nil {
		return false, err
	}
	first, last := batch[0], batch[len(batch)-1]
	err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM analysis_index_rows_v12 WHERE index_id=? AND kind=?
 AND (key,member)>=(?,?) AND (key,member)<=(?,?)`, id, kind, first.Key, first.Member, last.Key, last.Member)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
		if count != int64(len(batch)) {
			// 另一进程先完成了同一范围的一部分。回滚这次删除，再读剩余行。
			return errIndexBatchChanged
		}
		return saveIndexPages(ctx, tx, id, pages, true)
	})
	if errors.Is(err, errIndexBatchChanged) {
		return true, nil
	}
	return err == nil, err
}

var errIndexBatchChanged = errors.New("待整理的索引范围已变化")

func (s *Store) compactIndexes(ctx context.Context) error {
	for {
		more, err := s.compactIndexBatch(ctx)
		if err != nil || !more {
			return err
		}
	}
}

/**
 * 逐条处理已有的分析缓存和分析结果。计算在事务外完成，
 * 提交时用摘要或原文复核并发变化。
 */
func (s *Store) compactAnalysisBodies(ctx context.Context) error {
	for _, table := range []string{"analysis_cache_v12", "analysis_records_v12"} {
		var last int64
		for {
			var old storedBody
			err := s.read(ctx, func(ctx context.Context, q queryer) error {
				if table == "analysis_records_v12" {
					return q.QueryRowContext(ctx, `SELECT rowid,data FROM analysis_records_v12 WHERE rowid>? AND encoding='' ORDER BY rowid LIMIT 1`, last).Scan(&last, &old.Body)
				}
				return q.QueryRowContext(ctx, `SELECT rowid,encoding,raw_bytes,sha256,body FROM analysis_cache_v12
 WHERE rowid>? AND kind<>? AND encoding IN ('gzip','json') ORDER BY rowid LIMIT 1`, last, indexPageKind).Scan(&last, &old.Encoding, &old.RawBytes, &old.SHA256, &old.Body)
			})
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				return err
			}
			raw := old.Body
			if old.Encoding != "" {
				raw, err = old.decode(ctx)
				if err != nil {
					return fmt.Errorf("%s 记录 %d: %w", table, last, err)
				}
			}
			body, err := prepareBody(ctx, raw)
			if err != nil {
				return err
			}
			if table == "analysis_cache_v12" && len(body.Body) >= len(old.Body) {
				continue
			}
			err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				if table == "analysis_records_v12" {
					_, err := tx.ExecContext(ctx, `UPDATE analysis_records_v12 SET data=?,encoding=?,raw_bytes=?,sha256=? WHERE rowid=? AND encoding='' AND data=?`, body.Body, body.Encoding, body.RawBytes, body.SHA256, last, string(raw))
					return err
				}
				_, err := tx.ExecContext(ctx, `UPDATE analysis_cache_v12 SET body=?,encoding=? WHERE rowid=? AND sha256=? AND kind<>? AND length(body)>?`, body.Body, body.Encoding, last, body.SHA256, indexPageKind, len(body.Body))
				return err
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

/**
 * 显式维护时整理旧索引并重排数据库。VACUUM 拥有独立事务，
 * 沿用 SQLite 的锁等待上限，重排本身使用命令的可取消 context。
 */
func (s *Store) ReclaimSpace(ctx context.Context) error {
	if err := s.compactIndexes(ctx); err != nil {
		return fmt.Errorf("整理分析索引: %w", err)
	}
	if err := s.compactAnalysisBodies(ctx); err != nil {
		return fmt.Errorf("整理分析正文: %w", err)
	}
	conn, _, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("归还数据库空闲页: %w", err)
	}
	var busy, logged, copied int
	if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logged, &copied); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("数据库已整理，仍有连接占用 WAL（已回写 %d/%d 页），空闲后可再次运行 compact", copied, logged)
	}
	return nil
}
