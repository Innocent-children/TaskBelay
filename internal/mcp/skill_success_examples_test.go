package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/version"
)

// These scenarios execute the documented calls through dispatch and the real
// application/store. The observer supplies fixed Git observations; no Host or Git
// operation is represented as a native end-to-end test.
func TestSkillSuccessExamplesMatchExecution(t *testing.T) {
	for _, host := range []string{"codex", "deepseek", "claude", "zcode"} {
		examples := readSkillExamples(t, host)
		for _, example := range examples {
			if example.Kind != "mcp" {
				continue
			}
			t.Run(host+"/"+example.Tool+"/"+example.Name, func(t *testing.T) {
				scenario := newSkillScenario(t, host, examples)
				scenario.prepare(example)
				input := scenario.bind(example)
				output := scenario.server.dispatch(context.Background(), example.Tool, "request-success-example", input)
				if output.IsError {
					t.Fatalf("documented call failed: %s\ninput: %s", output.JSON, input)
				}
				validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(example.Tool)), output.JSON)
				normalizedInput, normalizedOutput := normalizeSkillExecution(input, output.JSON, scenario.task)
				verifySkillSuccessFile(t, host, example, normalizedInput, normalizedOutput)
			})
		}
	}
}

func TestSkillSuccessExampleLineEndings(t *testing.T) {
	t.Setenv("TASKBELAY_UPDATE_SKILL_EXAMPLES", "")
	scenario := newSkillScenario(t, "codex", readSkillExamples(t, "codex"))
	example := scenario.example("handshake")
	input := scenario.bind(example)
	output := scenario.server.dispatch(context.Background(), example.Tool, "request-success-example", input)
	if output.IsError {
		t.Fatalf("documented call failed: %s", output.JSON)
	}
	normalizedInput, normalizedOutput := normalizeSkillExecution(input, output.JSON, scenario.task)
	guide, err := os.ReadFile(example.Path)
	if err != nil {
		t.Fatal(err)
	}
	name := example.Tool + "-" + example.Name + ".md"
	success, err := os.ReadFile(filepath.Join(filepath.Dir(example.Path), "successes", name))
	if err != nil {
		t.Fatal(err)
	}
	for _, ending := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(ending.name, func(t *testing.T) {
			root := t.TempDir()
			fixture := example
			fixture.Path = filepath.Join(root, filepath.Base(example.Path))
			successPath := filepath.Join(root, "successes", name)
			if err := os.MkdirAll(filepath.Dir(successPath), 0755); err != nil {
				t.Fatal(err)
			}
			for path, contents := range map[string][]byte{fixture.Path: guide, successPath: success} {
				text := strings.ReplaceAll(string(contents), "\r\n", "\n")
				text = strings.ReplaceAll(text, "\n", ending.value)
				if err := os.WriteFile(path, []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			}
			verifySkillSuccessFile(t, "codex", fixture, normalizedInput, normalizedOutput)
		})
	}
}

type skillScenario struct {
	t           *testing.T
	host        string
	examples    []skillExample
	server      *Server
	database    *store.SQLite
	observer    *skillSuccessObserver
	task        domain.ProcessTask
	savedAction domain.ID
}

type skillSuccessObserver struct {
	origins  map[string]domain.WorkspaceOrigin
	bindings map[string]domain.RepositoryBinding
}

