package webui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type readHandlers struct {
	reader ControlCenterReader
	status SystemStatusProvider
}

func (h *readHandlers) dashboard(w http.ResponseWriter, r *http.Request) {
	requestID := readRequestID()
	dashboard, err := h.reader.Dashboard(r.Context())
	if err != nil {
		writeReadError(w, requestID, err)
		return
	}
	counts := []DashboardCount{
		{Lifecycle: LifecycleActive, Count: dashboard.Counts["active"]},
		{Lifecycle: LifecycleBlocked, Count: dashboard.Counts["blocked"]},
		{Lifecycle: LifecycleDone, Count: dashboard.Counts["done"]},
		{Lifecycle: LifecycleCancelled, Count: dashboard.Counts["cancelled"]},
	}
	_ = WriteJSON(w, http.StatusOK, DashboardResponse{OK: true, RequestID: requestID, Readiness: ReadinessReady, Counts: counts, Recent: projectSummaries(dashboard.Recent)})
}

func (h *readHandlers) taskList(w http.ResponseWriter, r *http.Request) {
	requestID := readRequestID()
	filter, err := parseTaskListFilter(r.URL.Query())
	if err != nil {
		writeReadError(w, requestID, err)
		return
	}
	result, err := h.reader.ListTasks(r.Context(), application.ListControlCenterTasksRequest{Filter: filter})
	if err != nil {
		writeReadError(w, requestID, err)
		return
	}
	_ = WriteJSON(w, http.StatusOK, TaskListResponse{OK: true, RequestID: requestID, Readiness: ReadinessReady, Page: result.Page, HasNext: result.HasNext, Items: projectSummaries(result.Items)})
}

func (h *readHandlers) taskDetail(w http.ResponseWriter, r *http.Request) {
	requestID := readRequestID()
	taskID := domain.ID(r.PathValue("task_id"))
	result, err := h.reader.GetTaskDetail(r.Context(), application.GetControlCenterTaskRequest{TaskID: taskID})
	if err != nil {
		writeReadError(w, requestID, err)
		return
	}
	response, err := projectTaskDetail(requestID, result)
	if err != nil {
		writeReadError(w, requestID, domain.ErrInternal)
		return
	}
	_ = WriteJSON(w, http.StatusOK, response)
}

func (h *readHandlers) systemStatus(w http.ResponseWriter, _ *http.Request) {
	status := h.status()
	status.OK = true
	status.RequestID = readRequestID()
	_ = WriteJSON(w, http.StatusOK, status)
}

func (h *readHandlers) filterOptions(w http.ResponseWriter, _ *http.Request) {
	definition := workflow.StandardProcess()
	nodeIDs := make([]string, len(definition.Nodes))
	for index, node := range definition.Nodes {
		nodeIDs[index] = string(node.NodeID)
	}
	_ = WriteJSON(w, http.StatusOK, FilterOptionsResponse{OK: true, RequestID: readRequestID(), NodeIDs: nodeIDs})
}

func parseTaskListFilter(values url.Values) (application.TaskListFilter, error) {
	allowed := map[string]bool{"text": true, "host": true, "repository": true, "node": true, "lifecycle": true, "updated_from": true, "updated_to": true, "page": true}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return application.TaskListFilter{}, domain.ErrInvalidArgument
		}
	}
	filter := application.TaskListFilter{Text: values.Get("text"), Host: domain.Host(values.Get("host")), Repository: values.Get("repository"), Node: domain.NodeID(values.Get("node")), Lifecycle: values.Get("lifecycle"), Page: 1}
	if raw := values.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return application.TaskListFilter{}, domain.ErrInvalidArgument
		}
		filter.Page = page
	}
	for raw, target := range map[string]**time.Time{"updated_from": &filter.UpdatedFrom, "updated_to": &filter.UpdatedTo} {
		if value := values.Get(raw); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return application.TaskListFilter{}, domain.ErrInvalidArgument
			}
			parsed = parsed.UTC()
			*target = &parsed
		}
	}
	return filter, nil
}

