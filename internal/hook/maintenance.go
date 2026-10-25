package hook

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/**
 * 清理使用调用入口的剩余时间，不在快速工具前路径或后台循环运行。
 * 记录先提交，耗时的 git 引用操作随后执行；失败项留待下次入口并可查看。
 */
func (t Tasks) Maintain(ctx context.Context, now time.Time) error {
	if err := t.Store.Prune(ctx, t.Workspace.ID, now); err != nil {
		return err
	}
	items, err := t.Store.CleanupItems(ctx, t.Workspace.ID, now)
	if err != nil {
		return err
	}
	var failures []error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		switch item.Kind {
		case "plan":
			err = t.Store.CleanupPlan(ctx, item)
		case "ref":
			var used bool
			used, err = t.Store.RefInUse(ctx, item.Target)
			if err == nil && !used {
				err = t.Workspace.ReleaseSnapshot(ctx, item.Target)
			}
			if err == nil {
				err = t.Store.FinishCleanup(ctx, item.ID, nil)
			}
		default:
			err = fmt.Errorf("未知清理类型: %s", item.Kind)
		}
		if err != nil {
			failures = append(failures, err)
			if saveErr := t.Store.FinishCleanup(ctx, item.ID, err); saveErr != nil {
				failures = append(failures, saveErr)
			}
		}
	}
	// 引用先于观察事务写入。崩溃留下的未入账引用也按三十天清理，
	// 年龄门槛排除正常的一秒观察窗口；仍有任何账本引用时继续保留。
	refs, err := t.Workspace.OldSnapshots(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	removed := 0
	for _, ref := range refs {
		if removed >= 32 {
			break
		}
		used, err := t.Store.RefInUse(ctx, ref)
		if err != nil {
			return errors.Join(append(failures, err)...)
		}
		if used {
			continue
		}
		if err := t.Workspace.ReleaseSnapshot(ctx, ref); err != nil {
			failures = append(failures, err)
		}
		removed++
	}
	return errors.Join(failures...)
}
