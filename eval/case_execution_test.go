package eval_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func runCaseCheck(t *testing.T, service hook.Tasks, input caseInput, depth string) ledger.CheckRecord {
	t.Helper()
	if len(input.DuringCheck) == 0 {
		record, err := service.Check(t.Context(), taskActor("check"), check.RunInput{Depth: depth})
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	if runtime.GOOS == "windows" {
		t.Skip("OT-14 prepare gate uses POSIX; native Windows concurrent-edit validation remains pending in N20")
	}
	marker := filepath.Join(t.TempDir(), "prepared")
	quote := func(text string) string { return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'" }
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "test.go.prepare", "printf ready > "+quote(marker)+"; sleep 0.3"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type result struct {
		record ledger.CheckRecord
		err    error
	}
	done := make(chan result, 1)
	go func() {
		record, err := service.Check(ctx, taskActor("check"), check.RunInput{Depth: depth})
		done <- result{record, err}
	}()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case result := <-done:
			t.Fatalf("dynamic check ended before edit gate: %+v", result)
		case <-deadline.C:
			cancel()
			<-done
			t.Fatal("dynamic check did not reach edit gate")
		case <-ticker.C:
		}
	}
	for path, content := range input.DuringCheck {
		write(t, service.Workspace.Root, path, content)
	}
	completed := <-done
	if completed.err != nil {
		t.Fatal(completed.err)
	}
	return completed.record
}

type caseCommand struct {
	Text         string `json:"text"`
	Shell        string `json:"shell"`
	ExitCode     *int   `json:"exit_code"`
	Environment  string `json:"environment"`
	Unknown      bool   `json:"unknown"`
	UnknownScope bool   `json:"unknown_scope"`
	Rerun        bool   `json:"rerun"`
}

type caseCommandResult struct {
	States   []string `json:"states"`
	Denied   int      `json:"denied"`
	Executed int      `json:"executed"`
	Hints    int      `json:"scope_hints"`
}

type caseScope struct{ step caseCommand }

func (scope caseScope) VerificationScope(_ context.Context, _ gitstate.Workspace, _ shell.Script, tree string) (testrun.VerificationScope, error) {
	if scope.step.UnknownScope {
		return testrun.VerificationScope{}, nil
	}
	return testrun.VerificationScope{InputID: tree, ToolID: "case-tool-v1", EnvironmentID: "case-env-" + scope.step.Environment, Complete: true, EnvironmentKnown: !scope.step.Unknown, Suggestion: "资料包已核对的包依赖范围"}, nil
}

func runCaseCommands(t *testing.T, service hook.Tasks, task string, commands []caseCommand, expected *caseCommandResult) {
	t.Helper()
	if expected == nil {
		t.Fatal("commands require execution expectations")
	}
	hints := 0
	for i, step := range commands {
		service.ScopeAnalyzer = caseScope{step}
		if step.Rerun {
			records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task)
			if err != nil || len(records) == 0 {
				t.Fatal("rerun needs previous verification", err)
			}
			message := userMessage(t, service, taskActor("rerun-message"), fmt.Sprintf("rerun-%d", i), "请重新运行这次验证")
			addNote(t, service, taskActor(fmt.Sprintf("rerun-note-%d", i)), ledger.NoteInput{Kind: "decision", Text: message.Text, SourceID: message.ID, Quote: message.Text, Target: &ledger.DecisionTarget{Kind: "verification", ID: records[len(records)-1].ID, Effect: "rerun_once", Scope: "once"}})
		}
		e := commandEvent(service, fmt.Sprintf("command-%d", i), step.Text)
		if step.Shell == "powershell" {
			e.Command.Shell = event.PowerShell
		} else if step.Shell != "" && step.Shell != "posix" {
			t.Fatalf("unsupported fixture shell %q", step.Shell)
		}
		decision := commandHook(t, service, e)
		if strings.Contains(decision.AdditionalContext, "V2") {
			hints++
		}
		if repeated := commandHook(t, service, e); repeated.DenyTool != decision.DenyTool {
			t.Fatal("pre-event replay changed decision")
		}
		// 这里的结果来自资料包的构造事件；原生工具执行另由动态案例验证。
		completeCommand(t, service, e, event.Completed, step.ExitCode)
		completeCommand(t, service, e, event.Completed, step.ExitCode)
	}
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task)
	if err != nil {
		t.Fatal(err)
	}
	var got caseCommandResult
	got.Hints = hints
	identities := map[string]string{}
	for _, record := range records {
		got.States = append(got.States, record.State)
		if record.Denied {
			got.Denied++
		}
		if record.Executed {
			got.Executed++
		}
		if record.Denied && record.Executed {
			t.Fatal("denied command counted as execution")
		}
		if record.Issue != nil {
			if previous := identities[record.Issue.Identity]; previous != "" && previous != record.Issue.ID {
				t.Fatal("same issue got new identity")
			}
			identities[record.Issue.Identity] = record.Issue.ID
		}
	}
	if !slices.Equal(got.States, expected.States) || got.Denied != expected.Denied || got.Executed != expected.Executed || got.Hints != expected.Hints {
		t.Errorf("command results = %+v, want %+v; records=%+v", got, expected, records)
	}
}