func projectSummaries(items []application.ControlCenterTaskSummary) []TaskSummary {
	result := make([]TaskSummary, len(items))
	for index, item := range items {
		keys := make([]string, len(item.RepositoryKeys))
		for keyIndex, key := range item.RepositoryKeys {
			keys[keyIndex] = string(key)
		}
		result[index] = TaskSummary{TaskID: string(item.TaskID), RequestSummary: item.RequestSummary, OriginHost: string(item.OriginHost), ExecutionHost: string(item.ExecutionHost), CurrentNode: string(item.CurrentNode), Lifecycle: Lifecycle(item.Lifecycle), Revision: item.Revision, UpdatedAt: item.UpdatedAt, Archived: item.Archived, RepositoryKeys: keys, RepositoryGroupID: string(item.RepositoryGroupID), WorktreePath: item.WorktreePath, Blocker: item.Blocker, Outcome: item.Outcome}
	}
	return result
}

func projectTaskDetail(requestID string, detail application.ControlCenterTaskDetail) (TaskDetailResponse, error) {
	summary := projectSummaries([]application.ControlCenterTaskSummary{summarizeDetail(detail)})[0]
	repositories := []RepositoryView{projectRepository(
		detail.Task.EffectivePrimaryRepositoryKey(), "primary", detail.Task.WorkspaceOrigin, detail.Task.Repository,
	)}
	for _, repository := range detail.Task.AdditionalRepositories {
		repositories = append(repositories, projectRepository(repository.Key, "additional", repository.Origin, repository.Binding))
	}
	history, err := detail.Task.ReadBaselineHistory(domain.BaselineHistoryQuery{Revision: detail.Task.Revision, Limit: domain.MaxBaselineHistoryPageEntries})
	if err != nil {
		return TaskDetailResponse{}, err
	}
	baselines, err := projectNamedFacts([]namedFact{{"requirements", "Requirements", detail.Task.Requirements}, {"design", "Design", detail.Task.Design}, {"task_plan", "Task plan", detail.Task.TaskPlan}, {"baseline_history", "Baseline history first page (complete history: taskbelay_get_task baseline_history)", history}, {"branch_rename", "Prepared branch rename", detail.Task.BranchRename}})
	if err != nil {
		return TaskDetailResponse{}, err
	}
	records, err := projectNamedFacts([]namedFact{{"implementation", "Implementation", detail.Task.Implementation}, {"test", "Test", detail.Task.Test}, {"comprehension", "Comprehension", detail.Task.Comprehension}, {"verification_attempts", "Recent verification attempts", detail.Task.VerificationAttempts}, {"file_scope_records", "File scope decisions", detail.Task.FileScopeRecords}, {"last_operation", "Last operation", detail.Task.LastOperation}})
	if err != nil {
		return TaskDetailResponse{}, err
	}
	evidence := make([]Fact, 0, len(detail.Task.Evidence))
	for _, item := range detail.Task.Evidence {
		fact, factErr := projectFact("evidence", item.Name, item)
		if factErr != nil {
			return TaskDetailResponse{}, factErr
		}
		evidence = append(evidence, fact)
	}
	var blocker, outcome *Fact
	if detail.Task.Blocker != nil {
		fact, factErr := projectFact("blocker", "Current blocker", detail.Task.Blocker)
		if factErr != nil {
			return TaskDetailResponse{}, factErr
		}
		blocker = &fact
	}
	if detail.Task.Outcome != nil {
		fact, factErr := projectFact("outcome", "Outcome", detail.Task.Outcome)
		if factErr != nil {
			return TaskDetailResponse{}, factErr
		}
		outcome = &fact
	}
	events := make([]TaskEventView, len(detail.Events))
	for index, event := range detail.Events {
		var transitionID, reason *string
		if event.TransitionID != nil {
			value := string(*event.TransitionID)
			transitionID = &value
		}
		if event.TransitionReason != "" {
			value := event.TransitionReason
			reason = &value
		}
		events[index] = TaskEventView{Revision: event.Revision, EventType: string(event.Kind), SourceNode: string(event.SourceNode), DestinationNode: string(event.DestinationNode), TransitionID: transitionID, Reason: reason, RepositoryDeltaPaths: append([]string{}, event.RepositoryDeltaPaths...), CreatedAt: event.CreatedAt}
	}
	readiness := ReadinessReady
	if detail.ReadOnly {
		readiness = ReadinessReadOnly
	}
	criteria := append([]string{}, detail.Task.Intent.KnownAcceptanceCriteria...)
	currentAction, err := projectAction(detail.Task.CurrentAction, detail.Task.Blocker)
	if err != nil {
		return TaskDetailResponse{}, err
	}
	scope := application.CurrentFileScopeStatus(detail.Task)
	fileScope := FileScopeView{
		ExpectedPaths:       append([]string{}, scope.ExpectedPaths...),
		CurrentChangedPaths: append([]string{}, detail.Task.CurrentChangedPaths...),
		UnexplainedPaths:    append([]string{}, scope.UnexplainedPaths...),
		CoveredHostTools:    append([]string{}, scope.CoveredHostTools...),
		DecisionCount:       len(scope.Records),
		FinalCheckEnabled:   scope.FinalCheckEnabled,
	}
	workspace := projectWorkspace(detail.Task)
	return TaskDetailResponse{PendingActionID: actionIDText(detail.PendingActionID), OK: true, RequestID: requestID, Readiness: readiness, Summary: summary, Intent: detail.Task.Intent.Request, AcceptanceCriteria: criteria, Verification: projectVerification(detail.Task), MethodProfile: string(detail.Task.Intent.MethodProfile), Repositories: repositories, Baselines: baselines, Records: records, Evidence: evidence, Blocker: blocker, Outcome: outcome, Events: events, Graph: projectGraph(detail.Graph), CurrentAction: currentAction, FileScope: fileScope, Workspace: workspace}, nil
}

