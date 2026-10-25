package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) PlanEditContext(ctx context.Context, workspace string, actor Actor) (PlanEditContext, error) {
	var result PlanEditContext
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result.TaskID, err = currentID(ctx, q, workspace)
		if err != nil {
			return err
		}
		settings, err := effectiveSettings(ctx, q, workspace, result.TaskID, false)
		if err != nil {
			return err
		}
		result.Workflow, result.SettingsRevision = settings.Values["workflow"].Value, settings.Revision
		if result.TaskID == "" {
			return nil
		}
		task, err := loadTask(ctx, q, workspace, result.TaskID)
		if err != nil {
			return err
		}
		result.PlanRevision, result.AuthorizationRevision = task.PlanRevision, task.AuthorizationRevision
		result.Plans, err = readPlans(ctx, q, task)
		if err != nil {
			return err
		}
		notes, err := readNotes(ctx, q, task.ID)
		if err != nil {
			return err
		}
		result.Decisions = effectiveDecisions(notes, task, settings, actor)
		return nil
	})
	return result, err
}

func publishPlanEdit(ctx context.Context, tx *sql.Tx, observation Observation, edit *EditRecord) error {
	proof := edit.PlanCheck
	if proof == nil {
		return nil
	}
	proof.CheckedAt = observation.At
	if err := checkCurrent(ctx, tx, observation.WorkspaceID, proof.TaskID); err != nil {
		return err
	}
	settings, err := effectiveSettings(ctx, tx, observation.WorkspaceID, proof.TaskID, false)
	if err != nil {
		return err
	}
	if settings.Revision != proof.SettingsRevision {
		return fmt.Errorf("编辑核对期间模式或设置已变化")
	}
	for i := range proof.Issues {
		if settings.Values["enforcement"].Value == "hint" {
			proof.Issues[i].Level = "hint"
		}
	}
	if proof.TaskID != "" {
		task, err := loadTask(ctx, tx, observation.WorkspaceID, proof.TaskID)
		if err != nil {
			return err
		}
		if task.PlanRevision != proof.PlanRevision || task.AuthorizationRevision != proof.AuthorizationRevision {
			return fmt.Errorf("编辑核对期间方案授权已变化")
		}
		result := CheckRecord{Input: CheckInput{TaskID: task.ID}, Issues: proof.Issues}
		if err := reconcileIssues(ctx, tx, &result); err != nil {
			return err
		}
		proof.Issues = result.Issues
	} else {
		for i := range proof.Issues {
			issue := &proof.Issues[i]
			issue.Identity = fmt.Sprintf("%x", sha256.Sum256([]byte(observation.WorkspaceID+"\x00"+issue.Rule+"\x00"+issue.At+"\x00"+issue.FactKey)))
			issue.ID = "i-" + issue.Identity[:20] + "-1"
			issue.Occurrence, issue.LogicalAt = 1, issue.At
		}
	}
	if edit.Decision.DenyTool == "" {
		blocked, err := blockIssues(ctx, tx, observation.WorkspaceID, Actor{Agent: edit.Agent, SessionID: edit.SessionID, RoundID: observation.RoundID}, proof.Issues)
		if err != nil {
			return err
		}
		if len(blocked) > 0 {
			proof.Blocked = true
			edit.Decision.DenyTool = "[S1] 编辑目标尚未获得方案授权，请先提交方案并记录用户确认；已确认范围可继续。"
		}
	}
	var text strings.Builder
	for _, issue := range proof.Issues {
		fmt.Fprintf(&text, "[S1 %s] %s；编号=%s；状态=%s\n%s\n%s\n依据：%s\n", issue.Level, issue.At, issue.ID, issue.Status, strings.Join(issue.Facts, " "), issue.Advice, strings.Join(issue.Evidence, "；"))
	}
	edit.Decision.AdditionalContext = strings.TrimSpace(edit.Decision.AdditionalContext + "\n" + text.String())
	return nil
}

func (s *Store) TaskEdits(ctx context.Context, workspace, task string, through int64) ([]EditRecord, error) {
	var result []EditRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT e.data FROM edits e JOIN observations o ON o.id=e.before_observation
WHERE e.workspace_id=? AND o.task_id=? AND e.after_observation>0 AND e.after_observation<=? ORDER BY e.after_observation`, workspace, task, through)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var data string
			var edit EditRecord
			if err := rows.Scan(&data); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(data), &edit); err != nil {
				return err
			}
			result = append(result, edit)
		}
		return rows.Err()
	})
	return result, err
}