func (o *skillSuccessObserver) Observe(_ context.Context, path string) (domain.RepositoryBinding, error) {
	b, ok := o.bindings[path]
	if !ok {
		return b, domain.ErrWorkspaceUnavailable
	}
	return b, nil
}
func (o *skillSuccessObserver) ObserveWorkspace(ctx context.Context, path string, _ repository.WorkspaceOriginSelection, _ *domain.RepositoryBinding) (domain.WorkspaceOrigin, domain.RepositoryBinding, error) {
	b, err := o.Observe(ctx, path)
	return o.origins[path], b, err
}
func (o *skillSuccessObserver) ObserveBranchRename(ctx context.Context, path string, selection repository.WorkspaceOriginSelection, previous domain.RepositoryBinding, source, target string) (domain.WorkspaceOrigin, domain.RepositoryBinding, repository.BranchRenameFacts, error) {
	origin, binding, err := o.ObserveWorkspace(ctx, path, selection, &previous)
	facts := repository.BranchRenameFacts{IndexDigest: domain.Digest(strings.Repeat("f", 64))}
	if binding.CurrentBranch != nil {
		if *binding.CurrentBranch == source {
			facts.SourceHead = binding.CurrentHead
		}
		if *binding.CurrentBranch == target {
			facts.TargetHead = binding.CurrentHead
		}
	}
	return origin, binding, facts, err
}
func newSkillScenario(t *testing.T, host string, examples []skillExample) *skillScenario {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "examples.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	observer := &skillSuccessObserver{origins: map[string]domain.WorkspaceOrigin{}, bindings: map[string]domain.RepositoryBinding{}}
	for i, name := range []string{"endpoint-field", "api", "web"} {
		path := testPath("work", "tasks", name)
		origin, binding, _ := mcpWorkspaceFixture(time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), path, byte('a'+i))
		branch := "task/endpoint-field"
		origin.SourceType = "local"
		origin.RemoteName = ""
		origin.BaseCommit = strings.Repeat("1", 40)
		origin.TaskBranch = branch
		origin.ProvisioningReceiptID = "receipt-example"
		if i > 0 {
			origin.ProvisioningReceiptID = domain.ID("receipt-" + filepath.Base(path))
		}
		binding.CurrentBranch = &branch
		binding.CurrentHead = origin.BaseCommit
		binding.HeadTree = origin.BaseCommit
		observer.origins[path] = origin
		observer.bindings[path] = binding
	}
	service, err := application.NewService(database, observer)
	if err != nil {
		t.Fatal(err)
	}
	v, err := version.Current()
	if err != nil {
		t.Fatal(err)
	}
	return &skillScenario{t: t, host: host, examples: examples, database: database, observer: observer, server: &Server{application: service, version: v}}
}
func (s *skillScenario) example(name string) skillExample {
	for _, e := range s.examples {
		if e.Kind == "mcp" && e.Name == name {
			return e
		}
	}
	s.t.Fatalf("missing scenario request %s", name)
	return skillExample{}
}
func (s *skillScenario) call(name string) {
	e := s.example(name)
	out := s.server.dispatch(context.Background(), e.Tool, domain.ID(fmt.Sprintf("setup-%d-%s", s.task.Revision, name)), s.bind(e))
	if out.IsError {
		s.t.Fatalf("setup %s: %s", name, out.JSON)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	json.Unmarshal(out.JSON, &env)
	var id struct {
		TaskID domain.ID `json:"task_id"`
		Task   struct {
			TaskID domain.ID `json:"task_id"`
		} `json:"task"`
	}
	json.Unmarshal(env.Result, &id)
	taskID := id.TaskID
	if taskID == "" {
		taskID = id.Task.TaskID
	}
	if taskID != "" {
		var err error
		s.task, err = s.database.LoadTask(context.Background(), taskID)
		if err != nil {
			s.t.Fatal(err)
		}
	}
}
func (s *skillScenario) prepare(e skillExample) {
	if e.Tool == ToolServerInfo || e.Tool == ToolOpenTask && (e.Name == "create" || e.Name == "multiple") {
		return
	}
	s.call("create")
	target := "REQUIREMENTS"
	switch e.Tool {
	case ToolSubmitDesign:
		target = "DESIGN"
	case ToolSubmitTasks:
		target = "TASKS"
	case ToolSubmitImplementation:
		target = "IMPLEMENT"
	case ToolSubmitTest:
		target = "TEST"
	case ToolSubmitComprehension:
		target = "COMPREHENSION_REVIEW"
	case ToolSubmitDelivery:
		target = "DELIVERY"
	case ToolSubmitRefactor:
		target = "REFACTOR"
	case ToolResolveBlocker:
		target = "IMPLEMENT"
	}
	for string(s.task.CurrentNode) != target {
		switch s.task.CurrentNode {
		case domain.NodeRequirements:
			s.call("requirements_ready")
		case domain.NodeDesign:
			s.call("design_ready")
		case domain.NodeTasks:
			if s.task.TaskPlan == nil {
				s.call("tasks_plan_saved")
			}
			s.call("tasks_ready")
		case domain.NodeImplement:
			if target == "REFACTOR" {
				s.call("implementation_ready_for_test")
			} else {
				s.call("implementation_ready_for_test")
			}
		case domain.NodeTest:
			s.call("tests_passed")
		case domain.NodeComprehensionReview:
			if target == "REFACTOR" {
				s.call("code_too_complex")
			} else {
				s.call("comprehension_passed")
			}
		default:
			s.t.Fatalf("cannot prepare %s from %s", target, s.task.CurrentNode)
		}
	}
	if e.Name == "tasks_ready" {
		s.call("tasks_plan_saved")
	}
	if e.Tool == ToolRecoverAction {
		s.savedAction = s.task.CurrentAction.ActionID
		s.call("requirements_ready")
	}
	if e.Tool == ToolAbandonTask {
		b := s.observer.bindings[s.task.WorkspaceOrigin.CanonicalWorktreeRoot]
		b.WorktreeInstanceDigest = domain.Digest(strings.Repeat("e", 64))
		b.IdentityDigest = b.WorktreeInstanceDigest
		b.BindingDigest = b.WorktreeInstanceDigest
		s.observer.bindings[s.task.WorkspaceOrigin.CanonicalWorktreeRoot] = b
	}
	if e.Tool == ToolResolveBlocker {
		s.prepareBlocker(e.Name)
	}
}
func (s *skillScenario) bind(e skillExample) []byte {
	var input map[string]any
	json.Unmarshal(e.Input, &input)
	if s.task.TaskID != "" {
		if _, ok := input["task_id"]; ok {
			input["task_id"] = s.task.TaskID
		}
		if _, ok := input["action_id"]; ok {
			input["action_id"] = s.task.CurrentAction.ActionID
		}
		if e.Tool == ToolRecoverAction {
			input["action_id"] = s.savedAction
		}
		if _, ok := input["revision"]; ok {
			input["revision"] = s.task.Revision
		}
		if _, ok := input["rename_id"]; ok {
			input["rename_id"] = s.task.BranchRename.RenameID
		}
		if _, ok := input["relocation_id"]; ok {
			input["relocation_id"] = s.task.Relocation.RelocationID
		}
		if result, ok := input["node_result"].(map[string]any); ok {
			if confirmation, ok := result["user_confirmation"].(map[string]any); ok && e.Tool == ToolSubmitTasks {
				confirmation["requirements_digest"] = s.task.Requirements.Digest
				confirmation["design_digest"] = s.task.Design.Digest
				confirmation["task_plan_digest"] = s.task.TaskPlan.Digest
				confirmation["task_plan_revision"] = s.task.TaskPlan.Revision
			}
			if acceptance, ok := result["known_failure_acceptance"].(map[string]any); ok {
				d, _ := s.task.EffectiveWorkspaceDigests()
				acceptance["content_digest"] = d.Content
				acceptance["task_plan_revision"] = s.task.TaskPlan.Revision
			}
			if criteria, ok := result["acceptance"].([]any); ok {
				for _, v := range criteria {
					criterion := v.(map[string]any)
					if ids, ok := criterion["evidence_ids"].([]any); ok {
						for i, id := range ids {
							if id == "evidence-endpoint" {
								ids[i] = s.task.Test.EvidenceIDs[0]
							}
						}
					}
				}
			}
		}
	}
	input = skillRequestPaths(input).(map[string]any)
	raw, err := json.Marshal(input)
	if err != nil {
		s.t.Fatal(err)
	}
	return raw
}
func (s *skillScenario) prepareBlocker(name string) {
	ctx := context.Background()
	switch name {
	case "allow_once", "expand_scope", "reject":
		tool := "apply_patch"
		if s.host == "deepseek" {
			tool = "edit"
		}
		if s.host == "claude" || s.host == "zcode" {
			tool = "Edit"
		}
		_, err := s.server.application.PrepareFileChange(ctx, application.PrepareFileChangeRequest{Host: domain.Host(s.host), RepositoryPath: s.task.WorkspaceOrigin.CanonicalWorktreeRoot, ToolName: tool, Paths: []string{testPath("work", "tasks", "endpoint-field", "src", "extra.js")}, PathParseComplete: true, IntentDigest: domain.Digest(strings.Repeat("d", 64))})
		if err != nil {
			s.t.Fatal(err)
		}
	case "rename-complete", "rename-cancel":
		prepared, err := s.server.application.PrepareTaskBranchRename(ctx, application.PrepareTaskBranchRenameRequest{RequestID: "setup-rename", Host: domain.Host(s.host), TaskID: s.task.TaskID, ExpectedRevision: s.task.Revision, RepositoryKey: s.task.EffectivePrimaryRepositoryKey(), TargetBranch: "task/renamed-endpoint", Reason: "Correct the branch name for the existing Task."})
		if err != nil {
			s.t.Fatal(err)
		}
		s.task = prepared.Task
		if name == "rename-complete" {
			path := s.task.WorkspaceOrigin.CanonicalWorktreeRoot
			b := s.observer.bindings[path]
			branch := "task/renamed-endpoint"
			b.CurrentBranch = &branch
			b.BindingDigest = domain.Digest(strings.Repeat("e", 64))
			s.observer.bindings[path] = b
		}
	case "relocation":
		result, err := s.server.application.PrepareTaskRelocation(ctx, application.PrepareTaskRelocationRequest{Host: domain.Host(s.host), TaskID: s.task.TaskID, ExpectedRevision: s.task.Revision, RequestID: "setup-relocation"})
		if err != nil {
			s.t.Fatal(err)
		}
		s.task = result.Task
		path := testPath("work", "tasks", "relocated-endpoint")
		o := s.observer.origins[s.task.WorkspaceOrigin.CanonicalWorktreeRoot]
		b := s.observer.bindings[s.task.WorkspaceOrigin.CanonicalWorktreeRoot]
		o.CanonicalWorktreeRoot = path
		o.WorktreeGitDirDigest = domain.Digest(strings.Repeat("e", 64))
		b.WorktreeInstanceDigest = o.WorktreeGitDirDigest
		b.IdentityDigest = o.WorktreeGitDirDigest
		b.BindingDigest = o.WorktreeGitDirDigest
		s.observer.origins[path] = o
		s.observer.bindings[path] = b
	case "history":
		b := s.observer.bindings[s.task.WorkspaceOrigin.CanonicalWorktreeRoot]
		b.HistoryRelation = domain.RepositoryHistoryRewrite
		s.observer.bindings[s.task.WorkspaceOrigin.CanonicalWorktreeRoot] = b
		s.server.dispatch(ctx, ToolGetNextAction, "setup-history", s.bind(s.example("guarded-read")))
	case "verification-or-recovery":
		for i := 0; i < 3; i++ {
			s.call("implementation_ready_for_test")
			if i == 0 {
				s.call("verification_budget_increased")
			}
			s.call("tests_failed_implementation")
		}
	default:
		s.t.Fatalf("no blocker setup for %s", name)
	}
	var err error
	s.task, err = s.database.LoadTask(ctx, s.task.TaskID)
	if err != nil {
		s.t.Fatal(err)
	}
	if s.task.CurrentNode != domain.NodeBlocked {
		s.t.Fatalf("setup did not block: %s", s.task.CurrentNode)
	}
}

// Only runtime identities, timestamps and operation digests vary. The same alias
// map is applied to request and response so references remain visible and checked.
func normalizeSkillExecution(input, output []byte, task domain.ProcessTask) ([]byte, []byte) {
	aliases := map[string]string{}
	counts := map[string]int{}
	if task.TaskID != "" {
		aliases[string(task.TaskID)] = "task-example"
		if task.CurrentAction != nil {
			aliases[string(task.CurrentAction.ActionID)] = "action-example"
		}
	}
	randomID := regexp.MustCompile(`^([a-z_]+)-[0-9a-f]{32}$`)
	var walk func(any, string) any
	walk = func(value any, key string) any {
		switch v := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v[k] = walk(v[k], k)
			}
			return v
		case []any:
			for i := range v {
				v[i] = walk(v[i], key)
			}
			return v
		case string:
			if strings.HasPrefix(v, testPath("work")+string(filepath.Separator)) {
				return "/work/" + filepath.ToSlash(strings.TrimPrefix(v, testPath("work")+string(filepath.Separator)))
			}
			if alias, ok := aliases[v]; ok {
				return alias
			}
			if m := randomID.FindStringSubmatch(v); m != nil {
				counts[m[1]]++
				alias := fmt.Sprintf("%s-generated-%d", m[1], counts[m[1]])
				aliases[v] = alias
				return alias
			}
			if _, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return "2026-09-10T00:00:00Z"
			}
			if key == "payload_digest" {
				return strings.Repeat("d", 64)
			}
		}
		return value
	}
	normalize := func(raw []byte) []byte {
		var v any
		json.Unmarshal(raw, &v)
		b, _ := json.MarshalIndent(walk(v, ""), "", "  ")
		return b
	}
	return normalize(input), normalize(output)
}
func verifySkillSuccessFile(t *testing.T, host string, e skillExample, input, output []byte) {
	t.Helper()
	name := e.Tool + "-" + e.Name + ".md"
	path := filepath.Join(filepath.Dir(e.Path), "successes", name)
	if strings.Contains(filepath.ToSlash(e.Path), "/nodes/") {
		path = filepath.Join(filepath.Dir(e.Path), "..", "successes", name)
	}
	if os.Getenv("TASKBELAY_UPDATE_SKILL_EXAMPLES") == "1" && host == "codex" {
		source := filepath.Join("..", "..", "skills", "taskbelay", "core", "successes", name)
		text := "# " + e.Tool + ": " + e.Name + "\n\nImplementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.\n\nComplete successful call from the documented scenario. The repository observer is a fixed test fixture;\nHost authorization and Git operations are not executed here. Runtime IDs and timestamps use stable\nexample values, and operation digests use a sample digest. All other fields are compared with the\nactual Core response. The resolved request below shows the current Task values substituted for the\nidentity and confirmation placeholders in the calling reference.\n\nResolved request:\n\n<!-- example:resolved-mcp " + e.Tool + " " + e.Name + " -->\n```json\n" + string(input) + "\n```\n\nComplete response:\n\n<!-- example:mcp-success " + e.Tool + " " + e.Name + " -->\n```json\n" + string(output) + "\n```\n"
		for _, key := range []string{"host", "origin_host"} {
			text = strings.ReplaceAll(text, `"`+key+`": "codex"`, `"`+key+`": "{{host}}"`)
		}
		if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("TASKBELAY_UPDATE_SKILL_EXAMPLES") == "1" {
		return
	}
	guide, err := os.ReadFile(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	link, err := filepath.Rel(filepath.Dir(e.Path), path)
	if err != nil || !strings.Contains(string(guide), "]("+filepath.ToSlash(link)+")") {
		t.Fatalf("%s must link its complete response", e.Path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(contents), "\r\n", "\n")
	blocks := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindAllStringSubmatch(text, -1)
	if len(blocks) != 2 {
		t.Fatal("success file requires its resolved request and complete response")
	}
	for i, raw := range [][]byte{input, output} {
		var got, want any
		json.Unmarshal(raw, &got)
		if err := json.Unmarshal([]byte(blocks[i][1]), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("success example %s block %d differs from execution; regenerate with TASKBELAY_UPDATE_SKILL_EXAMPLES=1: %s", path, i, skillValueDifference(got, want, "$"))
		}
	}
}

func skillValueDifference(got, want any, path string) string {
	if reflect.DeepEqual(got, want) {
		return ""
	}
	switch g := got.(type) {
	case map[string]any:
		w, ok := want.(map[string]any)
		if ok {
			keys := make([]string, 0, len(g))
			for k := range g {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if d := skillValueDifference(g[k], w[k], path+"."+k); d != "" {
					return d
				}
			}
		}
	case []any:
		w, ok := want.([]any)
		if ok && len(g) == len(w) {
			for i := range g {
				if d := skillValueDifference(g[i], w[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
					return d
				}
			}
		}
	}
	return fmt.Sprintf("%s got=%v want=%v", path, got, want)
}

// Request paths use the test platform's stable absolute root; examples retain
// portable POSIX spellings independently of where the tests execute.
func skillRequestPaths(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			v[k] = skillRequestPaths(item)
		}
		return v
	case []any:
		for i := range v {
			v[i] = skillRequestPaths(v[i])
		}
		return v
	case string:
		if strings.HasPrefix(v, "/work/") {
			return testPath(strings.Split(strings.TrimPrefix(v, "/"), "/")...)
		}
	}
	return value
}
