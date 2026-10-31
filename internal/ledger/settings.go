package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Scope string

const (
	Default Scope = "default"
	User    Scope = "user"
	Project Scope = "project"
	Task    Scope = "task"
)

type Setting struct {
	Value    string
	Source   Scope
	ScopeID  string
	Revision int64
}

type EffectiveSettings struct {
	Values   map[string]Setting
	Revision string
}

var testLanguages = [...]string{"go", "python", "javascript", "typescript", "java", "rust"}

func DefaultValue(name string) (string, error) {
	switch name {
	case "workflow":
		return "auto", nil
	case "enforcement":
		return "block", nil
	case "analysis.tests", "debug":
		return "off", nil
	}
	parts := strings.Split(name, ".")
	if len(parts) == 3 && parts[0] == "test" {
		for _, language := range testLanguages {
			if parts[1] != language {
				continue
			}
			switch parts[2] {
			case "command", "prepare":
				return "", nil
			case "parallel":
				return "1", nil
			}
		}
	}
	return "", fmt.Errorf("未知设置项 %q", name)
}

func ValidateSetting(name, value string) error {
	if _, err := DefaultValue(name); err != nil {
		return err
	}
	switch {
	case name == "workflow":
		if value != "auto" && value != "semi" {
			return fmt.Errorf("workflow 只能是 auto 或 semi")
		}
	case name == "enforcement":
		if value != "block" && value != "hint" {
			return fmt.Errorf("enforcement 只能是 block 或 hint")
		}
	case name == "analysis.tests" || name == "debug":
		if value != "on" && value != "off" {
			return fmt.Errorf("%s 只能是 on 或 off", name)
		}
	case strings.HasSuffix(name, ".parallel"):
		if value != "1" && value != "2" && value != "3" && value != "4" {
			return fmt.Errorf("测试并行数只能是 1 至 4")
		}
	}
	return nil
}

func (s *Store) SetSetting(ctx context.Context, workspaceID string, scope Scope, name, value string) error {
	if err := ValidateSetting(name, value); err != nil {
		return err
	}
	if scope != User && scope != Project && scope != Task {
		return fmt.Errorf("无效的设置作用范围 %q", scope)
	}
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureSchema(ctx, tx); err != nil {
		return err
	}
	var scopeID string
	if scope != User {
		var projectID, taskID string
		if err := tx.QueryRowContext(ctx, "SELECT project_id, current_task_id FROM workspaces_v11 WHERE id=?", workspaceID).Scan(&projectID, &taskID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("当前目录不在已识别的 git 工作区里")
			}
			return err
		}
		scopeID = projectID
		if scope == Task {
			if taskID == "" {
				return fmt.Errorf("当前工作区没有当前任务，不能保存任务设置")
			}
			scopeID = taskID
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO settings_v11 (scope, scope_id, name, value, revision) VALUES (?, ?, ?, ?, 1)
ON CONFLICT(scope, scope_id, name) DO UPDATE SET value=excluded.value, revision=settings_v11.revision+1
WHERE settings_v11.value != excluded.value`, scope, scopeID, name, value)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Settings(ctx context.Context, workspaceID string) (EffectiveSettings, error) {
	var result EffectiveSettings
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = effectiveSettings(ctx, q, workspaceID, "", true)
		return err
	})
	return result, err
}

func effectiveSettings(ctx context.Context, q queryer, workspaceID, taskID string, current bool) (EffectiveSettings, error) {
	values := make(map[string]Setting)
	names := []string{"workflow", "enforcement", "analysis.tests", "debug"}
	for _, language := range testLanguages {
		for _, field := range []string{"command", "prepare", "parallel"} {
			names = append(names, "test."+language+"."+field)
		}
	}
	for _, name := range names {
		value, _ := DefaultValue(name)
		values[name] = Setting{Value: value, Source: Default}
	}
	// 用一个读语句同时读取当前任务和设置，避免任务切换时混合两套上下文。
	rows, err := q.QueryContext(ctx, `SELECT scope, scope_id, name, value, revision FROM settings_v11
WHERE scope='user'
   OR (scope='project' AND scope_id=(SELECT project_id FROM workspaces_v11 WHERE id=?))
   OR (scope='task' AND scope_id=CASE WHEN ? THEN (SELECT current_task_id FROM workspaces_v11 WHERE id=?) ELSE ? END)
ORDER BY CASE scope WHEN 'user' THEN 0 WHEN 'project' THEN 1 ELSE 2 END, name`, workspaceID, current, workspaceID, taskID)
	if err != nil {
		return EffectiveSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value Setting
		if err := rows.Scan(&value.Source, &value.ScopeID, &name, &value.Value, &value.Revision); err != nil {
			return EffectiveSettings{}, err
		}
		if err := ValidateSetting(name, value.Value); err != nil {
			return EffectiveSettings{}, fmt.Errorf("账本中的设置无效: %w", err)
		}
		values[name] = value
	}
	if err := rows.Err(); err != nil {
		return EffectiveSettings{}, err
	}
	// 调试开关不改变接续、授权和检查的版本，避免开启记录触发业务重算。
	versioned := make(map[string]Setting, len(values))
	for name, value := range values {
		if name != "debug" {
			versioned[name] = value
		}
	}
	encoded, err := json.Marshal(versioned)
	if err != nil {
		return EffectiveSettings{}, err
	}
	// 只包含实际生效的条目，低优先级但被覆盖的修改不会使检查失效。
	digest := sha256.Sum256(encoded)
	return EffectiveSettings{Values: values, Revision: hex.EncodeToString(digest[:])}, nil
}
