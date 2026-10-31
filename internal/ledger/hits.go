package ledger

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

type HitObservation struct {
	At           time.Time `json:"at"`
	Kind         string    `json:"kind"`
	RecordID     string    `json:"record_id"`
	Status       string    `json:"status"`
	InputVersion string    `json:"input_version,omitempty"`
	Issue        Issue     `json:"issue"`
}

type HitBlock struct {
	Actor Actor  `json:"actor"`
	At    string `json:"at,omitempty"`
}

type Hit struct {
	TaskID       string           `json:"task_id"`
	Issue        Issue            `json:"current_issue"`
	Observations []HitObservation `json:"observations"`
	Blocks       []HitBlock       `json:"blocks"`
	Handling     []Note           `json:"handling"`
}

/**
 * 导出时间窗口内实际记录的问题和反馈，并附上当前处理状态。只读取账本，
 * 不重新运行检查；过期检查仍标明 expired，不能算作当前有效拦截。
 */
func (s *Store) Hits(ctx context.Context, workspace string, since time.Time) ([]Hit, error) {
	var result []Hit
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		byID := map[string]*Hit{}
		rows, err := q.QueryContext(ctx, `SELECT task_id,data,status FROM issues_v11 WHERE task_id IN (SELECT id FROM tasks_v11 WHERE workspace_id=?)`, workspace)
		if err != nil {
			return err
		}
		for rows.Next() {
			var hit Hit
			var raw, status string
			if err := rows.Scan(&hit.TaskID, &raw, &status); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &hit.Issue); err != nil {
				rows.Close()
				return err
			}
			hit.Issue.Status = status
			byID[hit.Issue.ID] = &hit
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = q.QueryContext(ctx, `SELECT task_id,CAST(seq AS TEXT),status,input_version,json_extract(data,'$.finished_at'),json_extract(data,'$.issues') FROM checks_v11
WHERE task_id IN (SELECT id FROM tasks_v11 WHERE workspace_id=?) AND julianday(json_extract(data,'$.finished_at'))>=julianday(?) ORDER BY seq`, workspace, since.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		for rows.Next() {
			var task, id, status, version, at string
			var raw *string
			if err := rows.Scan(&task, &id, &status, &version, &at, &raw); err != nil {
				rows.Close()
				return err
			}
			if raw == nil {
				continue
			}
			var issues []Issue
			if err := json.Unmarshal([]byte(*raw), &issues); err != nil {
				rows.Close()
				return err
			}
			observed, err := time.Parse(time.RFC3339Nano, at)
			if err != nil {
				rows.Close()
				return err
			}
			for _, issue := range issues {
				hit := byID[issue.ID]
				if hit == nil {
					hit = &Hit{TaskID: task, Issue: issue}
					byID[issue.ID] = hit
				}
				hit.Observations = append(hit.Observations, HitObservation{At: observed, Kind: "check", RecordID: id, Status: status, InputVersion: version, Issue: issue})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = q.QueryContext(ctx, `SELECT data FROM verifications_v11 WHERE workspace_id=? AND (julianday(json_extract(data,'$.started_at'))>=julianday(?) OR julianday(json_extract(data,'$.ended_at'))>=julianday(?)) ORDER BY seq`, workspace, since.UTC().Format(time.RFC3339Nano), since.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw string
			var v VerificationRecord
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				rows.Close()
				return err
			}
			if v.Issue == nil {
				continue
			}
			hit := byID[v.Issue.ID]
			if hit == nil {
				hit = &Hit{TaskID: v.TaskID, Issue: *v.Issue}
				byID[v.Issue.ID] = hit
			}
			at := v.StartedAt
			if at.IsZero() {
				at = v.EndedAt
			}
			status := v.State
			if v.Denied {
				status = "denied"
			}
			hit.Observations = append(hit.Observations, HitObservation{At: at, Kind: "verification", RecordID: v.ID, Status: status, Issue: *v.Issue})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = q.QueryContext(ctx, `SELECT b.issue_id,b.agent,b.session_id,b.round_id,f.at FROM issue_blocks_v11 b JOIN check_feedback_v11 f ON f.id=b.feedback_id
WHERE f.workspace_id=? AND julianday(f.at)>=julianday(?) ORDER BY f.id`, workspace, since.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var block HitBlock
			if err := rows.Scan(&id, &block.Actor.Agent, &block.Actor.SessionID, &block.Actor.RoundID, &block.At); err != nil {
				rows.Close()
				return err
			}
			if hit := byID[id]; hit != nil {
				hit.Blocks = append(hit.Blocks, block)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		notes := map[string][]Note{}
		for _, hit := range byID {
			if len(hit.Observations)+len(hit.Blocks) == 0 {
				continue
			}
			if _, ok := notes[hit.TaskID]; !ok {
				values, err := readNotes(ctx, q, hit.TaskID)
				if err != nil {
					return err
				}
				notes[hit.TaskID] = values
			}
			for _, note := range notes[hit.TaskID] {
				matched := note.Target != nil && note.Target.Kind == "issue" && note.Target.ID == hit.Issue.ID
				for _, id := range hit.Issue.HandledBy {
					matched = matched || id == note.ID
				}
				if matched {
					hit.Handling = append(hit.Handling, note)
				}
			}
			result = append(result, *hit)
		}
		sort.Slice(result, func(i, j int) bool {
			if result[i].TaskID != result[j].TaskID {
				return result[i].TaskID < result[j].TaskID
			}
			return result[i].Issue.ID < result[j].Issue.ID
		})
		return nil
	})
	return result, err
}
