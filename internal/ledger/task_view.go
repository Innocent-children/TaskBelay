package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"
)

type TaskView struct {
	CurrentTaskID      string               `json:"current_task_id"`
	Task               *TaskRecord          `json:"task,omitempty"`
	Candidates         []TaskRecord         `json:"candidates"`
	Settings           EffectiveSettings    `json:"settings"`
	Requirements       []Requirement        `json:"requirements"`
	History            []Requirement        `json:"requirement_history"`
	Messages           []Message            `json:"messages"`
	Sources            []Source             `json:"sources"`
	Notes              []Note               `json:"notes"`
	EffectiveDecisions []Note               `json:"effective_decisions"`
	WorkspaceUpdates   []FileChange         `json:"workspace_updates"`
	PendingLocations   []Location           `json:"pending_locations"`
	PendingActions     []string             `json:"pending_actions,omitempty"`
	LatestCheck        string               `json:"latest_check"`
	Checks             []CheckRecord        `json:"checks"`
	Issues             []Issue              `json:"issues"`
	Verifications      []VerificationRecord `json:"verifications"`
	Plans              PlanView             `json:"plans"`
}

func taskList(ctx context.Context, q queryer, workspace string) ([]TaskRecord, error) {
	rows, err := q.QueryContext(ctx, "SELECT data FROM tasks_v11 WHERE workspace_id=? ORDER BY seq", workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []TaskRecord{}
	for rows.Next() {
		var data string
		var task TaskRecord
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) TaskView(ctx context.Context, workspace, id string, actor Actor) (TaskView, error) {
	view := TaskView{LatestCheck: "not_checked", Requirements: []Requirement{}, History: []Requirement{}, Messages: []Message{}, Sources: []Source{}, Notes: []Note{}, EffectiveDecisions: []Note{}, WorkspaceUpdates: []FileChange{}, PendingLocations: []Location{}}
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		view.CurrentTaskID, err = currentID(ctx, q, workspace)
		if err != nil {
			return err
		}
		if id == "" {
			id = view.CurrentTaskID
		}
		if id == "" {
			view.PendingActions, err = unboundActions(ctx, q, workspace)
			if err != nil {
				return err
			}
		}
		view.Candidates, err = taskList(ctx, q, workspace)
		if err != nil {
			return err
		}
		view.Settings, err = effectiveSettings(ctx, q, workspace, id, false)
		if err != nil {
			return err
		}
		view.Verifications, err = readVerifications(ctx, q, workspace, id)
		if err != nil {
			return err
		}
		var startMessage string
		if id != "" {
			task, err := loadTask(ctx, q, workspace, id)
			if err != nil {
				return err
			}
			task.Workflow, task.SettingsRevision = view.Settings.Values["workflow"].Value, view.Settings.Revision
			view.Task = &task
			view.Plans, err = readPlans(ctx, q, task)
			if err != nil {
				return err
			}
			startMessage = task.StartMessageID
			rows, err := q.QueryContext(ctx, "SELECT data FROM requirement_versions_v11 WHERE task_id=? ORDER BY rowid", id)
			if err != nil {
				return err
			}
			indices := map[string]int{}
			for rows.Next() {
				var data string
				var requirement Requirement
				if err := rows.Scan(&data); err != nil {
					rows.Close()
					return err
				}
				if err := json.Unmarshal([]byte(data), &requirement); err != nil {
					rows.Close()
					return err
				}
				view.History = append(view.History, requirement)
				if i, ok := indices[requirement.ID]; ok {
					view.Requirements[i] = requirement
				} else {
					indices[requirement.ID] = len(view.Requirements)
					view.Requirements = append(view.Requirements, requirement)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			view.Notes, err = readNotes(ctx, q, id)
			for i := range view.Requirements {
				if err := currentLocations(ctx, q, &view.Requirements[i]); err != nil {
					return err
				}
			}
			if err != nil {
				return err
			}
			view.EffectiveDecisions = effectiveDecisions(view.Notes, task, view.Settings, actor)
			view.PendingActions, err = pendingActions(ctx, q, task, view.Requirements)
			if err != nil {
				return err
			}
			if err := viewChecks(ctx, q, &view, actor); err != nil {
				return err
			}
			rows, err = q.QueryContext(ctx, `SELECT c.data FROM file_changes_v11 c JOIN observations_v11 o ON o.id=c.observation_id WHERE o.workspace_id=? AND o.task_id=? ORDER BY c.id`, workspace, id)
			if err != nil {
				return err
			}
			for rows.Next() {
				var data string
				var change FileChange
				if err := rows.Scan(&data); err != nil {
					rows.Close()
					return err
				}
				if err := json.Unmarshal([]byte(data), &change); err != nil {
					rows.Close()
					return err
				}
				if change.Kind == "workspace_update" {
					view.WorkspaceUpdates = append(view.WorkspaceUpdates, change)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			rows, err = q.QueryContext(ctx, "SELECT data FROM task_updates_v11 WHERE task_id=? ORDER BY observation_id", id)
			if err != nil {
				return err
			}
			for rows.Next() {
				var data string
				var changes []FileChange
				if err := rows.Scan(&data); err != nil {
					rows.Close()
					return err
				}
				if err := json.Unmarshal([]byte(data), &changes); err != nil {
					rows.Close()
					return err
				}
				view.WorkspaceUpdates = append(view.WorkspaceUpdates, changes...)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
		rows, err := q.QueryContext(ctx, `SELECT data FROM messages_v11 WHERE workspace_id=? AND seq>=COALESCE((SELECT seq FROM messages_v11 WHERE id=?),0) ORDER BY seq`, workspace, startMessage)
		if err != nil {
			return err
		}
		for rows.Next() {
			var message Message
			var data string
			if err := rows.Scan(&data); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(data), &message); err != nil {
				rows.Close()
				return err
			}
			view.Messages = append(view.Messages, message)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		ids := map[string]bool{}
		add := func(c *Citation) {
			if c != nil {
				ids[c.SourceID] = true
			}
		}
		for _, message := range view.Messages {
			ids[message.ID] = true
		}
		if view.Task != nil {
			ids[view.Task.StartMessageID] = true
			add(view.Task.Cancellation)
		}
		for _, req := range view.History {
			add(req.Citation)
			for _, location := range req.ImplementedBy {
				add(location.Citation)
			}
		}
		for _, note := range view.Notes {
			add(note.Citation)
		}
		loaded := map[string]bool{}
		knownMessages := map[string]bool{}
		for _, message := range view.Messages {
			knownMessages[message.ID] = true
		}
		var readSource func(string) error
		readSource = func(id string) error {
			if loaded[id] {
				return nil
			}
			loaded[id] = true
			source, err := loadSource(ctx, q, workspace, id)
			if err != nil {
				return err
			}
			view.Sources = append(view.Sources, source)
			if source.Kind == "user_message" && !knownMessages[id] {
				var data string
				if err := q.QueryRowContext(ctx, "SELECT data FROM messages_v11 WHERE workspace_id=? AND id=?", workspace, id).Scan(&data); err != nil {
					return err
				}
				var message Message
				if err := json.Unmarshal([]byte(data), &message); err != nil {
					return err
				}
				view.Messages = append(view.Messages, message)
				knownMessages[id] = true
			}
			if source.Confirmation != nil {
				return readSource(source.Confirmation.SourceID)
			}
			return nil
		}
		for id := range ids {
			if err := readSource(id); err != nil {
				return err
			}
		}
		sort.Slice(view.Sources, func(i, j int) bool { return view.Sources[i].ID < view.Sources[j].ID })
		sort.Slice(view.Messages, func(i, j int) bool { return view.Messages[i].Seq < view.Messages[j].Seq })
		return nil
	})
	return view, err
}

/**
 * 仅在分支实际变化时自动选择唯一的暂停任务。无匹配或多匹配时保留候选，
 * 不依据最近活动猜测用户意图；HEAD 变化不会重置原始起点。
 */
func (s *Store) ReconcileBranch(ctx context.Context, workspace, current, previousBranch string, observationID int64, updates map[string][]FileChange) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := checkCurrent(ctx, tx, workspace, current); err != nil {
			return err
		}
		observation, err := checkObservation(ctx, tx, workspace, observationID)
		if err != nil {
			return err
		}
		branch := previousBranch
		if current != "" {
			task, err := loadTask(ctx, tx, workspace, current)
			if err != nil {
				return err
			}
			branch = task.Context.Branch
		}
		if branch == observation.Context.Branch {
			return nil
		}
		tasks, err := taskList(ctx, tx, workspace)
		if err != nil {
			return err
		}
		var matches []TaskRecord
		for _, task := range tasks {
			if task.State == TaskPaused && task.Context.Branch == observation.Context.Branch {
				matches = append(matches, task)
			}
		}
		if len(matches) == 1 {
			_, err := switchTask(ctx, tx, Call{WorkspaceID: workspace}, current, observation, StartTaskInput{TaskID: matches[0].ID}, updates[matches[0].ID])
			return err
		}
		if err := pauseTask(ctx, tx, workspace, current, time.Now().UTC()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE workspaces_v11 SET current_task_id='' WHERE id=?", workspace)
		return err
	})
}
