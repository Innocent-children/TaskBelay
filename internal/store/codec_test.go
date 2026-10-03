package store

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func TestStrictCodecAndRestart(t *testing.T) {
	for _, host := range []domain.Host{domain.HostCodex, domain.HostZCode} {
		t.Run(string(host), func(t *testing.T) { testStrictCodecAndRestart(t, host) })
	}
}

func testStrictCodecAndRestart(t *testing.T, host domain.Host) {
	t.Helper()
	task := multiRepositoryGraphTask(t)
	task.OriginHost = host
	task.Requirements = &domain.RequirementsBaseline{Revision: 1, Digest: task.Process.DefinitionDigest, Goal: "Graph storage", AcceptanceCriteria: []string{"Restart exactly"}, CreatedAt: task.CreatedAt}
	raw, err := encodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeTask(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(task, decoded) {
		t.Fatal("codec changed task")
	}
	overLimit := task
	overLimit.BaselineHistory = []domain.BaselineReference{{Kind: domain.BaselineRequirements, Revision: 1}}
	if _, err := encodeTask(overLimit); err == nil {
		t.Fatal("invalid baseline reference accepted")
	}
	unknown := append(bytes.TrimSuffix(raw, []byte("}")), []byte(`,"unknown":true}`)...)
	if _, err := decodeTask(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
	duplicate := append([]byte(`{"task_id":"task",`), raw[1:]...)
	if _, err := decodeTask(duplicate); err == nil {
		t.Fatal("duplicate field accepted")
	}
	path := dbPath(t)
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	mutation := testMutation(t, task)
	if err := store.CommitTask(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reopened, err := store.LoadTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mutation.Task, reopened) {
		t.Fatal("restart changed task/action")
	}
	claimed, err := store.LoadActiveTask(context.Background(), task.Repository.WorktreeInstanceDigest)
	if err != nil || claimed.TaskID != task.TaskID || claimed.OriginHost != host {
		t.Fatalf("restart lost the original Host claim: task=%+v err=%v", claimed, err)
	}
	beforeDigest, err := mutation.Task.EffectiveRepositoryBindingDigest()
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, err := reopened.EffectiveRepositoryBindingDigest()
	if err != nil || beforeDigest != afterDigest || len(reopened.AdditionalRepositories) != 1 {
		t.Fatalf("scope restart digest=%s want=%s additions=%d err=%v", afterDigest, beforeDigest, len(reopened.AdditionalRepositories), err)
	}
}

func TestStrictCodecRetainsVerificationBudgetAdjustment(t *testing.T) {
	task := fullGraphTask(t)
	previous := task.TaskPlan.VerificationPlan.InitialBudget
	current := previous
	current.MaxAutomaticCommands++
	task.VerificationBudgetAdjustments = []domain.VerificationBudgetAdjustment{{
		Revision: 1, TaskPlanRevision: task.TaskPlan.Revision, Basis: domain.VerificationAdjustmentGap,
		Reason:                      "A missing public-contract check was identified.",
		AdditionalChecks:            []domain.VerificationPlanCheck{{Name: "public-contract", Rationale: "The check covers the identified contract gap."}},
		AdditionalAutomaticCommands: 1, PreviousBudget: previous, CurrentBudget: current, CreatedAt: task.CreatedAt,
	}}
	raw, err := encodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeTask(raw)
	if err != nil || !reflect.DeepEqual(decoded.VerificationBudgetAdjustments, task.VerificationBudgetAdjustments) {
		t.Fatalf("adjustment round trip=%#v err=%v", decoded.VerificationBudgetAdjustments, err)
	}
}

func TestStrictCodecRetainsFileScopeDecisionsAndCurrentPaths(t *testing.T) {
	task := fullGraphTask(t)
	now := task.CreatedAt
	allowedActionID := domain.ID("implementation-action")
	task.Repository.TaskSurface = []domain.RepositoryChangedEntry{storeChangedEntry("internal/extra.go", task.Repository.ContentDigest)}
	task.CurrentChangedPaths = []string{"internal/extra.go"}
	acceptedStates, err := task.FileScopePathStates(task.CurrentChangedPaths)
	if err != nil {
		t.Fatal(err)
	}
	task.FileScopeRecords = []domain.FileScopeRecord{{
		RequestID: "scope-request", Paths: []string{"internal/extra.go"}, IntentDigest: task.Repository.BindingDigest,
		TaskPlanRevision: 1, SourceNode: domain.NodeImplement, SourceActionID: "source-action",
		Decision: domain.FileScopeAllowOnce, Reason: "Allow the exact implementation write.", Applicability: domain.FileScopeExactWrite,
		AllowedActionID: &allowedActionID, Consumed: true, AcceptedPathStates: acceptedStates, CreatedAt: now, DecidedAt: &now,
	}}
	raw, err := encodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeTask(raw)
	if err != nil || !reflect.DeepEqual(task.FileScopeRecords, decoded.FileScopeRecords) || !reflect.DeepEqual(task.CurrentChangedPaths, decoded.CurrentChangedPaths) {
		t.Fatalf("decoded scope=%#v paths=%#v err=%v", decoded.FileScopeRecords, decoded.CurrentChangedPaths, err)
	}
}