func projectVerification(task domain.ProcessTask) VerificationView {
	usage := task.CurrentVerificationUsage()
	view := VerificationView{Usage: VerificationUsageView{AutomaticCommands: usage.AutomaticCommands, FullSuiteRuns: usage.FullSuiteRuns, EvidenceItems: usage.EvidenceItems}, Adjustments: []VerificationAdjustmentView{}}
	if task.TaskPlan != nil {
		checks := make([]VerificationPlanCheckView, len(task.TaskPlan.VerificationPlan.Checks))
		for index, check := range task.TaskPlan.VerificationPlan.Checks {
			checks[index] = VerificationPlanCheckView{Name: check.Name, Rationale: check.Rationale}
		}
		initial := projectVerificationBudget(task.TaskPlan.VerificationPlan.InitialBudget)
		view.Plan = &VerificationPlanView{Checks: checks, InitialBudget: initial, FullSuiteExpected: task.TaskPlan.VerificationPlan.FullSuiteExpected, TestCodeChangesExpected: task.TaskPlan.VerificationPlan.TestCodeChangesExpected}
		if budget, ok := task.CurrentVerificationBudget(); ok {
			projected := projectVerificationBudget(budget)
			view.CurrentBudget = &projected
		}
	}
	for _, adjustment := range task.VerificationBudgetAdjustments {
		checks := make([]VerificationPlanCheckView, len(adjustment.AdditionalChecks))
		for index, check := range adjustment.AdditionalChecks {
			checks[index] = VerificationPlanCheckView{Name: check.Name, Rationale: check.Rationale}
		}
		view.Adjustments = append(view.Adjustments, VerificationAdjustmentView{Revision: adjustment.Revision, TaskPlanRevision: adjustment.TaskPlanRevision, Basis: string(adjustment.Basis), Reason: adjustment.Reason, AdditionalChecks: checks, AdditionalAutomaticCommands: adjustment.AdditionalAutomaticCommands, AllowFullSuite: adjustment.AllowFullSuite, AllowManualHandoff: adjustment.AllowManualHandoff, CurrentBudget: projectVerificationBudget(adjustment.CurrentBudget), CreatedAt: adjustment.CreatedAt})
	}
	return view
}

