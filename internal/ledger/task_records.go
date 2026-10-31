package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

type RecordQuery struct {
	TaskID   string
	Category string
	AfterSeq int64
	Limit    int
	ID       string
	Revision int64
}

type TaskRecordPage struct {
	TaskID  string
	Total   int
	Records []TaskRecordBody
	HasMore bool
}

type TaskRecordBody struct {
	Seq      int64
	ID       string
	Revision int64
	Data     json.RawMessage
	State    map[string]any
}

var RecordCategories = []string{"messages", "sources", "requirements", "notes", "plans", "checks", "verifications", "workspace_updates", "task_updates"}

/**
 * 分页只读取指定任务中的记录。游标是保留期间不改写的记录序号，不是切片下标；
 * 单条正文与查询时的状态分开，确认、交付或验证结束不会移动正文的字符偏移。
 */
func (s *Store) TaskRecords(ctx context.Context, workspace string, actor Actor, input RecordQuery) (TaskRecordPage, error) {
	var result TaskRecordPage
	if input.AfterSeq < 0 || input.Revision < 0 || input.Limit < 0 || input.Limit > 100 {
		return result, fmt.Errorf("after_seq、revision 不能为负；limit 为 1 至 100，默认 20")
	}
	if input.ID != "" && (input.Category == "requirements" || input.Category == "plans") && input.Revision == 0 {
		return result, fmt.Errorf("读取需求或方案正文必须指定 revision；从列表取得对应修订号")
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		if input.TaskID == "" {
			var err error
			input.TaskID, err = currentID(ctx, q, workspace)
			if err != nil {
				return err
			}
		}
		if input.TaskID == "" {
			return fmt.Errorf("当前没有任务，请先用 task_get 选择任务")
		}
		task, err := loadTask(ctx, q, workspace, input.TaskID)
		if err != nil {
			return err
		}
		result.TaskID = task.ID
		query, args, err := recordQuery(task, input.Category)
		if err != nil {
			return err
		}
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+query+")", args...).Scan(&result.Total); err != nil {
			return err
		}
		columns := "seq,id,revision,'{}',state"
		if input.ID != "" {
			columns = "seq,id,revision,data,state"
		}
		query = "SELECT " + columns + " FROM (" + query + ") WHERE seq>?"
		args = append(args, input.AfterSeq)
		if input.ID != "" {
			query += " AND id=?"
			args = append(args, input.ID)
		}
		if input.Revision != 0 {
			query += " AND revision=?"
			args = append(args, input.Revision)
		}
		query += " ORDER BY seq LIMIT ?"
		args = append(args, input.Limit+1)
		rows, err := q.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var record TaskRecordBody
			var data, state string
			if err := rows.Scan(&record.Seq, &record.ID, &record.Revision, &data, &state); err != nil {
				return err
			}
			if len(result.Records) == input.Limit {
				result.HasMore = true
				break
			}
			if err := json.Unmarshal([]byte(state), &record.State); err != nil {
				return err
			}
			if input.ID == "" {
				result.Records = append(result.Records, record)
				continue
			}
			if input.Category == "checks" {
				data, err = restoreCheckBody(ctx, q, record.Seq, data)
				if err != nil {
					return err
				}
				var check CheckRecord
				if err := json.Unmarshal([]byte(data), &check); err != nil {
					return err
				}
				current, err := checkInput(ctx, q, workspace, task.ID, actor, check.Input.Rules, check.Input.Capabilities)
				record.State["validity"] = "unknown"
				if record.State["status"] == string(CheckExpired) {
					record.State["validity"] = "expired"
				} else if err == nil {
					record.State["validity"] = "current"
					if current.Version() != check.InputVersion {
						record.State["status"], record.State["level"], record.State["validity"] = "expired", "hint", "expired"
					}
				}
			}
			record.Data, err = recordBody(input.Category, record.Seq, data)
			if err != nil {
				return fmt.Errorf("读取 %s %s: %w", input.Category, record.ID, err)
			}
			result.Records = append(result.Records, record)
		}
		return rows.Err()
	})
	return result, err
}

