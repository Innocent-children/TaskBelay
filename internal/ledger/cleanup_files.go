package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var planCleanupPath = regexp.MustCompile(`^plans/[A-Za-z0-9_-]+/v[1-9][0-9]*\.md$`)

/**
 * 方案文件只在确认没有已提交版本时移除。检查与 unlink 共用账本写锁，
 * 避免另一进程在核对后发布同名新版本；内容改变或路径越界则保留并报告。
 */
func (s *Store) CleanupPlan(ctx context.Context, item CleanupItem) error {
	if item.Kind != "plan" {
		return fmt.Errorf("不是方案清理记录")
	}
	rel, err := filepath.Rel(s.dataDir, item.Target)
	if err != nil || !planCleanupPath.MatchString(filepath.ToSlash(rel)) {
		return fmt.Errorf("不属于 TaskBelay 方案目录: %s", item.Target)
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var committed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM plans_v11 WHERE json_extract(data,'$.path')=?)`, item.Target).Scan(&committed); err != nil {
			return err
		}
		if !committed {
			root, err := os.OpenRoot(s.dataDir)
			if err != nil {
				return err
			}
			defer root.Close()
			parts := strings.Split(filepath.ToSlash(rel), "/")
			for i := range parts {
				info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
				if errors.Is(err, os.ErrNotExist) {
					_, err = tx.ExecContext(ctx, `DELETE FROM cleanup_items_v11 WHERE id=?`, item.ID)
					return err
				}
				if err != nil {
					return err
				}
				if info.Mode()&os.ModeSymlink != 0 || (i == len(parts)-1 && !info.Mode().IsRegular()) {
					return fmt.Errorf("方案清理路径不是普通文件: %s", item.Target)
				}
			}
			data, err := root.ReadFile(rel)
			if err != nil {
				return err
			}
			if contentHash(string(data)) != item.Expected {
				return fmt.Errorf("方案内容已改变，保留文件: %s", item.Target)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := root.Remove(rel); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM cleanup_items_v11 WHERE id=?`, item.ID)
		return err
	})
}
