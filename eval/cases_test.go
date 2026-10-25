package eval_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type caseInput struct {
	Request      string                    `json:"request"`
	Workflow     string                    `json:"workflow"`
	Editor       string                    `json:"editor"`
	Attempts     int                       `json:"attempts"`
	Enforcement  string                    `json:"enforcement"`
	Depth        string                    `json:"depth"`
	Requirements []ledger.RequirementInput `json:"requirements"`
	Plans        []casePlan                `json:"plans"`
	Notes        []ledger.NoteInput        `json:"notes"`
	Commands     []caseCommand             `json:"commands"`
	DuringCheck  map[string]string         `json:"during_check"`
	Retain       []caseRetention           `json:"retain"`
}

type caseRetention struct {
	Rule    string `json:"rule"`
	At      string `json:"at"`
	Message string `json:"message"`
}

type casePlan struct {
	Plan    ledger.PlanInput `json:"plan"`
	Confirm string           `json:"confirm"`
}

type caseExpectation struct {
	Status         ledger.CheckStatus `json:"status"`
	Issues         []caseIssue        `json:"issues"`
	Absent         []caseIssue        `json:"absent"`
	Missing        []string           `json:"missing"`
	BlockedEdits   int                `json:"blocked_edits"`
	DeliveryBlocks []string           `json:"delivery_blocks"`
	Preserved      map[string]string  `json:"preserved"`
	Commands       *caseCommandResult `json:"commands"`
	Dynamic        *caseDynamicResult `json:"dynamic"`
}

type caseIssue struct {
	Rule            string   `json:"rule"`
	At              string   `json:"at"`
	Status          string   `json:"status"`
	Level           string   `json:"level"`
	FactContains    []string `json:"fact_contains"`
	Handled         *bool    `json:"handled"`
	BlockOnDelivery *bool    `json:"block_on_delivery"`
}

func loadCaseJSON(t *testing.T, path string, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(read(t, path)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("%s: trailing content: %v", path, err)
	}
}

func caseFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("case fixture must be a regular file: %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(read(t, path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCasePacks(t *testing.T) {
	var cases []string
	err := filepath.WalkDir("cases", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "ledger.yaml" {
			cases = append(cases, filepath.Dir(path))
		}
		return nil
	})
	if err != nil || len(cases) == 0 {
		t.Fatalf("case inventory: %v (%d cases)", err, len(cases))
	}
	for _, dir := range cases {
		t.Run(strings.TrimPrefix(filepath.ToSlash(dir), "cases/"), func(t *testing.T) {
			runCasePack(t, dir)
		})
	}
}