func projectVerificationBudget(budget domain.VerificationBudget) VerificationBudgetView {
	return VerificationBudgetView{Level: string(budget.Level), MaxAutomaticCommands: budget.MaxAutomaticCommands, AllowFullSuite: budget.AllowFullSuite, AllowManualHandoff: budget.AllowManualHandoff}
}

func summarizeDetail(detail application.ControlCenterTaskDetail) application.ControlCenterTaskSummary {
	keys := []domain.RepositoryKey{detail.Task.EffectivePrimaryRepositoryKey()}
	for _, repository := range detail.Task.AdditionalRepositories {
		keys = append(keys, repository.Key)
	}
	var blocker, outcome *string
	if detail.Task.Blocker != nil {
		value := detail.Task.Blocker.Message
		blocker = &value
	}
	if detail.Task.Outcome != nil {
		value := detail.Task.Outcome.Summary
		outcome = &value
	}
	return application.ControlCenterTaskSummary{TaskID: detail.Task.TaskID, RequestSummary: truncateWebSummary(detail.Task.Intent.Request), OriginHost: detail.Task.OriginHost, ExecutionHost: detail.Task.OriginHost, CurrentNode: detail.Task.CurrentNode, Lifecycle: lifecycleFromNode(detail.Task.CurrentNode), Revision: detail.Task.Revision, UpdatedAt: detail.Task.UpdatedAt, Archived: detail.Archived, RepositoryKeys: keys, RepositoryGroupID: detail.Task.WorkspaceOrigin.SourceRepositoryGroupDigest, WorktreePath: detail.Task.WorkspaceOrigin.CanonicalWorktreeRoot, Blocker: blocker, Outcome: outcome}
}

func projectRepository(key domain.RepositoryKey, role string, origin domain.WorkspaceOrigin, binding domain.RepositoryBinding) RepositoryView {
	return RepositoryView{
		Key: string(key), Path: origin.CanonicalWorktreeRoot, Role: role,
		RepositoryGroupID: string(origin.SourceRepositoryGroupDigest),
		Origin: WorkspaceOriginView{
			Mode: string(origin.Mode), SourceType: origin.SourceType, CarryChanges: origin.CarryChanges, RemoteName: origin.RemoteName, BaseBranch: origin.BaseBranch,
			BaseCommit: origin.BaseCommit, TaskBranch: origin.TaskBranch,
			ProvisioningReceiptID: string(origin.ProvisioningReceiptID),
		},
		Observation: WorkspaceObservationView{
			WorktreeInstanceDigest: string(binding.WorktreeInstanceDigest), IdentityDigest: string(binding.IdentityDigest),
			HistoryDigest: string(binding.HistoryDigest), ContentDigest: string(binding.ContentDigest),
			CurrentBranch: cloneString(binding.CurrentBranch), Detached: binding.Detached,
			CurrentHead: binding.CurrentHead, HeadTree: binding.HeadTree, HistoryRelation: string(binding.HistoryRelation),
			BaseCommitAncestor: binding.BaseCommitAncestor,
			ChangedEntries:     projectChangedEntries(binding.ChangedEntries), TaskSurface: projectChangedEntries(binding.TaskSurface),
			ObservedAt: binding.ObservedAt, BindingDigest: string(binding.BindingDigest),
		},
	}
}

