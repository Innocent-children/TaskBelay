package ledger

import (
	"context"
	"database/sql"
	"errors"
)

/**
 * 接续事项从任务事实计算，不依赖触发 Stop 的 session，也不另存提示队列。
 */
func pendingActions(ctx context.Context, q queryer, task TaskRecord, requirements []Requirement) ([]string, error) {
	if task.State != TaskActive && task.State != TaskPending {
		return nil, nil
	}
	var actions []string
	active := false
	for _, requirement := range requirements {
		active = active || requirement.Status != "revoked"
	}
	if !active {
		var edited bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM file_changes_v11 c JOIN observations_v11 o ON o.id=c.observation_id
WHERE c.workspace_id=? AND o.id>? AND (o.task_id=? OR o.task_id='')
AND json_extract(c.data,'$.Kind')='agent_edit')`, task.WorkspaceID, task.OriginObservationID, task.ID).Scan(&edited)
		if err != nil {
			return nil, err
		}
		if edited {
			actions = append(actions, "已有可核对的 Agent 编辑，尚未记录有效需求；请补记需求及实现位置。")
		}
	}
	var state string
	err := q.QueryRowContext(ctx, `SELECT state FROM finish_requests_v11 WHERE workspace_id=? AND task_id=?
ORDER BY json_extract(data,'$.at') DESC,rowid DESC LIMIT 1`, task.WorkspaceID, task.ID).Scan(&state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if state == "expired" {
		actions = append(actions, "最近的交付请求已过期；请读取当前内容，完成修改后重新申请交付。")
	}
	return actions, nil
}

func unboundActions(ctx context.Context, q queryer, workspace string) ([]string, error) {
	var edited bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM file_changes_v11 c JOIN observations_v11 o ON o.id=c.observation_id
WHERE c.workspace_id=? AND o.task_id='' AND json_extract(c.data,'$.Kind')='agent_edit'
AND o.id>COALESCE((SELECT MAX(json_extract(data,'$.last_observation_id')) FROM tasks_v11 WHERE workspace_id=?),0))`, workspace, workspace).Scan(&edited)
	if err != nil {
		return nil, err
	}
	if edited {
		return []string{"已有可核对的 Agent 编辑，尚未建立任务及有效需求；请按当前请求补记需求及实现位置。"}, nil
	}
	return nil, nil
}
