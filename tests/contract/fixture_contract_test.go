package contract_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	core "github.com/Innocent-children/taskbelay/internal/mcp"
)

func TestFixtureFilesExist(t *testing.T) {
	root := contractRepositoryRoot(t)
	for _, name := range []string{
		"graph-server-info.json",
		"graph-multi-repository-open.json",
		"graph-workspace-lifecycle.json",
		"graph-open-requirements.json",
		"graph-design-action.json",
		"graph-invalid-edge.json",
		"graph-host-parity-codex.json",
		"graph-host-parity-deepseek.json",
	} {
		if _, err := os.Stat(filepath.Join(root, "protocol", "fixtures", name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceLifecycleFixtureMatchesClosedMCPInputs(t *testing.T) {
	path := filepath.Join(contractRepositoryRoot(t), "protocol", "fixtures", "graph-workspace-lifecycle.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureKind               string          `json:"fixture_kind"`
		StorageSchemaVersion      string          `json:"storage_schema_version"`
		WorkspaceOrigin           json.RawMessage `json:"workspace_origin"`
		PrepareRelocationInput    json.RawMessage `json:"prepare_relocation_input"`
		RelocationResolutionInput json.RawMessage `json:"relocation_resolution_input"`
		HistoryResolutionInput    json.RawMessage `json:"history_resolution_input"`
		AbandonInput              json.RawMessage `json:"abandon_input"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.FixtureKind != "workspace_lifecycle" || fixture.StorageSchemaVersion != "0.8.0" {
		t.Fatalf("workspace lifecycle identity=%q/%q", fixture.FixtureKind, fixture.StorageSchemaVersion)
	}
	for tool, input := range map[string]json.RawMessage{
		"taskbelay_prepare_task_relocation": fixture.PrepareRelocationInput,
		"taskbelay_resolve_blocker":         fixture.RelocationResolutionInput,
		"taskbelay_abandon_task":            fixture.AbandonInput,
	} {
		if err := core.ValidateToolInput(tool, input); err != nil {
			t.Fatalf("%s fixture input: %v", tool, err)
		}
	}
	if err := core.ValidateToolInput("taskbelay_resolve_blocker", fixture.HistoryResolutionInput); err != nil {
		t.Fatalf("history fixture input: %v", err)
	}
	var origin map[string]any
	if json.Unmarshal(fixture.WorkspaceOrigin, &origin) != nil || len(origin) != 8 || origin["mode"] != "dedicated_worktree" {
		t.Fatalf("workspace origin=%s", fixture.WorkspaceOrigin)
	}
}

type hostParityFixture struct {
	FixtureKind          string                  `json:"fixture_kind"`
	Host                 string                  `json:"host"`
	OriginHost           string                  `json:"origin_host"`
	RequestID            string                  `json:"request_id"`
	ProcessID            string                  `json:"process_id"`
	DefinitionDigest     string                  `json:"definition_digest"`
	CurrentNode          string                  `json:"current_node"`
	ActionKind           string                  `json:"action_kind"`
	SubmissionTool       string                  `json:"submission_tool"`
	NodeContract         hostParityNodeContract  `json:"node_contract"`
	MethodProfileEnum    []string                `json:"method_profile_enum"`
	SemanticMethodSteps  []string                `json:"semantic_method_steps"`
	AvailableTransitions []string                `json:"available_transitions"`
	SubmissionInputShape []string                `json:"submission_input_shape"`
	ProblemClassRules    []hostParityProblemRule `json:"problem_class_rules"`
	ErrorShape           hostParityErrorShape    `json:"error_shape"`
}

type hostParityNodeContract struct {
	Purpose                string   `json:"purpose"`
	EntryConditionIDs      []string `json:"entry_condition_ids"`
	CompletionConditionIDs []string `json:"completion_condition_ids"`
	AllowedEffects         []string `json:"allowed_effects"`
	RequiredEvidence       []string `json:"required_evidence"`
}

type hostParityProblemRule struct {
	ProblemClass   string `json:"problem_class"`
	TransitionID   string `json:"transition_id"`
	ReasonRequired bool   `json:"reason_required"`
}

type hostParityErrorShape struct {
	Required         []string `json:"required"`
	RecoveryRequired []string `json:"recovery_required"`
}

func TestCurrentHostParityFixtures(t *testing.T) {
	root := contractRepositoryRoot(t)
	codex, codexRaw := readHostParityFixture(t, filepath.Join(root, "protocol", "fixtures", "graph-host-parity-codex.json"))
	deepseek, deepseekRaw := readHostParityFixture(t, filepath.Join(root, "protocol", "fixtures", "graph-host-parity-deepseek.json"))

	if codex.Host != "codex" || codex.OriginHost != "codex" {
		t.Fatalf("Codex identity = %q/%q", codex.Host, codex.OriginHost)
	}
	if deepseek.Host != "deepseek" || deepseek.OriginHost != "deepseek" {
		t.Fatalf("DeepSeek identity = %q/%q", deepseek.Host, deepseek.OriginHost)
	}
	if codex.RequestID == deepseek.RequestID {
		t.Fatal("fixture request IDs must remain opaque and distinct")
	}

	for name, fixture := range map[string]hostParityFixture{"codex": codex, "deepseek": deepseek} {
		if fixture.FixtureKind != "core_host_parity" {
			t.Fatalf("%s contract identity = %#v", name, fixture)
		}
		if fixture.ProcessID != "standard-development" || fixture.CurrentNode != "REQUIREMENTS" || fixture.ActionKind != "COMPLETE_REQUIREMENTS" || fixture.SubmissionTool != "taskbelay_submit_requirements" {
			t.Fatalf("%s process/action identity = %#v", name, fixture)
		}
		if len(fixture.DefinitionDigest) != 64 {
			t.Fatalf("%s definition digest = %q", name, fixture.DefinitionDigest)
		}
		if !reflect.DeepEqual(fixture.MethodProfileEnum, []string{"plain", "spec-kit", "openspec"}) {
			t.Fatalf("%s method profiles = %#v", name, fixture.MethodProfileEnum)
		}
		if !reflect.DeepEqual(fixture.SemanticMethodSteps, []string{"requirements.capture", "requirements.clarify", "requirements.validate"}) {
			t.Fatalf("%s semantic method steps = %#v", name, fixture.SemanticMethodSteps)
		}
		if !reflect.DeepEqual(fixture.AvailableTransitions, []string{"requirements_ready"}) {
			t.Fatalf("%s transitions = %#v", name, fixture.AvailableTransitions)
		}
		if !reflect.DeepEqual(fixture.SubmissionInputShape, []string{"host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"}) {
			t.Fatalf("%s submission shape = %#v", name, fixture.SubmissionInputShape)
		}
		if fixture.NodeContract.Purpose == "" || len(fixture.NodeContract.EntryConditionIDs) != 3 || len(fixture.NodeContract.CompletionConditionIDs) != 6 || len(fixture.NodeContract.AllowedEffects) != 3 || len(fixture.NodeContract.RequiredEvidence) != 2 {
			t.Fatalf("%s node contract is incomplete: %#v", name, fixture.NodeContract)
		}
		if len(fixture.ProblemClassRules) != 1 || fixture.ProblemClassRules[0] != (hostParityProblemRule{ProblemClass: "none", TransitionID: "requirements_ready", ReasonRequired: false}) {
			t.Fatalf("%s problem class rules = %#v", name, fixture.ProblemClassRules)
		}
		if !reflect.DeepEqual(fixture.ErrorShape.Required, []string{"code", "message", "recovery"}) || !reflect.DeepEqual(fixture.ErrorShape.RecoveryRequired, []string{"retry_safe", "action", "message"}) {
			t.Fatalf("%s error shape = %#v", name, fixture.ErrorShape)
		}
	}

	normalizeHostFixture := func(fixture hostParityFixture) hostParityFixture {
		fixture.Host = ""
		fixture.OriginHost = ""
		fixture.RequestID = ""
		return fixture
	}
	if !reflect.DeepEqual(normalizeHostFixture(codex), normalizeHostFixture(deepseek)) {
		t.Fatal("Host parity fixtures differ outside host, origin_host, and request_id")
	}

	for name, raw := range map[string][]byte{"codex": codexRaw, "deepseek": deepseekRaw} {
		text := string(raw)
		for _, forbidden := range []string{`"phase"`, `"source_phase"`, `"destination"`, `"new_task"`, `"product_support"`, `"adapter_implemented"`, `"native_journey"`} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s fixture contains forbidden field %s", name, forbidden)
			}
		}
	}
}

func readHostParityFixture(t *testing.T, path string) (hostParityFixture, []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var fixture hostParityFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode closed fixture %s: %v", filepath.Base(path), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture %s has trailing JSON", filepath.Base(path))
	}
	return fixture, raw
}

func TestGraphServerInfoFixtureContainsCompletePublicDTO(t *testing.T) {
	path := filepath.Join(contractRepositoryRoot(t), "protocol", "fixtures", "graph-server-info.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || len(value) != 9 {
		t.Fatalf("incomplete top-level fixture: %s", raw)
	}
	for _, key := range []string{"product", "version", "transport", "health", "supported_hosts", "supported_processes", "method_profiles", "tools", "host_preferences"} {
		if _, ok := value[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
	processes, ok := value["supported_processes"].([]any)
	if !ok || len(processes) != 1 {
		t.Fatal("supported_processes is incomplete")
	}
	process, ok := processes[0].(map[string]any)
	if !ok || len(process) != 3 || process["definition_digest"] == nil || process["new_task_supported"] != true || process["process_definition_digest"] != nil {
		t.Fatalf("public process DTO=%#v", process)
	}
	tools, ok := value["tools"].([]any)
	if !ok || len(tools) != 18 {
		t.Fatalf("tools=%#v", tools)
	}
	preferences, ok := value["host_preferences"].(map[string]any)
	if !ok || len(preferences) != 4 || preferences["codex"].(map[string]any)["codebase_memory"] != false || preferences["deepseek"].(map[string]any)["codebase_memory"] != false || preferences["claude"].(map[string]any)["codebase_memory"] != false || preferences["zcode"].(map[string]any)["codebase_memory"] != false {
		t.Fatalf("host_preferences=%#v", value["host_preferences"])
	}
	previous := -1
	for _, key := range []string{`"product"`, `"version"`, `"transport"`, `"health"`, `"supported_hosts"`, `"supported_processes"`, `"method_profiles"`, `"tools"`, `"host_preferences"`} {
		index := strings.Index(string(raw), key)
		if index <= previous {
			t.Fatalf("field order drift at %s", key)
		}
		previous = index
	}
}

type multiRepositoryOpenFixture struct {
	FixtureKind string                     `json:"fixture_kind"`
	Created     bool                       `json:"created"`
	Task        multiRepositoryFixtureTask `json:"task"`
}

type multiRepositoryFixtureTask struct {
	TaskID                  string                             `json:"task_id"`
	OriginHost              string                             `json:"origin_host"`
	ProcessID               string                             `json:"process_id"`
	ProcessDefinitionDigest string                             `json:"process_definition_digest"`
	CurrentCursor           string                             `json:"current_cursor"`
	Revision                uint64                             `json:"revision"`
	PrimaryRepositoryKey    string                             `json:"primary_repository_key"`
	WorkspaceOrigin         multiRepositoryFixtureOrigin       `json:"workspace_origin"`
	Repository              multiRepositoryFixtureRepository   `json:"repository"`
	AdditionalRepositories  []multiRepositoryFixtureAdditional `json:"additional_repositories"`
	CurrentChangedPaths     []string                           `json:"current_changed_paths"`
	CurrentAction           multiRepositoryFixtureAction       `json:"current_action"`
}

type multiRepositoryFixtureAdditional struct {
	Key             string                           `json:"key"`
	WorkspaceOrigin multiRepositoryFixtureOrigin     `json:"workspace_origin"`
	Repository      multiRepositoryFixtureRepository `json:"repository"`
}

type multiRepositoryFixtureOrigin struct {
	Mode                        string `json:"mode"`
	SourceType                  string `json:"source_type"`
	CarryChanges                bool   `json:"carry_changes"`
	RemoteName                  string `json:"remote_name"`
	BaseBranch                  string `json:"base_branch"`
	BaseCommit                  string `json:"base_commit"`
	TaskBranch                  string `json:"task_branch"`
	SourceRepositoryGroupDigest string `json:"source_repository_group_digest"`
	CanonicalWorktreeRoot       string `json:"canonical_worktree_root"`
	WorktreeGitDirDigest        string `json:"worktree_git_dir_digest"`
	ProvisioningReceiptID       string `json:"provisioning_receipt_id"`
}

type multiRepositoryFixtureRepository struct {
	WorktreeInstanceDigest string  `json:"worktree_instance_digest"`
	IdentityDigest         string  `json:"identity_digest"`
	HistoryDigest          string  `json:"history_digest"`
	ContentDigest          string  `json:"content_digest"`
	CurrentBranch          *string `json:"current_branch"`
	Detached               bool    `json:"detached"`
	CurrentHead            string  `json:"current_head"`
	HeadTree               string  `json:"head_tree"`
	HistoryRelation        string  `json:"history_relation"`
	BaseCommitAncestor     bool    `json:"base_commit_ancestor"`
	ChangedEntries         []any   `json:"changed_entries"`
	TaskSurface            []any   `json:"task_surface"`
	ObservedAt             string  `json:"observed_at"`
	BindingDigest          string  `json:"binding_digest"`
}

type multiRepositoryFixtureAction struct {
	TaskID                  string `json:"task_id"`
	Revision                uint64 `json:"revision"`
	ActionID                string `json:"action_id"`
	ActionKind              string `json:"action_kind"`
	SubmissionTool          string `json:"submission_tool"`
	RepositoryBindingDigest string `json:"repository_binding_digest"`
	IssuanceIdentityDigest  string `json:"issuance_identity_digest"`
	IssuanceHistoryDigest   string `json:"issuance_history_digest"`
	IssuanceContentDigest   string `json:"issuance_content_digest"`
}

func TestGraphMultiRepositoryOpenFixtureUsesOneTaskActionAndDigest(t *testing.T) {
	path := filepath.Join(contractRepositoryRoot(t), "protocol", "fixtures", "graph-multi-repository-open.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var fixture multiRepositoryOpenFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("multi repository fixture has trailing JSON")
	}
	const effectiveDigest = "9999999999999999999999999999999999999999999999999999999999999999"
	if fixture.FixtureKind != "multi_repository_open" || !fixture.Created || fixture.Task.TaskID == "" || fixture.Task.ProcessID != "standard-development" || len(fixture.Task.ProcessDefinitionDigest) != 64 || fixture.Task.Revision != 1 || fixture.Task.CurrentAction.TaskID != fixture.Task.TaskID || fixture.Task.CurrentAction.Revision != fixture.Task.Revision || fixture.Task.CurrentAction.SubmissionTool != "taskbelay_submit_requirements" {
		t.Fatalf("task/action identity=%#v", fixture)
	}
	if fixture.Task.PrimaryRepositoryKey != "core" || fixture.Task.WorkspaceOrigin.CanonicalWorktreeRoot != "/workspace/core" || len(fixture.Task.AdditionalRepositories) != 1 || fixture.Task.AdditionalRepositories[0].Key != "docs" || fixture.Task.AdditionalRepositories[0].WorkspaceOrigin.CanonicalWorktreeRoot != "/workspace/docs" || len(fixture.Task.CurrentChangedPaths) != 0 {
		t.Fatalf("repository scope=%#v", fixture.Task)
	}
	if fixture.Task.CurrentAction.RepositoryBindingDigest != effectiveDigest || fixture.Task.CurrentAction.RepositoryBindingDigest == fixture.Task.Repository.BindingDigest || fixture.Task.CurrentAction.RepositoryBindingDigest == fixture.Task.AdditionalRepositories[0].Repository.BindingDigest {
		t.Fatalf("effective digest=%#v", fixture.Task.CurrentAction)
	}
	if bytes.Count(raw, []byte(`"repository_binding_digest"`)) != 1 || bytes.Contains(raw, []byte("repository_scope_digest")) {
		t.Fatalf("digest fields=%s", raw)
	}
	for _, digest := range []string{fixture.Task.CurrentAction.IssuanceIdentityDigest, fixture.Task.CurrentAction.IssuanceHistoryDigest, fixture.Task.CurrentAction.IssuanceContentDigest} {
		if len(digest) != 64 {
			t.Fatalf("invalid issuance digest %q", digest)
		}
	}
}
