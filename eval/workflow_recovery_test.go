package eval_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

func TestWorkflowRecovery(t *testing.T) {
	t.Run("WF-13", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("工具恢复使用临时 POSIX 启动器；Windows 原生流程单独记录")
		}
		goPath, err := exec.LookPath("go")
		if err != nil {
			t.Fatal(err)
		}
		p := newWorkflowProbeWithFiles(t, "WF-13", map[string]string{
			"go.mod":        "module example.test/cache\n\ngo 1.26\n",
			"range.go":      "package cache\nfunc Allowed(n int) bool {return n<5}\n",
			"range_test.go": "package cache\nimport \"testing\"\nfunc TestAllowed(t *testing.T){if !Allowed(1){t.Fatal(\"expected true\")}}\n",
		})
		message := p.message("方案按 A 做，直接改")
		task := p.start(ledger.StartTaskInput{Title: "cache inputs", Workflow: "semi"})
		plan := firstPlan()
		plan.Files, plan.Entries = []ledger.PlanFile{{Path: "range.go", Change: "modify", Note: "范围判断"}}, nil
		confirmedRevision := int64(0)
		confirm := func() {
			plan.BaseRevision = confirmedRevision
			saved, err := p.service.SavePlan(t.Context(), p.nextActor(), plan)
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.service.ConfirmPlan(t.Context(), p.nextActor(), ledger.PlanConfirmInput{Version: saved.Plan.Version, SourceID: message.ID, Quote: message.Text})
			if err != nil {
				t.Fatal(err)
			}
			confirmedRevision = result.View.Revision
		}
		confirm()
		p.replace("range.go", "n<5", "n<6")
		saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "范围判断", Acceptance: "边界正确", ImplementedBy: &[]ledger.Location{{At: "range.go:Allowed", Purpose: "当前需求"}}})
		run := func(depth string) ledger.CheckRecord {
			r, err := p.service.Check(t.Context(), p.nextActor(), check.RunInput{Depth: depth})
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		first := run("static")
		if first.Status != ledger.CheckComplete || run("static").ID != first.ID {
			t.Fatal("unchanged static input was not reused", first.Status, first.Missing)
		}
		p.resume()
		message = p.message("方案说明按更新后的步骤做，直接改")
		plan.Content += "\n补充当前步骤的执行说明。"
		confirm()
		second := run("static")
		if second.ID == first.ID || second.Input.Tree != first.Input.Tree || second.Input.RequirementRevision != first.Input.RequirementRevision || second.Input.PlanRevision == first.Input.PlanRevision {
			t.Fatal("plan update did not invalidate only the same-code input", first.Input, second.Input)
		}
		p.resume()
		if err := p.service.Store.SetSetting(t.Context(), p.service.Workspace.ID, ledger.Task, "analysis.tests", "on"); err != nil {
			t.Fatal(err)
		}
		third := run("static")
		if third.ID == second.ID || third.Input.Tree != second.Input.Tree || third.Input.AnalysisSettings["analysis.tests"] != "on" {
			t.Fatal("analysis setting reused an old result")
		}
		p.resume()
		launcher := filepath.Join(t.TempDir(), "go")
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
		if err := p.service.Store.SetSetting(t.Context(), p.service.Workspace.ID, ledger.Task, "test.go.command", quote(launcher)+" test ./..."); err != nil {
			t.Fatal(err)
		}
		missing := run("tests")
		if missing.Status != ledger.CheckPartial || missing.Report.Dynamic == nil || missing.Report.Dynamic.Complete {
			t.Fatal("missing tool was presented as a complete check", missing.Status, missing.Missing)
		}
		p.resume()
		if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec "+quote(goPath)+" \"$@\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		repaired := run("tests")
		if repaired.Status != ledger.CheckComplete || repaired.ID == missing.ID || repaired.InputVersion != missing.InputVersion || repaired.Report.Dynamic == nil || !repaired.Report.Dynamic.Complete {
			t.Fatal("repaired tool did not complete the same input", repaired.Status, repaired.Missing)
		}
		view := getTask(t, p.service, p.actor, task.ID)
		if view.Task.OriginTree != task.OriginTree {
			t.Fatal("cache changes reset task origin")
		}
		p.complete()
	})
	t.Run("WF-14", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-14")
		p.start(ledger.StartTaskInput{Title: "concurrent revisions"})
		r := saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "目标", Acceptance: "原验收"})
		a := p.actor
		p.actor.Agent, p.actor.SessionID = "claude", "other"
		p.send(event.Event{Kind: event.SessionStart, StartMode: event.NewSession}, "", false)
		p.message("补充 B 的验收")
		b := p.actor
		p.actor = a
		first := saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{ID: r.ID, BaseRevision: &r.Revision, Content: r.Content, Acceptance: "A 的验收"})
		p.actor = b
		_, err := p.service.SaveRequirement(t.Context(), p.nextActor(), ledger.RequirementInput{ID: r.ID, BaseRevision: &r.Revision, Content: r.Content, Acceptance: "B 的验收"})
		var conflict *ledger.RequirementConflict
		if !errors.As(err, &conflict) || conflict.Latest.Revision != first.Revision || conflict.Latest.Acceptance != first.Acceptance {
			t.Fatal("conflict did not provide the latest revision", err)
		}
		merged := saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{ID: r.ID, BaseRevision: &first.Revision, Content: r.Content, Acceptance: "A 的验收；B 的验收"})
		view := getTask(t, p.service, p.actor, "")
		if merged.Revision != 3 || len(view.History) != 3 || view.History[1].Acceptance != first.Acceptance || view.Requirements[0].Acceptance != merged.Acceptance {
			t.Fatal("conflict or merge lost a successful update")
		}
		p.resume()
		p.actor = a
		p.complete()
	})
	t.Run("WF-15", func(t *testing.T) {
		p := newWorkflowProbeWithFiles(t, "WF-15", map[string]string{"main.go": "package main\nfunc main(){}\n", "work.md": "base\nkeep\n"})
		p.message("完成当前功能")
		task := p.start(ledger.StartTaskInput{Title: "stable issue"})
		p.replace("main.go", "func main(){}", "func main(){}\nfunc unused()int{return 7}")
		first, err := p.service.Check(t.Context(), p.nextActor(), check.RunInput{Depth: "static"})
		if err != nil || first.Status != ledger.CheckComplete {
			t.Fatal("first issue analysis failed", first.Missing, err)
		}
		var id string
		for _, issue := range first.Issues {
			if issue.Rule == "D1" && issue.At == "main.go:unused" {
				id = issue.ID
			}
		}
		if id == "" {
			t.Fatal("fixture did not establish the new unused internal declaration")
		}
		finish := func(allowed string) {
			if _, err := p.service.Finish(t.Context(), p.nextActor(), ledger.FinishInput{}); err != nil {
				t.Fatal(err)
			}
			p.send(event.Event{Kind: event.Stop}, allowed, false)
		}
		finish("D1:" + id)
		p.replace("work.md", "base", "other file changed")
		git(t, p.service.Workspace.Root, "add", ".")
		git(t, p.service.Workspace.Root, "commit", "-qm", "same issue remains")
		addNote(t, p.service, p.nextActor(), ledger.NoteInput{Kind: "next_step", Text: "当前进展已保存，继续处理原问题"})
		p.replace("main.go", "unused", "renamed")
		finish("")
		latest, err := p.service.Store.LatestCheck(t.Context(), task.ID, "static", "")
		if err != nil || latest == nil || !slices.ContainsFunc(latest.Issues, func(issue ledger.Issue) bool {
			return issue.Rule == "D1" && issue.ID == id && issue.At == "main.go:renamed"
		}) {
			t.Fatal("unrelated edit, commit, note or rename reset issue identity", err)
		}
		p.complete()
	})
	t.Run("WF-16", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-16")
		task := p.start(ledger.StartTaskInput{Title: "many requirements"})
		for range 12 {
			saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: strings.Repeat("只实现当前要求", 100), Acceptance: "保持范围"})
		}
		p.resume()
		write(t, p.service.Workspace.Root, "work.txt", "user version\nkeep\n")
		d := p.send(event.Event{Kind: event.SessionStart, StartMode: event.Resume}, "", false)
		text := d.AdditionalContext + d.UserMessage
		if summary.Tokens(text) > 2000 || !strings.Contains(text, "共 12 条") || !strings.Contains(text, "省略") || !strings.Contains(text, "work.txt") || !strings.Contains(text, task.ID) || !strings.HasSuffix(text, summary.Footer) {
			t.Fatal("summary budget lost current changes or the full-entry link", text)
		}
		p.send(event.Event{Kind: event.SessionStart, StartMode: event.Resume}, "", true)
		p.replace("work.txt", "keep", "adjacent edit")
		if string(read(t, filepath.Join(p.service.Workspace.Root, "work.txt"))) != "user version\nadjacent edit\n" {
			t.Fatal("summary continuation lost the user version")
		}
		p.complete()
	})
	t.Run("WF-17", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-17")
		task := p.start(ledger.StartTaskInput{Title: "workspace paths"})
		original := p.service.Workspace
		moved := filepath.Join(filepath.Dir(original.Root), "已移动 space")
		if err := os.Rename(original.Root, moved); err != nil {
			t.Fatal(err)
		}
		discover := func(path string) gitstate.Workspace {
			w, err := gitstate.Discover(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.service.Store.RegisterGitWorkspace(t.Context(), w); err != nil {
				t.Fatal(err)
			}
			return w
		}
		parent := discover(moved)
		if parent.ID != original.ID || parent.ProjectID != original.ProjectID {
			t.Fatal("directory move changed identity")
		}
		p.service.Workspace = parent
		p.resume()
		alias := filepath.Join(filepath.Dir(moved), "工作区 alias")
		if err := os.Symlink(moved, alias); err != nil {
			t.Skipf("目标环境未能创建符号链接，完整路径场景未验证: %v", err)
		}
		p.service.Workspace = discover(alias)
		if p.service.Workspace.ID != original.ID {
			t.Fatal("symlink selected a different workspace")
		}
		p.resume()
		path := "work.txt"
		if platform.DirectoryCaseRule(moved) == platform.CaseInsensitive {
			path = "WORK.TXT"
		} else {
			write(t, moved, "WORK.TXT", "distinct uppercase file\n")
		}
		p.replace(path, "base", "parent")
		childPath := filepath.Join(filepath.Dir(moved), "子 worktree")
		git(t, moved, "worktree", "add", "--quiet", "-b", "child", childPath)
		child := discover(childPath)
		if child.ID == parent.ID || child.ProjectID != parent.ProjectID {
			t.Fatal("worktree identity merged with parent")
		}
		p.service.Workspace = child
		p.resume()
		p.message("只修改子工作区")
		p.start(ledger.StartTaskInput{Title: "child workspace"})
		p.replace("work.txt", "base", "child")
		movedChild := childPath + " moved"
		git(t, moved, "worktree", "move", childPath, movedChild)
		p.service.Workspace = discover(movedChild)
		if p.service.Workspace.ID != child.ID {
			t.Fatal("moved worktree changed identity")
		}
		p.resume()
		p.service.Workspace = parent
		p.resume()
		view := getTask(t, p.service, p.actor, "")
		if view.Task == nil || view.Task.ID != task.ID || view.Task.OriginTree != task.OriginTree || string(read(t, filepath.Join(moved, "work.txt"))) != "parent\nkeep\n" || string(read(t, filepath.Join(movedChild, "work.txt"))) != "child\nkeep\n" {
			t.Fatal("workspace navigation mixed tasks or contents")
		}
		p.complete()
	})
	t.Run("WF-18", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-18")
		task := p.start(ledger.StartTaskInput{Title: "fault and retry"})
		p.actor.SessionID = "retry-session"
		e := event.Event{Kind: event.SessionStart, StartMode: event.Resume, Agent: p.actor.Agent, SessionID: p.actor.SessionID, RoundID: p.actor.RoundID, WorkingDir: p.service.Workspace.Root}
		fault := func(kind string, ctx context.Context, output func(event.Decision) error, matches func(error) bool) {
			before, err := p.service.Store.SummaryState(t.Context(), p.service.Workspace.ID, p.actor)
			if err != nil {
				t.Fatal(err)
			}
			attempted := false
			err = p.service.HandleOutput(ctx, e, nil, func(d event.Decision) error {
				attempted = true
				return output(d)
			})
			if err == nil || !matches(err) {
				t.Fatal("expected isolated failure was not observed", kind, err)
			}
			after, stateErr := p.service.Store.SummaryState(t.Context(), p.service.Workspace.ID, p.actor)
			if stateErr != nil || after.InputVersion != before.InputVersion {
				t.Fatal("failed output acknowledged a summary", after, stateErr)
			}
			encoded, _ := json.Marshal(map[string]any{"id": p.measure.ID, "kind": kind, "error": err.Error(), "output_attempted": attempted, "delivered": false, "summary_version_unchanged": true})
			t.Logf("TASKBELAY_WORKFLOW_FAULT %s", encoded)
		}
		closed := errors.New("workflow output closed")
		fault("output_closed", t.Context(), func(d event.Decision) error {
			if d.AdditionalContext == "" {
				t.Fatal("failed output did not attempt the pending summary")
			}
			return closed
		}, func(err error) bool { return errors.Is(err, closed) })
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Millisecond))
		defer cancel()
		fault("deadline", ctx, func(event.Decision) error { t.Fatal("expired hook wrote output"); return nil }, func(err error) bool { return errors.Is(err, context.DeadlineExceeded) })
		db, err := sql.Open("sqlite", filepath.Join(p.service.Store.DataDir(), "taskbelay.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		connection, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if _, err := connection.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
			t.Fatal(err)
		}
		defer connection.ExecContext(context.Background(), "ROLLBACK")
		busy, end := context.WithTimeout(t.Context(), 50*time.Millisecond)
		fault("database_busy", busy, func(event.Decision) error { t.Fatal("busy hook wrote output"); return nil }, func(err error) bool {
			return errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "locked")
		})
		end()
		if _, err := connection.ExecContext(t.Context(), "ROLLBACK"); err != nil {
			t.Fatal(err)
		}
		if d := p.send(e, "", false); d.AdditionalContext == "" {
			t.Fatal("successful retry did not deliver the lost summary")
		}
		p.send(e, "", true)
		p.message("继续当前任务")
		request := edit("replayed-edit", "work.txt", "base", "current")
		p.send(request, "", false)
		pre, err := p.service.Store.ObservationState(t.Context(), p.service.Workspace.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		p.send(request, "", true)
		duplicate, err := p.service.Store.ObservationState(t.Context(), p.service.Workspace.ID, nil)
		if err != nil || duplicate.LatestID != pre.LatestID {
			t.Fatal("repeated pre-event appended evidence", err)
		}
		write(t, p.service.Workspace.Root, "work.txt", "current\nuser version\n")
		request.Kind, request.Result = event.ToolEnd, &event.ToolResult{State: event.Completed}
		p.send(request, "", false)
		before, err := p.service.Store.TaskView(t.Context(), p.service.Workspace.ID, task.ID, p.actor)
		if err != nil {
			t.Fatal(err)
		}
		p.send(request, "", true)
		after, err := p.service.Store.TaskView(t.Context(), p.service.Workspace.ID, task.ID, p.actor)
		if err != nil {
			t.Fatal(err)
		}
		if before.Task.RecordRevision != after.Task.RecordRevision || before.Task.LastObservationID != after.Task.LastObservationID || len(after.WorkspaceUpdates) == 0 || string(read(t, filepath.Join(p.service.Workspace.Root, "work.txt"))) != "current\nuser version\n" {
			t.Fatalf("retry evidence: record %d -> %d; observation %d -> %d; updates %d; content %q", before.Task.RecordRevision, after.Task.RecordRevision, before.Task.LastObservationID, after.Task.LastObservationID, len(after.WorkspaceUpdates), read(t, filepath.Join(p.service.Workspace.Root, "work.txt")))
		}
		p.complete()
	})
}
