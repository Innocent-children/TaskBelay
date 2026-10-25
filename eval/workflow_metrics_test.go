package eval_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type workflowMeasurement struct {
	ID               string `json:"id"`
	Events           int    `json:"events"`
	ExtraApprovals   int    `json:"extra_approvals"`
	DuplicatePrompts int    `json:"duplicate_prompts"`
	InvalidBlocks    int    `json:"invalid_blocks"`
	Complete         bool   `json:"complete"`
}

type workflowProbe struct {
	t       *testing.T
	service hook.Tasks
	actor   ledger.Actor
	measure workflowMeasurement
	serial  int
}

func newWorkflowProbe(t *testing.T, id string) *workflowProbe {
	return newWorkflowProbeWithFiles(t, id, map[string]string{"work.txt": "base\nkeep\n"})
}

func newWorkflowProbeWithFiles(t *testing.T, id string, files map[string]string) *workflowProbe {
	p := &workflowProbe{t: t, service: hook.Tasks{Observer: observer(t)}, actor: taskActor(""), measure: workflowMeasurement{ID: id}}
	for path, content := range files {
		write(t, p.service.Workspace.Root, path, content)
	}
	git(t, p.service.Workspace.Root, "add", ".")
	git(t, p.service.Workspace.Root, "commit", "-qm", "base")
	t.Cleanup(func() {
		p.measure.Complete = p.measure.Complete && !t.Failed()
		encoded, _ := json.Marshal(p.measure)
		t.Logf("TASKBELAY_WORKFLOW_METRICS %s", encoded)
	})
	p.send(event.Event{Kind: event.SessionStart, StartMode: event.NewSession}, "", false)
	p.message("完成当前需求，保留用户修改")
	return p
}

func (p *workflowProbe) nextActor() ledger.Actor {
	p.serial++
	actor := p.actor
	actor.CallID = fmt.Sprintf("call-%d", p.serial)
	return actor
}

func (p *workflowProbe) message(text string) ledger.Message {
	p.serial++
	p.actor.RoundID = fmt.Sprintf("round-%d", p.serial)
	p.send(event.Event{Kind: event.UserPrompt, MessageID: p.actor.RoundID, Message: text}, "", false)
	view := getTask(p.t, p.service, p.actor, "")
	return view.Messages[len(view.Messages)-1]
}

func (p *workflowProbe) start(input ledger.StartTaskInput) ledger.TaskRecord {
	return startTask(p.t, p.service, p.nextActor(), input)
}

/**
 * 计数来自统一入口的实际决定。过期编辑退回属于必要核对；
 * 输入未变的接续必须无输出，不能因返回了相同文字就把它当作新信息。
 */
func (p *workflowProbe) send(e event.Event, allowedBlock string, quiet bool) event.Decision {
	p.t.Helper()
	e.Agent, e.SessionID, e.RoundID, e.WorkingDir = p.actor.Agent, p.actor.SessionID, p.actor.RoundID, p.service.Workspace.Root
	d, err := p.service.Handle(p.t.Context(), e, nil)
	blocked := d.DenyTool != "" || d.BlockStop != ""
	text := d.DenyTool + d.BlockStop + d.AdditionalContext + d.UserMessage
	scopeS1, scopeS2 := strings.Contains(text, "[S1"), strings.Contains(text, "[S2")
	permitted := d.BlockStop == "" && !scopeS2 && (allowedBlock == "stale" && strings.Contains(d.DenyTool, "过期") || allowedBlock == "S1" && strings.HasPrefix(d.DenyTool, "[S1]"))
	if id, ok := strings.CutPrefix(allowedBlock, "D1:"); ok && id != "" {
		permitted = d.DenyTool == "" && !scopeS1 && !scopeS2 && strings.HasPrefix(d.BlockStop, "[D1 ") && strings.Count(d.BlockStop, "编号=") == 1 && strings.Contains(d.BlockStop, "编号="+id+"\n")
	}
	invalid := blocked && !permitted
	approval := scopeS2 || scopeS1 && allowedBlock != "S1"
	duplicate := quiet && (d.AdditionalContext != "" || d.UserMessage != "" || blocked)
	p.measure.Events++
	if invalid {
		p.measure.InvalidBlocks++
	}
	if approval {
		p.measure.ExtraApprovals++
	}
	if duplicate {
		p.measure.DuplicatePrompts++
	}
	entry := map[string]any{"id": p.measure.ID, "event": p.measure.Events, "kind": e.Kind, "session": e.SessionID, "tool_call": e.ToolCallID, "decision": d, "allowed_block": allowedBlock, "expected_quiet": quiet, "invalid_block": invalid, "extra_approval": approval, "duplicate_prompt": duplicate}
	if err != nil {
		entry["error"] = err.Error()
	}
	encoded, _ := json.Marshal(entry)
	p.t.Logf("TASKBELAY_WORKFLOW_EVENT %s", encoded)
	if err != nil || invalid || approval || duplicate {
		p.t.Fatalf("workflow event: %+v, error: %v", entry, err)
	}
	return d
}