func projectWorkspace(task domain.ProcessTask) WorkspaceView {
	hasWorktree := task.WorkspaceOrigin.Mode == domain.WorkspaceModeDedicatedWorktree
	conflict := task.Repository.HistoryRelation != domain.RepositoryHistoryExact && task.Repository.HistoryRelation != domain.RepositoryHistoryLinearAdvance
	for _, repository := range task.AdditionalRepositories {
		hasWorktree = hasWorktree || repository.Origin.Mode == domain.WorkspaceModeDedicatedWorktree
		conflict = conflict || repository.Binding.HistoryRelation != domain.RepositoryHistoryExact && repository.Binding.HistoryRelation != domain.RepositoryHistoryLinearAdvance
	}
	var relocationID, resumeNode *string
	pending := task.Blocker != nil && task.Blocker.Cause == domain.BlockerCauseTaskRelocationPending
	if pending {
		value := string(task.Blocker.Condition.RelocationID)
		relocationID = &value
		resume := string(task.Blocker.ResumeNode)
		resumeNode = &resume
	}
	provisioningStatus := "last_known"
	if task.LastOperation != nil && task.LastOperation.Kind == domain.OperationAbandonTask {
		provisioningStatus = "unavailable"
	}
	return WorkspaceView{
		ProvisioningStatus:  provisioningStatus,
		CurrentChangedPaths: append([]string{}, task.CurrentChangedPaths...),
		HistoryConflict:     conflict || task.Blocker != nil && task.Blocker.Cause == domain.BlockerCauseWorkspaceHistoryConflict,
		Relocation:          RelocationView{Available: application.WorkspaceRelocationSupported(task), Pending: pending, RelocationID: relocationID, ResumeNode: resumeNode},
		Cleanup:             CleanupView{Automatic: false, HostActionRequired: hasWorktree, SeparateWorktreeAndBranch: hasWorktree, Terminal: task.CurrentNode.Terminal()},
	}
}

func projectChangedEntries(entries []domain.RepositoryChangedEntry) []ChangedEntryView {
	result := make([]ChangedEntryView, len(entries))
	for index, entry := range entries {
		result[index] = ChangedEntryView{Path: entry.Path, ChangeType: string(entry.ChangeType), FileMode: entry.FileMode, Gitlink: entry.Gitlink, ContentDigest: string(entry.ContentDigest)}
	}
	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func truncateWebSummary(value string) string {
	runes := []rune(value)
	if len(runes) <= 512 {
		return value
	}
	return string(runes[:511]) + "…"
}

func lifecycleFromNode(node domain.NodeID) string {
	switch node {
	case domain.NodeBlocked:
		return "blocked"
	case domain.NodeDone:
		return "done"
	case domain.NodeCancelled:
		return "cancelled"
	default:
		return "active"
	}
}

type namedFact struct {
	kind  string
	label string
	value any
}

func projectNamedFacts(items []namedFact) ([]Fact, error) {
	result := make([]Fact, 0, len(items))
	for _, item := range items {
		if isNilFact(item.value) {
			continue
		}
		fact, err := projectFact(item.kind, item.label, item.value)
		if err != nil {
			return nil, err
		}
		result = append(result, fact)
	}
	return result, nil
}

func isNilFact(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case *domain.RequirementsBaseline:
		return typed == nil
	case *domain.DesignBaseline:
		return typed == nil
	case *domain.TaskPlanBaseline:
		return typed == nil
	case *domain.ImplementationRecord:
		return typed == nil
	case *domain.TestRecord:
		return typed == nil
	case *domain.ComprehensionAssessment:
		return typed == nil
	case *domain.LastOperation:
		return typed == nil
	case []domain.FileScopeRecord:
		return len(typed) == 0
	default:
		return false
	}
}

func projectFact(kind, label string, value any) (Fact, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Fact{}, err
	}
	return Fact{Kind: kind, Label: label, Value: string(raw)}, nil
}