type caseDynamicResult struct {
	Complete          bool     `json:"complete"`
	Observations      int      `json:"observations"`
	Keep              []string `json:"keep"`
	Candidates        []string `json:"candidates"`
	NotCandidates     []string `json:"not_candidates"`
	MinimumCandidates int      `json:"minimum_candidates"`
	NoCandidates      bool     `json:"no_candidates"`
	MinimumMutations  int      `json:"minimum_mutations"`
	BaselineFailure   string   `json:"baseline_failure"`
	NoMutations       bool     `json:"no_mutations"`
}

func compareCaseDynamic(t *testing.T, record ledger.CheckRecord, expected *caseDynamicResult) {
	t.Helper()
	if expected == nil {
		return
	}
	report := record.Report.Dynamic
	if report == nil {
		t.Fatal("dynamic evidence missing")
	}
	if report.Complete != expected.Complete || len(report.Observations) != expected.Observations {
		t.Errorf("dynamic completion=%v observations=%d; want %+v; missing=%v", report.Complete, len(report.Observations), expected, report.Missing)
	}
	if expected.BaselineFailure != "" && !slices.ContainsFunc(report.Observations, func(observation testrun.Observation) bool {
		return string(observation.Baseline.Failure) == expected.BaselineFailure
	}) {
		t.Error("expected baseline failure not observed", expected.BaselineFailure, report.Observations)
	}
	var keep, candidates []string
	mutations := 0
	for _, group := range report.Groups {
		keep = append(keep, group.Keep...)
		candidates = append(candidates, group.Candidates...)
		mutations += len(group.Mutations)
		if !group.Complete && len(group.Candidates) > 0 {
			t.Error("incomplete evidence suggested deletion", group)
		}
	}
	for _, want := range expected.Keep {
		if !slices.Contains(keep, want) {
			t.Errorf("required case %s not retained: %+v", want, report.Groups)
		}
	}
	for _, want := range expected.Candidates {
		if !slices.Contains(candidates, want) {
			t.Errorf("candidate %s missing: %+v", want, report.Groups)
		}
	}
	for _, want := range expected.NotCandidates {
		if slices.Contains(candidates, want) {
			t.Errorf("required case %s suggested for deletion", want)
		}
	}
	if (expected.NoCandidates && len(candidates) > 0) || len(candidates) < expected.MinimumCandidates || mutations < expected.MinimumMutations || (expected.NoMutations && mutations != 0) {
		t.Errorf("dynamic evidence does not match expected: %+v; groups=%+v", expected, report.Groups)
	}
}
