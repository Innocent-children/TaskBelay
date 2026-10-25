package eval_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestCodeAnalysisUsesOnlyFixedTreeAndPreservesIndex(t *testing.T) {
	o := observer(t)
	write(t, o.Workspace.Root, "source.go", "package p\nfunc original() int { return 1 }\n")
	write(t, o.Workspace.Root, "package.json", `{"dependencies":{"known":"1"}}`)
	write(t, o.Workspace.Root, "docs/readme.md", "current requirements")
	git(t, o.Workspace.Root, "add", "source.go")
	index := git(t, o.Workspace.Root, "ls-files", "--stage")
	fixed, err := o.Workspace.Capture(t.Context())
	if err != nil || !fixed.Complete() {
		t.Fatal(fixed, err)
	}
	write(t, o.Workspace.Root, "source.go", "package p\nfunc userUpdated() int { return 2 }\n")
	write(t, o.Workspace.Root, "package.json", `{"dependencies":{"different":"2"}}`)
	result, err := check.AnalyzeTree(t.Context(), o.Workspace, fixed.Tree)
	if err != nil || len(result.Missing) != 0 {
		t.Fatal(result.Missing, err)
	}
	declarations := result.Declarations()
	if _, ok := declarations["source.go:original"]; !ok {
		t.Fatal(declarations)
	}
	if _, ok := declarations["source.go:userUpdated"]; ok || result.Files["package.json"].Dependencies[0].Name != "known" {
		t.Fatal("analysis read live workspace", result)
	}
	if result.Files["docs/readme.md"] == nil || len(result.Files["docs/readme.md"].Declarations) != 0 {
		t.Fatal("file-level document lost")
	}
	if current := git(t, o.Workspace.Root, "ls-files", "--stage"); current != index {
		t.Fatal("real index changed")
	}
	if got := string(read(t, filepath.Join(o.Workspace.Root, "source.go"))); got != "package p\nfunc userUpdated() int { return 2 }\n" {
		t.Fatal("user contents changed", got)
	}
}

