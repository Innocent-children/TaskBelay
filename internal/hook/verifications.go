package hook

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CommandScript(e event.Event) shell.Script {
	if e.Command == nil {
		return shell.Script{}
	}
	return shell.Parse(e.Command.Text, e.WorkingDir, e.Command.Shell, map[event.Shell]shell.Parser{event.POSIX: posix.Parser{}, event.PowerShell: pwsh.Parser{}})
}
func commandRecord(e event.Event) ledger.VerificationRecord {
	r := ledger.VerificationRecord{Algorithm: check.Algorithm, Actor: ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID, CallID: e.ToolCallID}, RequestDirectory: e.WorkingDir, Command: CommandScript(e)}
	if e.Command != nil {
		r.RequestHash = fmt.Sprintf("%x", sha256.Sum256([]byte(string(e.Command.Shell)+"\x00"+e.WorkingDir+"\x00"+e.Command.Text)))
		if e.Command.Background {
			for i := range r.Command.Segments {
				r.Command.Segments[i].Background = true
			}
		}
	}
	r.CommandKey = check.CommandKey(r)
	r.EnvironmentID = r.Command.HashEnvironment(os.Environ())
	r.Scope.EnvironmentChanged = environmentChange(r.Command)
	return r
}

// shell 的赋值及环境操作在输入边界识别；不据此识别验证工具。
func environmentChange(script shell.Script) bool {
	for _, s := range script.Segments {
		if len(s.Environment) > 0 || len(s.Unset) > 0 || s.ClearEnvironment {
			return true
		}
		if len(s.Args) > 0 {
			switch strings.ToLower(s.Args[0]) {
			case "export", "unset", "source", ".", "set-item", "remove-item":
				return true
			}
			if strings.HasPrefix(strings.ToLower(s.Args[0]), "$env:") {
				return true
			}
		}
	}
	return false
}

