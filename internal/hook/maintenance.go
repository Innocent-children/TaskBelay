package hook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

/**
 * 清理共用入口提供的独立维护预算，不在快速工具前路径或后台循环运行。
 * 记录先提交，耗时的 git 引用操作随后执行；失败项留待下次入口并可查看。
 */
func (t Tasks) Maintain(ctx context.Context, now time.Time) error {
	if err := t.Store.Prune(ctx, t.Workspace.ID, now); err != nil {
		return err
	}
	refs, err := t.Workspace.OldSnapshots(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		return err
	}
	queued := 0
	for _, ref := range refs {
		if queued >= 32 {
			break
		}
		used, err := t.Store.RefInUse(ctx, ref.Ref)
		if err != nil {
			return err
		}
		if used {
			continue
		}
		if err := t.Store.StageRefCleanup(ctx, ref, now); err != nil {
			return err
		}
		queued++
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
				err = t.Workspace.ReleaseSnapshot(ctx, gitstate.SnapshotRef{Ref: item.Target, Object: item.Expected, WorkspaceID: item.WorkspaceID})
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
	if err := t.Store.Compact(ctx, t.Workspace.ID); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