func locationAnalysis(t *testing.T, s hook.Tasks) (ledger.TaskView, ledger.Observation, codescan.Snapshot, codescan.Snapshot) {
	t.Helper()
	view := getTask(t, s, taskActor("read"), "")
	state, err := s.Store.ObservationState(t.Context(), s.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := s.Store.Observation(t.Context(), state.LatestID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := check.AnalyzeTree(t.Context(), s.Workspace, view.Task.OriginTree)
	if err != nil {
		t.Fatal(err)
	}
	after, err := check.AnalyzeTree(t.Context(), s.Workspace, observation.Tree)
	if err != nil {
		t.Fatal(err)
	}
	return view, observation, before, after
}

func TestCodeAnalysisPublishesVerifiedRenameAndPreservesRequirementHistory(t *testing.T) {
	for _, verified := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified-edit", false: "ambiguous-workspace-update"}[verified], func(t *testing.T) {
			s := hook.Tasks{Observer: observer(t)}
			actor := taskActor("start")
			original := "package p\nfunc old()int{return 1}\nfunc keep()int{return 1}\n"
			updated := "package p\nfunc renamed()int{return 1}\nfunc keep()int{return 1}\n"
			write(t, s.Workspace.Root, "a.go", original)
			message := userMessage(t, s, actor, "request", "给旧实现改名，保留需求")
			task := startTask(t, s, actor, ledger.StartTaskInput{Title: "改名"})
			locations := []ledger.Location{{At: "a.go:old", Purpose: "原需求实现", Change: "rename"}}
			requirement := saveRequirement(t, s, taskActor("save"), ledger.RequirementInput{Content: "给旧实现改名", Acceptance: "原需求继续关联", SourceID: message.ID, Quote: "给旧实现改名", ImplementedBy: &locations})
			request := edit("rename", "a.go", "func old()int{return 1}", "func renamed()int{return 1}")
			if verified {
				if _, err := s.BeforeEdit(t.Context(), request, task.ID, "block"); err != nil {
					t.Fatal(err)
				}
			}
			write(t, s.Workspace.Root, "a.go", updated)
			if verified {
				if _, err := s.AfterEdit(t.Context(), request, task.ID); err != nil {
					t.Fatal(err)
				}
			}
			view, observation, before, after := locationAnalysis(t, s)
			resolved, _, err := check.ResolveLocations(t.Context(), s.Store, s.Workspace, view, observation, before, after)
			if err != nil {
				t.Fatal(err)
			}
			location := resolved.Requirements[0].ImplementedBy[0]
			if verified {
				if location.At != "a.go:renamed" || location.Pending != "" || location.AnalysisTree != after.Tree || location.BeforeAt != "a.go:old" || location.BeforeBlob != before.Files["a.go"].Blob || location.Blob != after.Files["a.go"].Blob {
					t.Fatal(location)
				}
				repeated := getTask(t, s, actor, task.ID)
				if repeated.Requirements[0].ImplementedBy[0].Pending != "" || repeated.Requirements[0].ImplementedBy[0].AnalysisTree != after.Tree {
					state, _ := s.Store.ObservationState(t.Context(), s.Workspace.ID, nil)
					latest, _ := s.Store.Observation(t.Context(), state.LatestID)
					t.Fatalf("locations=%+v analyzed=%s observed=%s full=%v complete=%v", repeated.Requirements, after.Tree, latest.Tree, latest.Full, latest.Complete)
				}
				if repeated.Requirements[0].Revision != requirement.Revision || repeated.Task.RequirementRevision != view.Task.RequirementRevision || repeated.Task.AuthorizationRevision != view.Task.AuthorizationRevision || repeated.History[0].ImplementedBy[0].At != "a.go:old" {
					t.Fatal(repeated)
				}
				write(t, s.Workspace.Root, "a.go", updated+"// user update\n")
				changed := getTask(t, s, actor, task.ID)
				if changed.Requirements[0].ImplementedBy[0].AnalysisTree != "" || changed.Requirements[0].ImplementedBy[0].Pending == "" {
					t.Fatal("stale analysis reused", changed.Requirements)
				}
			} else if location.At != "a.go:old" || location.Pending == "" || location.AnalysisTree != "" {
				t.Fatal(location)
			}
		})
	}
}

func TestCodeAnalysisDeletedLocationAndConcurrentPublication(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	actor := taskActor("start")
	write(t, s.Workspace.Root, "a.go", "package p\nfunc old(){}\n")
	message := userMessage(t, s, actor, "request", "删除旧函数")
	startTask(t, s, actor, ledger.StartTaskInput{Title: "删除"})
	locations := []ledger.Location{{At: "a.go:old", Purpose: "删除旧函数", Change: "delete"}}
	requirement := saveRequirement(t, s, taskActor("save"), ledger.RequirementInput{Content: "删除旧函数", Acceptance: "不再保留实现", SourceID: message.ID, Quote: "删除旧函数", ImplementedBy: &locations})
	write(t, s.Workspace.Root, "a.go", "package p\n")
	view, observation, before, after := locationAnalysis(t, s)
	resolved, _, err := check.ResolveLocations(t.Context(), s.Store, s.Workspace, view, observation, before, after)
	if err != nil {
		t.Fatal(err)
	}
	location := resolved.Requirements[0].ImplementedBy[0]
	if location.Pending != "" || location.BeforeBlob != before.Files["a.go"].Blob || location.Blob != "" || location.AnalysisTree != after.Tree {
		t.Fatal(location)
	}
	input := ledger.RequirementInput{ID: requirement.ID, BaseRevision: &requirement.Revision, Content: "删除旧函数", Acceptance: "新的验收条件", SourceID: message.ID, Quote: "删除旧函数"}
	updated, err := s.Store.SaveRequirement(t.Context(), taskCall(t, s, taskActor("update"), "requirement_save", input), view.CurrentTaskID, observation.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = check.ResolveLocations(t.Context(), s.Store, s.Workspace, view, observation, before, after)
	var conflict *ledger.RequirementConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("wanted requirement conflict, got %v", err)
	}
	latest, err := s.Store.TaskView(t.Context(), s.Workspace.ID, "", actor)
	if err != nil || latest.Requirements[0].Revision != updated.Revision || latest.Requirements[0].Acceptance != "新的验收条件" {
		t.Fatal(latest, err)
	}
	if _, err := s.Observe(t.Context(), event.Event{Kind: event.SessionStart}, view.CurrentTaskID); err != nil {
		t.Fatal(err)
	}
	_, _, err = check.ResolveLocations(t.Context(), s.Store, s.Workspace, latest, observation, before, after)
	if !errors.Is(err, ledger.ErrObservationConflict) {
		t.Fatalf("wanted observation conflict, got %v", err)
	}
}

