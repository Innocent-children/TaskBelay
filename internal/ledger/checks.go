package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrCheckInputChanged = errors.New("检查输入已变化，请读取当前任务后继续")

type checkDeliveryUpdates struct {
	Verifications   []VerificationRecord `json:"verifications"`
	ValidationNotes []Note               `json:"validation_notes"`
}

func effectiveDecisions(notes []Note, task TaskRecord, settings EffectiveSettings, actor Actor) []Note {
	var result []Note
	for _, note := range notes {
		if !note.Active || note.Consumed || note.Target == nil {
			continue
		}
		if note.Target.Scope == "round" && (actor.Agent != note.Author.Agent || actor.SessionID != note.Author.SessionID || actor.RoundID != note.Author.RoundID) {
			continue
		}
		if note.Target.Effect == "rerun_once" && (note.InputRevision != task.RecordRevision || note.RequirementRevision != task.RequirementRevision || note.PlanRevision != task.PlanRevision || note.SettingsRevision != settings.Revision) {
			continue
		}
		result = append(result, note)
	}
	return result
}

func checkInput(ctx context.Context, q queryer, workspace, id string, actor Actor, algorithm string) (CheckInput, error) {
	task, err := loadTask(ctx, q, workspace, id)
	if err != nil {
		return CheckInput{}, err
	}
	observation, err := loadObservation(ctx, q, workspace, task.LastObservationID)
	if err != nil {
		return CheckInput{}, err
	}
	if !observation.Full || !observation.Complete || observation.Tree == "" {
		return CheckInput{}, fmt.Errorf("检查需要完整工作区快照")
	}
	settings, err := effectiveSettings(ctx, q, workspace, id, false)
	if err != nil {
		return CheckInput{}, err
	}
	notes, err := readNotes(ctx, q, id)
	if err != nil {
		return CheckInput{}, err
	}
	input := NewCheckInput(task, observation.Tree, settings, effectiveDecisions(notes, task, settings, actor), algorithm)
	input.PurposeNotes = PurposeNoteIDs(notes)
	for _, note := range notes {
		if note.Kind == "decision" && note.Target != nil && note.Target.Kind == "issue" {
			raw, err := json.Marshal(struct {
				Target   *DecisionTarget
				Text     string
				Citation *Citation
			}{note.Target, note.Text, note.Citation})
			if err != nil {
				return CheckInput{}, err
			}
			input.Feedback = append(input.Feedback, string(raw))
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT data FROM verifications_v12 WHERE task_id=? AND executed=1 AND state<>'running' AND json_extract(data,'$.algorithm')=? ORDER BY seq`, id, algorithm)
	if err != nil {
		return CheckInput{}, err
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return CheckInput{}, err
		}
		hash.Write([]byte(raw))
		hash.Write([]byte{0})
	}
	input.Verifications = fmt.Sprintf("%x", hash.Sum(nil))
	return input, rows.Err()
}

// 用途笔记不可变；编号把内容、引用和记录时的观察点一起绑定到检查输入。
func PurposeNoteIDs(notes []Note) []string {
	var ids []string
	for _, note := range notes {
		if note.Review != nil {
			ids = append(ids, note.ID)
		}
	}
	return ids
}

func (s *Store) CheckInput(ctx context.Context, workspace, id string, actor Actor, algorithm string) (CheckInput, error) {
	var input CheckInput
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		input, err = checkInput(ctx, q, workspace, id, actor, algorithm)
		return err
	})
	return input, err
}

func (s *Store) CheckReusable(ctx context.Context, workspace string, actor Actor, record CheckRecord) (bool, error) {
	var reusable bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		current, err := currentID(ctx, q, workspace)
		if err != nil {
			return err
		}
		task, err := loadTask(ctx, q, workspace, record.Input.TaskID)
		if err != nil {
			return err
		}
		input, err := checkInput(ctx, q, workspace, task.ID, actor, record.Input.Algorithm)
		if err != nil {
			return err
		}
		if current != task.ID || task.State != TaskActive && task.State != TaskPending || input.Version() != record.InputVersion {
			return ErrCheckInputChanged
		}
		var id int64
		var status CheckStatus
		if err := q.QueryRowContext(ctx, `SELECT seq,status FROM checks_v12 WHERE task_id=? AND depth=? AND input_version=? ORDER BY seq DESC LIMIT 1`, task.ID, record.Depth, record.InputVersion).Scan(&id, &status); err != nil {
			return err
		}
		reusable = id == record.ID && status == record.Status && record.ReusableStatic()
		return nil
	})
	return reusable, err
}

type withoutReportsKey struct{}
type withReportsKey struct{}

/**
 * WithCheckReports 让任务视图里的检查记录带上完整的报告正文。报告正文可达数兆字节，
 * 普通任务视图不解压正文，status 的检查详情和 debug 快照使用完整报告。
 */
func WithCheckReports(ctx context.Context) context.Context {
	return context.WithValue(ctx, withReportsKey{}, true)
}

func latestCheck(ctx context.Context, q queryer, task, depth, version string, excludeExpired bool) (*CheckRecord, error) {
	query := "SELECT seq,status,data FROM checks_v12 WHERE task_id=? AND depth=?"
	args := []any{task, depth}
	if version != "" {
		query += " AND input_version=?"
		args = append(args, version)
	}
	if excludeExpired {
		query += " AND status!='expired'"
	}
	query += " ORDER BY seq DESC LIMIT 1"
	var saved struct {
		CheckRecord
		DeliveryUpdates *checkDeliveryUpdates `json:"delivery_updates"`
	}
	var data string
	var id int64
	var status CheckStatus
	err := q.QueryRowContext(ctx, query, args...).Scan(&id, &status, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// 任务视图只用到检查的状态和提示，不解压单独保存的报告正文。
	if lean, _ := ctx.Value(withoutReportsKey{}).(bool); !lean {
		data, err = restoreCheckBody(ctx, q, id, data)
		if err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal([]byte(data), &saved); err != nil {
		return nil, err
	}
	result := saved.CheckRecord
	if result.Input.Algorithm == "" {
		var old struct {
			Input struct {
				Version string `json:"rules"`
			}
		}
		_ = json.Unmarshal([]byte(data), &old)
		result.HistoricalVersion = old.Input.Version
		if result.HistoricalVersion == "" {
			result.HistoricalVersion = "schema11"
		}
		result.Level = "historical"
		for i := range result.Issues {
			result.Issues[i].Level = "historical"
		}
	}
	if saved.DeliveryUpdates != nil {
		result.Report.Verifications = saved.DeliveryUpdates.Verifications
		result.Report.ValidationNotes = saved.DeliveryUpdates.ValidationNotes
	}
	result.ID, result.Status = id, status
	return &result, nil
}

func (s *Store) LatestCheck(ctx context.Context, task, depth, version string) (*CheckRecord, error) {
	var result *CheckRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = latestCheck(ctx, q, task, depth, version, false)
		return err
	})
	return result, err
}

func (s *Store) PreviousCheck(ctx context.Context, task, depth string) (*CheckRecord, error) {
	var result *CheckRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = latestCheck(ctx, q, task, depth, "", true)
		return err
	})
	return result, err
}

func viewChecks(ctx context.Context, q queryer, view *TaskView, actor Actor) error {
	var labels []string
	lean := ctx
	if full, _ := ctx.Value(withReportsKey{}).(bool); !full {
		lean = context.WithValue(ctx, withoutReportsKey{}, true)
	}
	for _, depth := range []string{"static", "tests"} {
		record, err := latestCheck(lean, q, view.Task.ID, depth, "", false)
		if err != nil {
			return err
		}
		if record == nil {
			continue
		}
		if record.Input.Algorithm == "" {
			view.Checks = append(view.Checks, *record)
			labels = append(labels, depth+": historical "+record.HistoricalVersion)
			continue
		}
		input, err := checkInput(ctx, q, view.Task.WorkspaceID, view.Task.ID, actor, record.Input.Algorithm)
		if err != nil {
			return err
		}
		if record.Input.Algorithm != "" && input.Version() != record.InputVersion {
			record.Status = CheckExpired
			record.Level = "hint"
			for i := range record.Issues {
				record.Issues[i].Level = "hint"
			}
		}
		view.Checks = append(view.Checks, *record)
		labels = append(labels, depth+": "+string(record.Status))
	}
	if len(labels) > 0 {
		view.LatestCheck = strings.Join(labels, "; ")
	}
	var err error
	view.Issues, err = readIssues(ctx, q, view.Task.ID)
	return err
}

/**
 * currentTree 由调用方在分析后重新采集；事务内再核对账本输入和任务状态。
 * 不持有工作区锁，当前结果必须同时满足这两次核对，过期结果只保存历史。
 */
func (s *Store) SaveCheck(ctx context.Context, workspace string, actor Actor, currentTree string, result CheckRecord) (CheckRecord, error) {
	if result.Depth != "static" && result.Depth != "tests" {
		return result, fmt.Errorf("未知检查深度")
	}
	if result.Status != CheckComplete && result.Status != CheckPartial && result.Status != CheckFailed {
		return result, fmt.Errorf("未知检查完成状态")
	}
	result.InputVersion = result.Input.Version()
	result.ComputedStatus = result.Status
	result.FinishedAt = time.Now().UTC()
	rawReport, err := json.Marshal(result.Report)
	if err != nil {
		return result, err
	}
	report, err := prepareBody(ctx, rawReport)
	if err != nil {
		return result, err
	}
	err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		input, err := checkInput(ctx, tx, workspace, result.Input.TaskID, actor, result.Input.Algorithm)
		if err != nil {
			return err
		}
		id, err := currentID(ctx, tx, workspace)
		if err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, workspace, result.Input.TaskID)
		if err != nil {
			return err
		}
		if id != task.ID || (task.State != TaskActive && task.State != TaskPending) || input.Version() != result.InputVersion || currentTree != result.Input.Tree {
			result.Status = CheckExpired
		} else if err := reconcileIssues(ctx, tx, &result); err != nil {
			return err
		}
		if result.Status != CheckExpired {
			if err := saveLocations(ctx, tx, workspace, task.LastObservationID, result.Locations); err != nil {
				return err
			}
		}
		result.Level = HighestLevel(result.Issues)
		if result.Level == "pass" {
			if result.Status != CheckComplete {
				result.Level = "unknown"
			} else {
				result.Level = "no_findings"
			}
		}
		if result.Status == CheckExpired {
			result.Level = "hint"
			for i := range result.Issues {
				result.Issues[i].Level = "hint"
			}
			if err := expireFinishRequests(ctx, tx, workspace, task.ID); err != nil {
				return err
			}
			if task.State == TaskPending {
				task.State = TaskActive
				task.Revision++
				if err := saveTask(ctx, tx, task); err != nil {
					return err
				}
			}
		}
		data, err := checkMetadata(result)
		if err != nil {
			return err
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO checks_v12(task_id,depth,input_version,status,data,report_encoding,report_raw_bytes,report_sha256,report_body) VALUES(?,?,?,?,?,?,?,?,?)`, task.ID, result.Depth, result.InputVersion, result.Status, string(data), report.Encoding, report.RawBytes, report.SHA256, report.Body)
		if err != nil {
			return err
		}
		result.ID, err = insert.LastInsertId()
		return err
	})
	return result, err
}
