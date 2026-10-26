package ledger

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type debugContextKey struct{}

type debugEntry struct {
	Format      int       `json:"format"`
	Time        time.Time `json:"time"`
	TaskID      string    `json:"task_id"`
	WorkspaceID string    `json:"workspace_id"`
	Actor       Actor     `json:"actor"`
	PID         int       `json:"pid"`
	TraceID     string    `json:"trace_id"`
	Sequence    int       `json:"sequence"`
	Operation   string    `json:"operation"`
	Phase       string    `json:"phase"`
	ElapsedMS   float64   `json:"elapsed_ms"`
	Details     any       `json:"details,omitempty"`
}

type DebugTrace struct {
	mu          sync.Mutex
	store       *Store
	workspace   string
	task        string
	actor       Actor
	operation   string
	id          string
	started     time.Time
	sequence    int
	pending     []debugEntry
	reportError func(error)
	failed      bool
}

/**
 * StartDebug 只为最外层调用建立追踪。没有任务编号的创建过程暂存在本次调用内，
 * 成功取得编号后才写任务文件；target 为 nil 时采用当前任务，指向空字符串时
 * 采用尚未创建任务的项目和用户设置。诊断失败不进入业务错误返回值。
 */
func (s *Store) StartDebug(ctx context.Context, workspace string, target *string, actor Actor, operation string, reportError func(error)) (context.Context, *DebugTrace) {
	if ctx.Value(debugContextKey{}) != nil {
		return ctx, nil
	}
	var settings EffectiveSettings
	var initial *TaskRecord
	var current, task string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		current, err = currentID(ctx, q, workspace)
		if err != nil {
			return err
		}
		task = current
		if target != nil {
			task = *target
		}
		settings, err = effectiveSettings(ctx, q, workspace, task, false)
		if err != nil || settings.Values["debug"].Value != "on" || task == "" {
			return err
		}
		value, err := loadTask(ctx, q, workspace, task)
		initial = &value
		return err
	})
	if err != nil || settings.Values["debug"].Value != "on" {
		return ctx, nil
	}
	trace := &DebugTrace{store: s, workspace: workspace, task: task, actor: actor, operation: operation, id: rand.Text(), started: time.Now(), reportError: reportError}
	build := map[string]string{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}
	if info, ok := debug.ReadBuildInfo(); ok {
		build["version"] = info.Main.Version
		for _, setting := range info.Settings {
			if strings.HasPrefix(setting.Key, "vcs.") {
				build[setting.Key] = setting.Value
			}
		}
	}
	trace.record("operation.start", map[string]any{"task": initial, "current_task_id": current, "settings": settings, "build": build})
	return context.WithValue(ctx, debugContextKey{}, trace), trace
}

func DebugEnabled(ctx context.Context) bool {
	return ctx.Value(debugContextKey{}) != nil
}

func DebugRecord(ctx context.Context, phase string, details any) {
	if trace, ok := ctx.Value(debugContextKey{}).(*DebugTrace); ok {
		trace.record(phase, details)
	}
}

/**
 * DebugBindTask 只接受本次业务操作返回的任务编号，不能用调用结束时的全局当前
 * 任务猜测归属：其他 session 可能已经切换了任务。
 */
func DebugBindTask(ctx context.Context, task string) {
	trace, ok := ctx.Value(debugContextKey{}).(*DebugTrace)
	if !ok || task == "" || task == trace.task {
		return
	}
	transition := map[string]any{"from": trace.task, "to": task}
	if trace.task != "" {
		trace.record("task.switch", transition)
	}
	trace.task = task
	trace.record("task.switch", transition)
}

/**
 * Finish 在业务处理之后补充账本视图。即使业务预算已结束，也只使用独立的
 * 短读取预算；没有完成的读取显式记录，不能把缺失快照当作空任务或成功。
 */
func (t *DebugTrace) Finish(ctx context.Context, result any, failure error) {
	if t == nil {
		return
	}
	read, cancel := context.WithTimeout(context.WithoutCancel(ctx), NormalWait)
	defer cancel()
	if t.task != "" {
		view, err := t.store.TaskView(read, t.workspace, t.task, t.actor)
		if err != nil {
			t.record("task.snapshot_failed", map[string]any{"error": err.Error()})
		} else {
			// 其他任务只保留编号联系，避免把不相关任务的完整内容复制进当前文件。
			view.Candidates = nil
			t.record("task.snapshot", view)
		}
	}
	details := map[string]any{"result": result, "status": "completed"}
	if failure != nil {
		details["status"], details["error"] = "failed", failure.Error()
	}
	t.record("operation.end", details)
}

func (t *DebugTrace) record(phase string, details any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed {
		return
	}
	now := time.Now()
	t.sequence++
	entry := debugEntry{Format: 2, Time: now, TaskID: t.task, WorkspaceID: t.workspace, Actor: t.actor, PID: os.Getpid(), TraceID: t.id, Sequence: t.sequence, Operation: t.operation, Phase: phase, ElapsedMS: float64(now.Sub(t.started).Microseconds()) / 1000, Details: details}
	if t.task == "" {
		t.pending = append(t.pending, entry)
		return
	}
	for _, pending := range t.pending {
		pending.TaskID = t.task
		if err := t.append(pending); err != nil {
			t.fail(err)
			return
		}
	}
	t.pending = nil
	if err := t.append(entry); err != nil {
		t.fail(err)
	}
}

func (t *DebugTrace) append(entry debugEntry) error {
	if !strings.HasPrefix(entry.TaskID, "t") || len(entry.TaskID) < 2 || strings.Trim(entry.TaskID[1:], "0123456789") != "" {
		return fmt.Errorf("无效的调试任务编号 %q", entry.TaskID)
	}
	root, err := os.OpenRoot(t.store.dataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	dir := filepath.Join("debug", entry.Time.Format("2006-01-02"))
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if entry.Details != nil {
		entry.Details = debugValue{root: root, directory: dir, task: entry.TaskID, value: entry.Details}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := root.OpenFile(filepath.Join(dir, entry.TaskID+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	// 每行用一次追加写入，避免不同 hook/MCP 进程把一条 JSON 拆开交错。
	data = append(data, '\n')
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, file.Close())
}

func (t *DebugTrace) fail(err error) {
	t.failed = true
	if t.reportError != nil {
		t.reportError(fmt.Errorf("debug 记录未完成（%s，任务 %s，调用 %s）：%w", t.operation, t.task, t.id, err))
	}
}