func TestWorkflowDecisions(t *testing.T) {
	t.Run("WF-07", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-07")
		message := p.message("实现输出，可以保留该参数")
		p.start(ledger.StartTaskInput{Title: "changing requirements"})
		r := saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "实现输出", Acceptance: "输出正确", SourceID: message.ID, Quote: "实现输出"})
		addNote(t, p.service, p.nextActor(), ledger.NoteInput{Kind: "decision", Text: "保留参数", SourceID: message.ID, Quote: "保留该参数", Target: &ledger.DecisionTarget{Kind: "requirement", ID: r.ID, At: "flag:--format", Effect: "retain", Scope: "task"}})
		p.resume()
		message = p.message("输出改成 JSON")
		r = saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{ID: r.ID, BaseRevision: &r.Revision, Content: message.Text, Acceptance: "输出正确的 JSON", SourceID: message.ID, Quote: message.Text})
		p.resume()
		message = p.message("取消输出需求，追加日志需求")
		saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{ID: r.ID, BaseRevision: &r.Revision, Content: r.Content, Acceptance: r.Acceptance, Status: "revoked", SourceID: message.ID, Quote: "取消输出需求"})
		saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "追加日志需求", Acceptance: "日志正确", SourceID: message.ID, Quote: "追加日志需求"})
		view := getTask(t, p.service, p.actor, "")
		if len(view.EffectiveDecisions) != 0 || len(view.Requirements) != 2 || view.Requirements[0].Status != "revoked" {
			t.Fatal("revoked requirement kept permission or lost the new requirement")
		}
		p.replace("work.txt", "base", "current requirement")
		p.complete()
	})
	t.Run("WF-08", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-08")
		for _, attached := range []bool{false, true} {
			request := "先保存方案"
			if attached {
				request = "方案按 A 做，直接改"
			}
			message := p.message(request)
			p.start(ledger.StartTaskInput{Title: "confirmation", Workflow: "semi"})
			plan := firstPlan()
			plan.Files, plan.Entries = []ledger.PlanFile{{Path: "work.txt", Change: "modify", Note: "当前文件"}}, nil
			saved, err := p.service.SavePlan(t.Context(), p.nextActor(), plan)
			if err != nil {
				t.Fatal(err)
			}
			if !attached {
				message = p.message("可以")
			}
			confirmed, err := p.service.ConfirmPlan(t.Context(), p.nextActor(), ledger.PlanConfirmInput{Version: saved.Plan.Version, SourceID: message.ID, Quote: message.Text})
			if err != nil || confirmed.View.Effective == nil {
				t.Fatal("confirmation failed", confirmed, err)
			}
			p.replace("work.txt", "keep", "approved")
			p.replace("work.txt", "approved", "keep")
			p.resume()
		}
		p.complete()
	})
	t.Run("WF-09", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-09")
		message := p.message("方案按 A 做，直接改")
		p.start(ledger.StartTaskInput{Title: "pending addition", Workflow: "semi"})
		plan := firstPlan()
		plan.Files, plan.Entries = []ledger.PlanFile{{Path: "work.txt", Change: "modify", Note: "当前范围"}}, nil
		saved, err := p.service.SavePlan(t.Context(), p.nextActor(), plan)
		if err != nil {
			t.Fatal(err)
		}
		confirmed, err := p.service.ConfirmPlan(t.Context(), p.nextActor(), ledger.PlanConfirmInput{Version: saved.Plan.Version, SourceID: message.ID, Quote: message.Text})
		if err != nil {
			t.Fatal(err)
		}
		plan.BaseRevision = confirmed.View.Revision
		plan.Files = []ledger.PlanFile{{Path: "extra.txt", Change: "add", Note: "新增范围待确认"}}
		if _, err := p.service.SavePlan(t.Context(), p.nextActor(), plan); err != nil {
			t.Fatal(err)
		}
		p.replace("work.txt", "base", "within original scope")
		e := edit("new-scope", "extra.txt", "", "extra")
		e.Edits[0].Before, e.Edits[0].WholeFile = nil, true
		if d := p.send(e, "S1", false); !strings.Contains(d.DenyTool, "[S1]") {
			t.Fatal("pending scope unexpectedly permitted", d)
		}
		p.replace("work.txt", "keep", "original scope continues")
		p.complete()
	})
	t.Run("WF-10", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-10")
		task := p.start(ledger.StartTaskInput{Title: "rerun"})
		p.service.ScopeAnalyzer = knownVerificationScope{}
		command := func(code *int) {
			e := commandEvent(p.service, p.nextActor().CallID, "go test ./...")
			p.send(e, "", false)
			e.Kind, e.Result = event.ToolEnd, &event.ToolResult{State: event.Completed, ExitCode: code}
			p.send(e, "", false)
		}
		failed, passed := 1, 0
		command(&failed)
		command(&passed)
		message := p.message("请重跑一次")
		records, err := p.service.Store.Verifications(t.Context(), p.service.Workspace.ID, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		addNote(t, p.service, p.nextActor(), ledger.NoteInput{Kind: "decision", Text: "明确重跑", SourceID: message.ID, Quote: message.Text, Target: &ledger.DecisionTarget{Kind: "verification", ID: records[len(records)-1].ID, Effect: "rerun_once", Scope: "once"}})
		command(nil)
		command(&passed)
		write(t, p.service.Workspace.Root, "work.txt", "new input\nkeep\n")
		command(&passed)
		p.service.ScopeAnalyzer = knownVerificationScope{environmentID: "fixture-environment-v2"}
		command(&passed)
		p.complete()
	})
	t.Run("WF-11", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX preparation fixture; Windows native process evidence is separate")
		}
		p := newWorkflowProbe(t, "WF-11")
		dir := p.service.Workspace.Root
		for file, source := range map[string]string{"go.mod": "module example.test/workflow\n\ngo 1.26\n", "value.go": "package sample\nfunc Allowed(n int) bool { return n < 5 }\n", "value_test.go": "package sample\nimport \"testing\"\nfunc TestAllowed(t *testing.T) { if !Allowed(1) { t.Fatal(\"expected true\") } }\n"} {
			write(t, dir, file, source)
		}
		message := p.message("完成范围判断")
		p.start(ledger.StartTaskInput{Title: "dynamic edit"})
		saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "完成范围判断", Acceptance: "保持用户范围", SourceID: message.ID, Quote: message.Text, ImplementedBy: &[]ledger.Location{{At: "value.go:Allowed", Purpose: "范围判断"}}})
		marker := filepath.Join(t.TempDir(), "prepared")
		prepare := "printf ready > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'; sleep 0.3"
		if err := p.service.Store.SetSetting(t.Context(), p.service.Workspace.ID, ledger.Task, "test.go.prepare", prepare); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			check ledger.CheckRecord
			err   error
		}
		done := make(chan outcome, 1)
		actor := p.nextActor()
		go func() {
			record, err := p.service.Check(t.Context(), actor, check.RunInput{Depth: "tests"})
			done <- outcome{record, err}
		}()
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			select {
			case result := <-done:
				t.Fatal("dynamic preparation did not start", result)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("dynamic preparation timed out")
			}
			time.Sleep(10 * time.Millisecond)
		}
		write(t, dir, "value.go", "package sample\nfunc Allowed(n int) bool { return n < 3 } // user\n")
		p.resume()
		result := <-done
		if result.err != nil || result.check.Status != ledger.CheckExpired || !strings.Contains(string(read(t, filepath.Join(dir, "value.go"))), "// user") {
			t.Fatal("stale dynamic result or lost user edit", result.check.Status, result.err)
		}
		p.complete()
	})
	t.Run("WF-12", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-12")
		task := p.start(ledger.StartTaskInput{Title: "finish request"})
		p.replace("work.txt", "base", "first delivery")
		if _, err := p.service.Finish(t.Context(), p.nextActor(), ledger.FinishInput{}); err != nil {
			t.Fatal(err)
		}
		p.send(event.Event{Kind: event.Stop}, "", false)
		view := getTask(t, p.service, p.actor, task.ID)
		if view.Task.State != ledger.TaskCompleted {
			t.Fatal("unchanged request did not finish")
		}
		p.message("继续修改同一个任务")
		p.start(ledger.StartTaskInput{TaskID: task.ID})
		p.replace("work.txt", "first delivery", "next delivery")
		if _, err := p.service.Finish(t.Context(), p.nextActor(), ledger.FinishInput{}); err != nil {
			t.Fatal(err)
		}
		write(t, p.service.Workspace.Root, "work.txt", "user version\nkeep\n")
		p.send(event.Event{Kind: event.Stop}, "", false)
		view = getTask(t, p.service, p.actor, task.ID)
		if view.Task.State != ledger.TaskActive {
			t.Fatal("changed input did not expire the request")
		}
		if _, err := p.service.Finish(t.Context(), p.nextActor(), ledger.FinishInput{}); err != nil {
			t.Fatal(err)
		}
		p.send(event.Event{Kind: event.Stop}, "", false)
		view = getTask(t, p.service, p.actor, task.ID)
		if view.Task.State != ledger.TaskCompleted || string(read(t, filepath.Join(p.service.Workspace.Root, "work.txt"))) != "user version\nkeep\n" {
			t.Fatal("renewed delivery lost task or user content")
		}
		p.complete()
	})
}