func runCasePack(t *testing.T, dir string) {
	t.Helper()
	var input caseInput
	var expected caseExpectation
	loadCaseJSON(t, filepath.Join(dir, "ledger.yaml"), &input)
	loadCaseJSON(t, filepath.Join(dir, "expected.yaml"), &expected)
	if input.Enforcement == "" {
		input.Enforcement = "block"
	}
	if len(read(t, filepath.Join(dir, "README.md"))) == 0 || input.Request == "" || expected.Status == "" {
		t.Fatal("case needs rationale, request and completion expectation")
	}
	base := caseFiles(t, filepath.Join(dir, "base"))
	service := hook.Tasks{Observer: observer(t)}
	stage := t.TempDir()
	for path, content := range base {
		write(t, service.Workspace.Root, path, content)
		write(t, stage, path, content)
	}
	patch, err := filepath.Abs(filepath.Join(dir, "change.patch"))
	if err != nil {
		t.Fatal(err)
	}
	git(t, stage, "apply", "--check", patch)
	git(t, stage, "apply", patch)
	current := caseFiles(t, stage)
	message := userMessage(t, service, taskActor("message"), "request", input.Request)
	task := startTask(t, service, taskActor("start"), ledger.StartTaskInput{Title: filepath.Base(dir), Workflow: input.Workflow})
	if input.Enforcement != "" {
		if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "enforcement", input.Enforcement); err != nil {
			t.Fatal(err)
		}
	}
	for i, step := range input.Plans {
		view := getTask(t, service, taskActor("plan-view"), task.ID)
		step.Plan.BaseRevision = view.Plans.Revision
		result, err := service.SavePlan(t.Context(), taskActor(fmt.Sprintf("plan-%d", i)), step.Plan)
		if err != nil {
			t.Fatal(err)
		}
		if step.Confirm != "" {
			reply := userMessage(t, service, taskActor("confirm-message"), fmt.Sprintf("confirm-%d", i), step.Confirm)
			if _, err := service.ConfirmPlan(t.Context(), taskActor(fmt.Sprintf("confirm-%d", i)), ledger.PlanConfirmInput{Version: result.Plan.Version, SourceID: reply.ID, Quote: reply.Text}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, note := range input.Notes {
		note.SourceID = message.ID
		if note.Target != nil && note.Target.ID == "$task" {
			note.Target.ID = task.ID
		}
		addNote(t, service, taskActor(fmt.Sprintf("note-%d", i)), note)
	}
	var paths []string
	for path := range base {
		paths = append(paths, path)
	}
	for path := range current {
		if _, exists := base[path]; !exists {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	blocked := 0
	for _, path := range paths {
		old, had := base[path]
		next, has := current[path]
		if had == has && old == next {
			continue
		}
		if input.Editor != "workspace" {
			e := edit("case-"+path, path, old, next)
			e.RoundID, e.Edits[0].WholeFile = "round1", true
			if !had {
				e.Edits[0].Before = nil
			}
			if !has {
				e.Edits[0].After = nil
				e.Edits[0].Delete = true
			}
			allowed := false
			for attempt := 0; attempt < max(1, input.Attempts); attempt++ {
				e.ToolCallID = fmt.Sprintf("case-%s-%d", path, attempt)
				decision, err := service.BeforeEdit(t.Context(), e, task.ID, input.Enforcement)
				if err != nil {
					t.Fatal(err)
				}
				if decision.DenyTool != "" {
					blocked++
					continue
				}
				allowed = true
				break
			}
			if !allowed {
				continue
			}
			applyCaseFile(t, service.Workspace.Root, path, next, has)
			e.Kind, e.Result = event.ToolEnd, &event.ToolResult{State: event.Completed}
			if _, err := service.AfterEdit(t.Context(), e, task.ID); err != nil {
				t.Fatal(err)
			}
		} else {
			applyCaseFile(t, service.Workspace.Root, path, next, has)
		}
	}
	if blocked != expected.BlockedEdits {
		t.Errorf("blocked edits = %d, want %d", blocked, expected.BlockedEdits)
	}
	for i, requirement := range input.Requirements {
		if requirement.Quote != "" {
			requirement.SourceID = message.ID
		}
		if requirement.ImplementedBy != nil {
			for j := range *requirement.ImplementedBy {
				location := &(*requirement.ImplementedBy)[j]
				if location.Citation != nil && location.Citation.SourceID == "$message" {
					location.Citation.SourceID = message.ID
				}
			}
		}
		saveRequirement(t, service, taskActor(fmt.Sprintf("requirement-%d", i)), requirement)
	}
	if len(input.Commands) > 0 {
		runCaseCommands(t, service, task.ID, input.Commands, expected.Commands)
	}
	depth := input.Depth
	if depth == "" {
		depth = "static"
	}
	record := runCaseCheck(t, service, input, depth)
	if len(input.Retain) > 0 {
		var retained []ledger.Issue
		for i, decision := range input.Retain {
			matches := slices.DeleteFunc(slices.Clone(record.Issues), func(issue ledger.Issue) bool {
				return issue.Rule != decision.Rule || issue.At != decision.At
			})
			if len(matches) != 1 || decision.Message == "" {
				t.Fatal("retention needs a precise observed issue and user message", decision, matches)
			}
			issue := matches[0]
			retained = append(retained, issue)
			reply := userMessage(t, service, taskActor(fmt.Sprintf("retain-message-%d", i)), fmt.Sprintf("retain-%d", i), decision.Message)
			addNote(t, service, taskActor(fmt.Sprintf("retain-note-%d", i)), ledger.NoteInput{
				Kind: "decision", Text: decision.Message, SourceID: reply.ID, Quote: reply.Text,
				Target: &ledger.DecisionTarget{Kind: "issue", ID: issue.ID, At: issue.At, Effect: "retain", Scope: "task"},
			})
		}
		record, err = service.Check(t.Context(), taskActor("retained-check"), check.RunInput{Depth: depth})
		if err != nil {
			t.Fatal(err)
		}
		for _, before := range retained {
			if !slices.ContainsFunc(record.Issues, func(issue ledger.Issue) bool {
				return issue.ID == before.ID && issue.Identity == before.Identity && issue.Status == "retained" && len(issue.HandledBy) > 0
			}) {
				t.Error("retention lost issue identity or handling evidence", before)
			}
		}
	}
	compareCaseCheck(t, record, expected)
	compareCaseDynamic(t, record, expected.Dynamic)
	for path, content := range expected.Preserved {
		if got := string(read(t, filepath.Join(service.Workspace.Root, path))); got != content {
			t.Errorf("%s lost expected user behavior: %q", path, got)
		}
	}
	if len(expected.DeliveryBlocks) > 0 {
		if _, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{}); err != nil {
			t.Fatal(err)
		}
		first := stopTask(t, service)
		for _, rule := range expected.DeliveryBlocks {
			if !strings.Contains(first.BlockStop, rule) {
				t.Errorf("missing first block for %s: %+v", rule, first)
			}
		}
		if next := stopTask(t, service); next.BlockStop != "" {
			t.Errorf("same feedback blocked twice: %+v", next)
		}
		stored, err := service.Store.LatestCheck(t.Context(), task.ID, depth, "")
		if err != nil || stored == nil || stored.ID != record.ID {
			t.Fatalf("feedback did not reuse original observation: %+v %v", stored, err)
		}
		for _, issue := range record.Issues {
			if !slices.ContainsFunc(stored.Issues, func(saved ledger.Issue) bool { return saved.ID == issue.ID && saved.Identity == issue.Identity }) {
				t.Errorf("feedback changed issue identity: %+v", issue)
			}
		}
	}
}

func applyCaseFile(t *testing.T, root, path, content string, exists bool) {
	t.Helper()
	if exists {
		write(t, root, path, content)
	} else if err := os.Remove(filepath.Join(root, path)); err != nil {
		t.Fatal(err)
	}
}

func compareCaseCheck(t *testing.T, record ledger.CheckRecord, expected caseExpectation) {
	t.Helper()
	if record.Status != expected.Status {
		t.Errorf("completion = %s, want %s; missing=%+v", record.Status, expected.Status, record.Missing)
	}
	for _, want := range expected.Issues {
		matches := slices.DeleteFunc(slices.Clone(record.Issues), func(issue ledger.Issue) bool {
			return issue.Rule != want.Rule || (want.At != "" && issue.At != want.At)
		})
		if len(matches) != 1 {
			t.Errorf("want one %s at %s; got %+v", want.Rule, want.At, matches)
			continue
		}
		got := matches[0]
		if got.ID == "" || got.Identity == "" || got.FactKey == "" || len(got.Facts) == 0 {
			t.Errorf("issue lacks identity or facts: %+v", got)
		}
		if (want.Status != "" && got.Status != want.Status) || (want.Level != "" && got.Level != want.Level) {
			t.Errorf("issue conclusion: %+v; want %+v", got, want)
		}
		if want.Handled != nil && (len(got.HandledBy) > 0) != *want.Handled {
			t.Errorf("issue handling: %+v", got)
		}
		if want.BlockOnDelivery != nil && got.BlockOnDelivery != *want.BlockOnDelivery {
			t.Errorf("block prerequisites: %+v", got)
		}
		for _, fact := range want.FactContains {
			if !strings.Contains(strings.Join(got.Facts, "\n"), fact) {
				t.Errorf("fact %q not found: %+v", fact, got)
			}
		}
	}
	for _, absent := range expected.Absent {
		for _, got := range record.Issues {
			if got.Rule == absent.Rule && (absent.At == "" || got.At == absent.At) {
				t.Errorf("unexpected issue: %+v", got)
			}
		}
	}
	for _, operation := range expected.Missing {
		if !slices.ContainsFunc(record.Missing, func(gap ledger.CheckGap) bool { return gap.Operation == operation && gap.Reason != "" }) {
			t.Errorf("missing incomplete operation %s: %+v", operation, record.Missing)
		}
	}
}