func recordQuery(task TaskRecord, category string) (string, []any, error) {
	args := []any{task.ID}
	switch category {
	case "messages":
		return `SELECT seq,id,0 revision,data,'{}' state FROM messages_v11 WHERE workspace_id=?
AND (seq>=COALESCE((SELECT seq FROM messages_v11 WHERE id=?),0) AND (?=0 OR seq<=?)
OR id IN (SELECT json_extract(data,'$.citation.source_id') FROM requirement_versions_v11 WHERE task_id=?))`,
			[]any{task.WorkspaceID, task.StartMessageID, task.EndMessageSeq, task.EndMessageSeq, task.ID}, nil
	case "sources":
		return `WITH RECURSIVE ids(id) AS (
SELECT id FROM messages_v11 WHERE workspace_id=? AND seq>=COALESCE((SELECT seq FROM messages_v11 WHERE id=?),0) AND (?=0 OR seq<=?)
UNION SELECT id FROM sources_v11 WHERE workspace_id=? AND task_id=?
UNION SELECT json_extract(data,'$.citation.source_id') FROM requirement_versions_v11 WHERE task_id=?
UNION SELECT json_extract(l.value,'$.citation.source_id') FROM requirement_versions_v11 r,json_each(r.data,'$.implemented_by') l WHERE r.task_id=?
UNION SELECT json_extract(data,'$.citation.source_id') FROM notes_v11 WHERE task_id=?
UNION SELECT json_extract(s.data,'$.confirmation.source_id') FROM sources_v11 s JOIN ids ON ids.id=s.id WHERE s.workspace_id=?
)
SELECT s.rowid seq,s.id,0 revision,s.data,'{}' state FROM sources_v11 s JOIN ids ON ids.id=s.id WHERE s.workspace_id=?`,
			[]any{task.WorkspaceID, task.StartMessageID, task.EndMessageSeq, task.EndMessageSeq, task.WorkspaceID, task.ID, task.ID, task.ID, task.ID, task.WorkspaceID, task.WorkspaceID}, nil
	case "requirements":
		return `SELECT rowid seq,id,revision,data,json_object('status',json_extract(data,'$.status')) state FROM requirement_versions_v11 WHERE task_id=?`, args, nil
	case "notes":
		return `SELECT seq,id,0 revision,data,json_object('active',active,'consumed',consumed,'invalidated_by',invalidated_by) state FROM notes_v11 WHERE task_id=?`, args, nil
	case "plans":
		return `SELECT version seq,json_extract(data,'$.id') id,json_extract(data,'$.revision') revision,data,
json_object('confirmed',json_extract(data,'$.confirmation') IS NOT NULL,'revocations',COALESCE(json_array_length(data,'$.revocations'),0),
'confirmation_source_id',json_extract(data,'$.confirmation.citation.source_id')) state FROM plans_v11 WHERE task_id=?`, args, nil
	case "checks":
		return `SELECT seq,CAST(seq AS TEXT) id,0 revision,data,json_object('status',status,'level',json_extract(data,'$.level'),
'validity','not_checked','delivery_updated',json_extract(data,'$.delivery_updates') IS NOT NULL) state FROM checks_v11 WHERE task_id=?`, args, nil
	case "verifications":
		return `SELECT seq,id,0 revision,data,json_object('state',state,'executed',executed,'denied',denied,
'exit_code',json_extract(data,'$.exit_code'),'ended_at',json_extract(data,'$.ended_at')) state FROM verifications_v11 WHERE task_id=?`, args, nil
	case "workspace_updates":
		return `SELECT c.id seq,CAST(c.id AS TEXT) id,0 revision,c.data,
json_object('trigger',json_extract(o.data,'$.Trigger')) state FROM file_changes_v11 c JOIN observations_v11 o ON o.id=c.observation_id
WHERE o.task_id=? AND json_extract(c.data,'$.Kind')='workspace_update'`, args, nil
	case "task_updates":
		return `SELECT observation_id seq,CAST(observation_id AS TEXT) id,0 revision,data,'{}' state FROM task_updates_v11 WHERE task_id=?`, args, nil
	}
	return "", nil, fmt.Errorf("未知记录类别 %q", category)
}

func recordBody(category string, seq int64, data string) (json.RawMessage, error) {
	if category == "task_updates" {
		if !json.Valid([]byte(data)) {
			return nil, fmt.Errorf("无效记录正文")
		}
		return json.RawMessage(data), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		return nil, err
	}
	var mutable []string
	switch category {
	case "notes":
		mutable = []string{"active", "consumed", "invalidated_by"}
	case "plans":
		mutable = []string{"confirmation", "revocations"}
	case "checks":
		fields["id"] = json.RawMessage(strconv.FormatInt(seq, 10))
		mutable = []string{"status", "level", "issues", "delivery_updates"}
		var issues []map[string]json.RawMessage
		if err := json.Unmarshal(fields["issues"], &issues); err != nil && string(fields["issues"]) != "null" {
			return nil, err
		}
		for _, issue := range issues {
			for _, key := range []string{"level", "status", "block_on_delivery", "blocked_sessions", "handled_by"} {
				delete(issue, key)
			}
		}
		encoded, err := json.Marshal(issues)
		if err != nil {
			return nil, err
		}
		fields["issue_facts"] = encoded
	case "verifications":
		mutable = []string{"state", "executed", "exit_code", "ended_at"}
	}
	for _, key := range mutable {
		delete(fields, key)
	}
	return json.Marshal(fields)
}

func (s *Store) TaskRecordCounts(ctx context.Context, workspace, id string) (map[string]int, error) {
	counts := make(map[string]int)
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		task, err := loadTask(ctx, q, workspace, id)
		if err != nil {
			return err
		}
		for _, category := range RecordCategories {
			query, args, err := recordQuery(task, category)
			if err != nil {
				return err
			}
			var count int
			if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+query+")", args...).Scan(&count); err != nil {
				return err
			}
			counts[category] = count
		}
		return nil
	})
	return counts, err
}