func toolIdentity(script shell.Script) string {
	var parts []string
	for _, s := range script.Segments {
		if len(s.Args) == 0 {
			return ""
		}
		program := s.Args[0]
		if len(s.Prefix) > 0 {
			program = s.Prefix[0]
		}
		resolved := program
		var err error
		if strings.ContainsAny(program, "/\\") {
			if !filepath.IsAbs(program) {
				resolved = filepath.Join(s.Dir, program)
			}
		} else {
			resolved, err = exec.LookPath(program)
		}
		if err != nil {
			return ""
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return ""
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return ""
		}
		identity, err := platform.FileIdentity(resolved)
		if err != nil {
			return ""
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%d:%d:%s", resolved, identity, info.Size(), info.ModTime().UnixNano(), info.Mode()))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

func (t Tasks) BeforeCommand(ctx context.Context, e event.Event) (event.Decision, error) {
	if e.Command == nil {
		return event.Decision{}, nil
	}
	record := commandRecord(e)
	record.WorkspaceID = t.Workspace.ID
	existing, err := t.Store.VerificationByCall(ctx, t.Workspace.ID, record.Actor)
	if err != nil {
		return event.Decision{}, err
	}
	if existing != nil {
		if existing.RequestHash != record.RequestHash {
			return event.Decision{}, fmt.Errorf("命令身份与原请求不一致")
		}
		return verificationDecision(*existing), nil
	}
	task, observation, err := t.Store.CommandContext(ctx, t.Workspace.ID)
	if err != nil {
		return event.Decision{}, err
	}
	record.TaskID, record.InputRevision, record.Tree = task.ID, task.RecordRevision, observation.Tree
	previous, err := t.Store.PreviousVerification(ctx, t.Workspace.ID, record.Actor, record.CommandKey)
	if err != nil {
		return event.Decision{}, err
	}
	if previous != nil {
		record.PreviousID = previous.ID
	}
	known, err := check.KnownValidation(ctx, t.Store, t.Workspace.ID, record)
	if err != nil {
		return event.Decision{}, err
	}
	if known && task.ID != "" {
		view, observed, err := t.observeEvent(ctx, e)
		if err != nil {
			return event.Decision{}, err
		}
		if view.Task == nil {
			return event.Decision{}, fmt.Errorf("当前任务已变化")
		}
		task = *view.Task
		record.InputRevision, record.Tree = task.RecordRevision, observed.Tree
		record.Scope.Tool = toolIdentity(record.Command)
		if err = check.Verification(ctx, t.Store, t.Workspace, task, &record); err != nil {
			record.Scope.Complete = false
			record.Scope.Reason = check.CommandResultReason(err)
		}
		current, err := t.Workspace.Capture(ctx)
		if err != nil || !current.Complete() || current.Tree != record.Tree {
			record.Scope.Complete = false
			record.Issue = nil
		}
	}
	record, err = t.Store.BeginVerification(ctx, record)
	if err != nil {
		return event.Decision{}, err
	}
	return verificationDecision(record), nil
}

func verificationDecision(record ledger.VerificationRecord) event.Decision {
	text := check.FormatVerification(record)
	d := event.Decision{AdditionalContext: text}
	if record.Issue != nil {
		action := "验证提示已输出"
		if record.Denied {
			action = "验证拒绝已输出"
		}
		d.Records = append(d.Records, feedbackRecord(record.TaskID, action, verificationCommand(record.Command), record.Issue.Advice, "verification:"+record.ID))
	}
	if record.Denied {
		d.DenyTool = text
	}
	return d
}

func (t Tasks) AfterCommand(ctx context.Context, e event.Event) (event.Decision, error) {
	if e.Command == nil {
		return event.Decision{}, nil
	}
	request := commandRecord(e)
	request.WorkspaceID = t.Workspace.ID
	existing, err := t.Store.VerificationByCall(ctx, t.Workspace.ID, request.Actor)
	if err != nil {
		return event.Decision{}, err
	}
	if existing != nil && existing.RequestDirectory != e.WorkingDir {
		copy := e
		copy.WorkingDir = existing.RequestDirectory
		request = commandRecord(copy)
		request.WorkspaceID = t.Workspace.ID
	}
	if existing != nil && existing.RequestHash != request.RequestHash {
		return event.Decision{}, fmt.Errorf("命令后置事件与原请求不一致")
	}
	result, err := t.Store.EndVerification(ctx, t.Workspace.ID, request.Actor, e.Result)
	if errors.Is(err, sql.ErrNoRows) {
		task, _, readErr := t.Store.CommandContext(ctx, t.Workspace.ID)
		if readErr != nil {
			return event.Decision{}, readErr
		}
		request.TaskID, request.InputRevision, request.PostOnly = task.ID, task.RecordRevision, true
		previous, readErr := t.Store.PreviousVerification(ctx, t.Workspace.ID, request.Actor, request.CommandKey)
		if readErr != nil {
			return event.Decision{}, readErr
		}
		if previous != nil {
			request.PreviousID = previous.ID
		}
		if _, err = t.Store.BeginVerification(ctx, request); err != nil {
			return event.Decision{}, err
		}
		result, err = t.Store.EndVerification(ctx, t.Workspace.ID, request.Actor, e.Result)
	}
	if err != nil {
		return event.Decision{}, err
	}
	if result.Record.Denied || !result.Changed {
		return event.Decision{}, nil
	}
	view, observed, err := t.observeEvent(ctx, e)
	if err != nil {
		return event.Decision{}, err
	}
	output := ""
	if e.Result != nil {
		output = e.Result.Output
	}
	if view.Task != nil {
		if err = check.CompleteVerification(ctx, t.Store, t.Workspace, *view.Task, result.Record, observed.Tree, output); err != nil {
			return event.Decision{}, err
		}
	}
	t.recordSession(request.Actor, verificationRecord(result.Record))
	return event.Decision{}, nil
}
