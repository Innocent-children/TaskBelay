package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func readIssues(ctx context.Context, q queryer, task string) ([]Issue, error) {
	rows, err := q.QueryContext(ctx, "SELECT data,status FROM issues_v11 WHERE task_id=? ORDER BY seq", task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Issue
	for rows.Next() {
		var data, status string
		var issue Issue
		if err := rows.Scan(&data, &status); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &issue); err != nil {
			return nil, err
		}
		issue.Status = status
		result = append(result, issue)
	}
	return result, rows.Err()
}

func saveIssue(ctx context.Context, tx *sql.Tx, task string, issue Issue) error {
	data, err := json.Marshal(issue)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO issues_v11(task_id,identity,occurrence,status,data) VALUES(?,?,?,?,?)
ON CONFLICT(task_id,identity,occurrence) DO UPDATE SET status=excluded.status,data=excluded.data`, task, issue.Identity, issue.Occurrence, issue.Status, string(data))
	return err
}

func relocated(at string, moves map[string]string) string {
	if to := moves[at]; to != "" {
		return to
	}
	// 参数和字段继承所属声明的迁移，但不继承整段声明的保留决定。
	longest := ""
	for from := range moves {
		if len(from) > len(longest) && strings.HasPrefix(at, from+".") {
			longest = from
		}
	}
	if longest != "" {
		return moves[longest] + strings.TrimPrefix(at, longest)
	}
	return at
}

func reconcileIssues(ctx context.Context, tx *sql.Tx, result *CheckRecord) error {
	result.Unconfirmed = nil
	old, err := readIssues(ctx, tx, result.Input.TaskID)
	if err != nil {
		return err
	}
	latest := map[string]Issue{}
	for _, issue := range old {
		latest[issue.Identity] = issue
	}
	seen, evaluated := map[string]bool{}, map[string]bool{}
	for _, scope := range result.Evaluated {
		evaluated[scope] = true
	}
	for i := range result.Issues {
		issue := &result.Issues[i]
		if issue.Status == "" {
			issue.Status = "open"
		}
		issue.LogicalAt = issue.At
		var previous *Issue
		for _, prior := range latest {
			if prior.Rule == issue.Rule && prior.FactKey == issue.FactKey && relocated(prior.At, result.Relocations) == issue.At {
				if previous != nil {
					previous = nil
					break
				}
				copy := prior
				previous = &copy
			}
		}
		if previous != nil {
			issue.Identity, issue.LogicalAt = previous.Identity, previous.LogicalAt
			issue.Occurrence = previous.Occurrence
			if previous.Status == "resolved" {
				issue.Occurrence++
			} else {
				issue.Blocked = previous.Blocked
			}
		} else {
			issue.Identity = fmt.Sprintf("%x", sha256.Sum256([]byte(result.Input.TaskID+"\x00"+issue.Rule+"\x00"+issue.LogicalAt+"\x00"+issue.FactKey)))
			issue.Occurrence = 1
		}
		issue.ID = fmt.Sprintf("i-%s-%d", issue.Identity[:20], issue.Occurrence)
		for _, note := range result.Report.Decisions {
			target := note.Target
			if target == nil || target.Effect != "retain" || relocated(target.At, result.Relocations) != issue.At || target.Kind == "issue" && target.ID != issue.ID {
				continue
			}
			issue.Status = "retained"
			issue.HandledBy = append(issue.HandledBy, note.ID)
			if note.Citation != nil {
				issue.Evidence = append(issue.Evidence, note.Citation.SourceID+": "+note.Citation.Quote)
			}
		}
		seen[issue.Identity] = true
		if err := saveIssue(ctx, tx, result.Input.TaskID, *issue); err != nil {
			return err
		}
	}
	for _, issue := range latest {
		if seen[issue.Identity] || issue.Status == "resolved" {
			continue
		}
		at := relocated(issue.At, result.Relocations)
		if evaluated[CheckScope(issue.Rule, at)] || evaluated[CheckScope(issue.Rule, "*")] {
			issue.Status = "resolved"
			if err := saveIssue(ctx, tx, result.Input.TaskID, issue); err != nil {
				return err
			}
		} else if issue.Status == "open" {
			if at != issue.At {
				issue.At = at
				if err := saveIssue(ctx, tx, result.Input.TaskID, issue); err != nil {
					return err
				}
			}
			issue.At, issue.Level, issue.BlockOnDelivery = at, "hint", false
			result.Unconfirmed = append(result.Unconfirmed, issue)
		}
	}
	return nil
}

func (s *Store) Issues(ctx context.Context, task string) ([]Issue, error) {
	var result []Issue
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = readIssues(ctx, q, task)
		return err
	})
	return result, err
}

func blockIssues(ctx context.Context, tx *sql.Tx, workspace string, actor Actor, issues []Issue) ([]Issue, error) {
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM check_feedback_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=?", workspace, actor.Agent, actor.SessionID, actor.RoundID).Scan(&count); err != nil || count >= 3 {
		return nil, err
	}
	var candidates []Issue
	for _, issue := range issues {
		if issue.Level != "block" || issue.Status != "open" {
			continue
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM issue_blocks_v11 WHERE issue_id=? AND agent=? AND session_id=?", issue.ID, actor.Agent, actor.SessionID).Scan(&count); err != nil {
			return nil, err
		}
		if count == 0 {
			candidates = append(candidates, issue)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	insert, err := tx.ExecContext(ctx, "INSERT INTO check_feedback_v11(workspace_id,agent,session_id,round_id,at) VALUES(?,?,?,?,?)", workspace, actor.Agent, actor.SessionID, actor.RoundID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	id, err := insert.LastInsertId()
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		issue := &candidates[i]
		_, err := tx.ExecContext(ctx, "INSERT INTO issue_blocks_v11 VALUES(?,?,?,?,?)", issue.ID, actor.Agent, actor.SessionID, actor.RoundID, id)
		if err != nil {
			return nil, err
		}
		issue.Blocked = append(issue.Blocked, Actor{Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID})
		// 发生记录中的事实不变，只追加实际反馈过的 session。
		data, err := json.Marshal(issue)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE issues_v11 SET data=? WHERE identity=? AND occurrence=?", string(data), issue.Identity, issue.Occurrence); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}