func projectGraph(graph workflow.ControlCenterGraph) GraphView {
	nodes := make([]GraphNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = GraphNode{NodeID: string(node.NodeID), Kind: node.Kind, Purpose: node.Purpose}
	}
	transitions := make([]GraphTransition, len(graph.Transitions))
	for index, transition := range graph.Transitions {
		transitions[index] = GraphTransition{TransitionID: string(transition.TransitionID), Source: string(transition.Source), Destination: string(transition.Destination)}
	}
	actual := make([]string, 0, len(graph.Traversals))
	for _, traversal := range graph.Traversals {
		if traversal.TransitionID != nil {
			actual = append(actual, string(*traversal.TransitionID))
		}
	}
	return GraphView{ProcessID: string(graph.Process.ID), DefinitionDigest: string(graph.Process.DefinitionDigest), CurrentNode: string(graph.CurrentNode), ResumeNode: optionalNode(graph.ResumeNode), Nodes: nodes, Transitions: transitions, ActualTransitionIDs: actual, CurrentLegalTransitionIDs: transitionStrings(graph.CurrentLegalTransitionIDs), FutureNodeIDs: nodeStrings(graph.FutureNodeIDs), FutureTransitionIDs: transitionStrings(graph.FutureTransitionIDs)}
}

func projectAction(action *domain.ProcessAction, blocker *domain.ProcessBlocker) (*ActionView, error) {
	if action == nil {
		return nil, nil
	}
	schema, err := workflow.CurrentSubmissionSchema(*action, blocker)
	if err != nil {
		return nil, err
	}
	payloadSchema, err := json.Marshal(schema)
	if err != nil {
		return nil, domain.ErrInternal
	}
	conditions := append([]string(nil), action.NodeContract.EntryConditions...)
	conditions = append(conditions, action.NodeContract.CompletionConditions...)
	effects := make([]string, len(action.AllowedEffects))
	for index, effect := range action.AllowedEffects {
		effects[index] = string(effect)
	}
	evidence := make([]string, len(action.RequiredEvidence))
	for index, requirement := range action.RequiredEvidence {
		evidence[index] = string(requirement.Kind)
	}
	steps := make([]string, len(action.SemanticMethodSteps))
	for index, step := range action.SemanticMethodSteps {
		steps[index] = step.Purpose
	}
	legal := make([]string, len(action.AvailableTransitions))
	for index, transition := range action.AvailableTransitions {
		legal[index] = string(transition.TransitionID)
	}
	return &ActionView{ActionID: string(action.ActionID), ActionKind: string(action.Kind), Purpose: action.NodeContract.Purpose, Conditions: conditions, AllowedEffects: effects, RequiredEvidence: evidence, MethodSteps: steps, LegalTransitionIDs: legal, PayloadSchema: payloadSchema}, nil
}

func writeReadError(w http.ResponseWriter, requestID string, err error) {
	status := http.StatusInternalServerError
	code := domain.ErrorInternal
	message := domain.ErrInternal.Message
	var typed *domain.Error
	if errors.As(err, &typed) {
		code, message = typed.Code, typed.Message
		switch typed.Code {
		case domain.ErrorInvalidArgument:
			status = http.StatusBadRequest
		case domain.ErrorTaskNotFound:
			status = http.StatusNotFound
		case domain.ErrorSchemaUnsupported, domain.ErrorProcessUnsupported:
			status = http.StatusConflict
		}
	}
	_ = WriteFailure(w, status, requestID, "not_committed", ErrorResponse{Code: string(code), Message: message, FieldPaths: domain.ViolationPaths(err)}, RecoveryAdvice{Action: RecoveryNone, RetrySafe: false, Message: "Refresh the current local WebUI state before continuing."})
}

func readRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "read-unavailable"
	}
	return "read-" + hex.EncodeToString(raw[:])
}

func optionalNode(value *domain.NodeID) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

func transitionStrings(values []domain.TransitionID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func nodeStrings(values []domain.NodeID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func actionIDText(id *domain.ID) *string {
	if id == nil {
		return nil
	}
	value := string(*id)
	return &value
}