func TestCodeAnalysisReadsDeletedDeclarationFromIntermediateObservation(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	actor := taskActor("start")
	write(t, s.Workspace.Root, "a.go", "package p\n")
	message := userMessage(t, s, actor, "request", "清理临时函数")
	task := startTask(t, s, actor, ledger.StartTaskInput{Title: "清理"})
	write(t, s.Workspace.Root, "a.go", "package p\nfunc temporary(){}\n")
	intermediate, err := s.Observe(t.Context(), event.Event{Kind: event.SessionStart}, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	locations := []ledger.Location{{At: "a.go:temporary", Change: "delete", Purpose: "删除任务过程中临时引入的函数"}}
	saveRequirement(t, s, taskActor("save"), ledger.RequirementInput{Content: "清理临时函数", Acceptance: "临时实现不保留", SourceID: message.ID, Quote: "清理临时函数", ImplementedBy: &locations})
	write(t, s.Workspace.Root, "a.go", "package p\n")
	view, observation, before, after := locationAnalysis(t, s)
	resolved, association, err := check.ResolveLocations(t.Context(), s.Store, s.Workspace, view, observation, before, after)
	if err != nil {
		t.Fatal(err)
	}
	location := resolved.Requirements[0].ImplementedBy[0]
	if location.Pending != "" || location.BeforeBlob != intermediate.Files["a.go"].Blob || len(association.Pending) > 0 || len(association.Requirements["a.go:temporary"]) != 1 {
		t.Fatal(location, association)
	}
}

func TestCodeAnalysisDoesNotFollowSymbolicLink(t *testing.T) {
	o := observer(t)
	outside := filepath.Join(t.TempDir(), "private.go")
	if err := os.WriteFile(outside, []byte("package p\nfunc external() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(o.Workspace.Root, "linked.go")); err != nil {
		t.Skipf("environment does not allow symlink creation: %v", err)
	}
	fixed, err := o.Workspace.Capture(t.Context())
	if err != nil || !fixed.Complete() {
		t.Fatal(fixed, err)
	}
	result, err := check.AnalyzeTree(t.Context(), o.Workspace, fixed.Tree)
	if err != nil || len(result.Declarations()) != 0 || result.Files["linked.go"].Mode != "120000" {
		t.Fatal(result, err)
	}
}

func TestCodeAnalysisRetainsDeletedRequirementAndAmbiguousPosition(t *testing.T) {
	o := observer(t)
	write(t, o.Workspace.Root, "a.go", "package p\nfunc old() int { return 1 }\n")
	before, err := o.Workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	write(t, o.Workspace.Root, "a.go", "package p\nfunc first() int { return 1 }\nfunc second() int { return 1 }\n")
	after, err := o.Workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	base, err := check.AnalyzeTree(t.Context(), o.Workspace, before.Tree)
	if err != nil {
		t.Fatal(err)
	}
	current, err := check.AnalyzeTree(t.Context(), o.Workspace, after.Tree)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := codescan.Compare(t.Context(), base, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := codescan.Associate(t.Context(), base, current, changes, []codescan.Position{{Requirement: "R1", At: "a.go:old", Purpose: "原需求的实现"}})
	if err != nil || len(result.Positions) != 1 || result.Positions[0].At != "a.go:old" || len(result.Pending) != 1 || len(result.Requirements["a.go:old"]) != 1 || len(codescan.Migrations(changes)) != 0 {
		t.Fatal(result, err)
	}
}