func (p *workflowProbe) resume() {
	p.send(event.Event{Kind: event.SessionStart, StartMode: event.Resume}, "", false)
	p.send(event.Event{Kind: event.SessionStart, StartMode: event.Resume}, "", true)
}

func (p *workflowProbe) replace(path, old, current string) {
	p.t.Helper()
	a := p.nextActor()
	e := edit(a.CallID, path, old, current)
	p.send(e, "", false)
	text := string(read(p.t, filepath.Join(p.service.Workspace.Root, path)))
	if strings.Count(text, old) != 1 {
		p.t.Fatal("probe edit must identify exactly one current fragment")
	}
	write(p.t, p.service.Workspace.Root, path, strings.Replace(text, old, current, 1))
	e.Kind, e.Result = event.ToolEnd, &event.ToolResult{State: event.Completed}
	p.send(e, "", false)
}

func (p *workflowProbe) complete() {
	p.send(event.Event{Kind: event.Stop}, "", false)
	p.resume()
	p.measure.Complete = true
}

func TestWorkflowInterruptions(t *testing.T) {
	t.Run("WF-01", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-01")
		task := p.start(ledger.StartTaskInput{Title: "three commits"})
		dir := p.service.Workspace.Root
		for i := 1; i <= 3; i++ {
			p.replace("work.txt", "keep", fmt.Sprintf("keep%d", i))
			git(t, dir, "add", "work.txt")
			write(t, dir, "unstaged.txt", fmt.Sprintf("untracked %d\n", i))
			index := filepath.Join(p.service.Workspace.GitDir, "index")
			before := read(t, index)
			p.resume()
			if !bytes.Equal(before, read(t, index)) {
				t.Fatal("hook changed the real staging area")
			}
			git(t, dir, "commit", "-qm", fmt.Sprintf("step %d", i))
			p.replace("work.txt", fmt.Sprintf("keep%d", i), "keep")
		}
		view := getTask(t, p.service, p.actor, "")
		if view.Task.OriginTree != task.OriginTree || len(view.WorkspaceUpdates) == 0 {
			t.Fatal("commit changed origin or lost unstaged work")
		}
		p.complete()
	})
	t.Run("WF-02", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-02")
		p.start(ledger.StartTaskInput{Title: "adjacent edit"})
		write(t, p.service.Workspace.Root, "work.txt", "user handling\nkeep\n")
		p.replace("work.txt", "keep", "adjacent change")
		if got := string(read(t, filepath.Join(p.service.Workspace.Root, "work.txt"))); got != "user handling\nadjacent change\n" {
			t.Fatal("adjacent edit lost external content", got)
		}
		p.complete()
	})
	t.Run("WF-03", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-03")
		p.start(ledger.StartTaskInput{Title: "intersecting edit"})
		write(t, p.service.Workspace.Root, "work.txt", "user handling\nkeep\n")
		d := p.send(edit("stale", "work.txt", "base", "stale replacement"), "stale", false)
		if d.DenyTool == "" || !strings.Contains(d.AdditionalContext, "user handling") {
			t.Fatal("stale request was not returned with current content", d)
		}
		p.replace("work.txt", "user handling", "user handling and current edit")
		p.complete()
	})
	t.Run("WF-04", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-04")
		message := p.message("保留文件的输出")
		p.start(ledger.StartTaskInput{Title: "move and delete"})
		req := saveRequirement(t, p.service, p.nextActor(), ledger.RequirementInput{Content: "保留文件的输出", Acceptance: "输出保留", SourceID: message.ID, Quote: message.Text, ImplementedBy: &[]ledger.Location{{At: "work.txt", Change: "modify", Purpose: "输出"}}})
		dir := p.service.Workspace.Root
		if err := os.Rename(filepath.Join(dir, "work.txt"), filepath.Join(dir, "moved.txt")); err != nil {
			t.Fatal(err)
		}
		p.resume()
		if err := os.Remove(filepath.Join(dir, "moved.txt")); err != nil {
			t.Fatal(err)
		}
		p.resume()
		view := getTask(t, p.service, p.actor, "")
		if len(view.Requirements) != 1 || view.Requirements[0].ID != req.ID {
			t.Fatal("file move or deletion removed its requirement")
		}
		p.complete()
	})
	t.Run("WF-05", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-05")
		a := p.start(ledger.StartTaskInput{Title: "A"})
		p.replace("work.txt", "base", "A")
		p.message("先完成 B，再继续 A")
		b := p.start(ledger.StartTaskInput{Title: "B"})
		p.replace("work.txt", "keep", "B completed")
		transition(t, p.service, p.nextActor(), b.ID, ledger.TaskPending)
		transition(t, p.service, p.nextActor(), b.ID, ledger.TaskCompleted)
		p.actor.Agent, p.actor.SessionID = "claude", "resumed-session"
		p.send(event.Event{Kind: event.SessionStart, StartMode: event.NewSession}, "", false)
		p.message("继续 A")
		resumed := p.start(ledger.StartTaskInput{TaskID: a.ID})
		if resumed.OriginTree != a.OriginTree || !strings.Contains(string(read(t, filepath.Join(p.service.Workspace.Root, "work.txt"))), "B completed") {
			t.Fatal("resume lost A origin or B content")
		}
		p.complete()
	})
	t.Run("WF-06", func(t *testing.T) {
		p := newWorkflowProbe(t, "WF-06")
		dir := p.service.Workspace.Root
		git(t, dir, "branch", "-M", "main")
		task := p.start(ledger.StartTaskInput{Title: "git operations"})
		p.replace("work.txt", "base", "changed")
		git(t, dir, "stash", "push", "-qm", "fixture")
		p.resume()
		git(t, dir, "stash", "pop", "--quiet")
		p.resume()
		git(t, dir, "add", "work.txt")
		git(t, dir, "commit", "-qm", "work")
		git(t, dir, "commit", "--amend", "--no-edit")
		p.resume()
		git(t, dir, "checkout", "-qb", "feature")
		p.resume()
		p.message("完成 feature 分支工作")
		feature := p.start(ledger.StartTaskInput{Title: "feature"})
		write(t, dir, "feature.txt", "feature\n")
		git(t, dir, "add", "feature.txt")
		git(t, dir, "commit", "-qm", "feature")
		git(t, dir, "checkout", "-q", "main")
		p.resume()
		view := getTask(t, p.service, p.actor, "")
		if view.Task == nil || view.Task.ID != task.ID || view.Task.OriginTree != task.OriginTree {
			t.Fatal("Git operations changed task boundary")
		}
		write(t, dir, "main.txt", "main update\n")
		git(t, dir, "add", "main.txt")
		git(t, dir, "commit", "-qm", "main update")
		p.resume()
		git(t, dir, "checkout", "-q", "feature")
		p.resume()
		git(t, dir, "rebase", "main")
		p.resume()
		upstream := t.TempDir()
		git(t, upstream, "clone", "-q", dir, ".")
		write(t, upstream, "pull.txt", "upstream\n")
		git(t, upstream, "add", "pull.txt")
		git(t, upstream, "commit", "-qm", "upstream")
		git(t, dir, "pull", "--ff-only", upstream, "feature")
		p.resume()
		view = getTask(t, p.service, p.actor, "")
		if view.Task == nil || view.Task.ID != feature.ID || view.Task.OriginTree != feature.OriginTree {
			t.Fatal("pull or rebase changed task boundary")
		}
		p.complete()
	})
}
