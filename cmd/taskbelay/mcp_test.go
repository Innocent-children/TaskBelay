package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpProcess(t *testing.T, dir, dataDir string) *mcp.ClientSession {
	t.Helper()
	processContext, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 20*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(processContext, os.Args[0], "-test.run=^TestMCPProcessHelper$")
	command.Env = append(os.Environ(), "TASKBELAY_MCP_TEST_DIR="+dir, "TASKBELAY_MCP_TEST_DATA="+dataDir)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "test"}, nil)
	session, err := client.Connect(processContext, &mcp.IOTransport{Reader: output, Writer: input}, nil)
	if err != nil {
		command.Process.Kill()
		command.Wait()
		t.Fatal(err, stderr.String())
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
		if err := command.Wait(); err != nil {
			t.Errorf("stdio server: %v %s", err, stderr.String())
		}
	})
	return session
}

func TestMCPProcessHelper(t *testing.T) {
	dir := os.Getenv("TASKBELAY_MCP_TEST_DIR")
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := run(ctx, []string{"mcp"}, platform.Env{DataDir: os.Getenv("TASKBELAY_MCP_TEST_DATA")}, dir, os.Stdout)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func protocolCall(t *testing.T, session *mcp.ClientSession, actor ledger.Actor, name string, input any, wantError bool) map[string]json.RawMessage {
	t.Helper()
	params := &mcp.CallToolParams{Name: name, Arguments: input}
	if actor.Agent != "" {
		params.Meta = mcp.Meta{"taskbelay/context": actor}
	}
	result, err := session.CallTool(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError != wantError {
		t.Fatalf("%s error=%t, result=%+v", name, result.IsError, result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("structured result %s: %v", data, err)
	}
	return decoded
}

func TestMCPStdioToolsContextConflictAndRetry(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "-C", dir, "init", "--quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	w, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ledger.Open(t.Context(), dataDir, ledger.NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	service := hook.Tasks{Observer: hook.Observer{Store: s, Workspace: w}}
	message, err := service.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: "codex", SessionID: "stdio-session", RoundID: "round", MessageID: "prompt", Message: "实现功能，可以保留"})
	if err != nil {
		t.Fatal(err)
	}
	session := mcpProcess(t, dir, dataDir)
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "check_run,note_add,plan_confirm,plan_save,requirement_save,task_finish,task_get,task_start" {
		t.Fatalf("unexpected tools: %v", names)
	}
	protocolCall(t, session, ledger.Actor{}, "task_get", map[string]any{}, false)
	missing := protocolCall(t, session, ledger.Actor{}, "task_start", map[string]any{"title": "task"}, true)
	if !bytes.Contains(missing["error"], []byte("taskbelay/context")) {
		t.Fatalf("missing provenance not explained: %s", missing["error"])
	}
	a := ledger.Actor{Agent: "codex", SessionID: "stdio-session", RoundID: "round", CallID: "start"}
	started := protocolCall(t, session, a, "task_start", map[string]any{"title": "task"}, false)
	var handoff string
	if err := json.Unmarshal(started["summary"], &handoff); err != nil || !strings.Contains(handoff, "task_get") || !strings.Contains(handoff, "task") {
		t.Fatal("task_start omitted handoff", handoff, err)
	}
	var task ledger.TaskRecord
	if err := json.Unmarshal(started["result"], &task); err != nil {
		t.Fatal(err)
	}
	duplicate := protocolCall(t, session, a, "task_start", map[string]any{"title": "task"}, false)
	if !bytes.Equal(started["result"], duplicate["result"]) {
		t.Fatal("duplicate start changed task")
	}
	a.CallID = "save"
	request := map[string]any{"content": "功能", "acceptance": "输出正确", "source_id": message.ID, "quote": "实现功能", "implemented_by": []any{map[string]any{"at": "flag:--mode", "purpose": "选择功能"}}}
	saved := protocolCall(t, session, a, "requirement_save", request, false)
	var requirement ledger.Requirement
	if err := json.Unmarshal(saved["result"], &requirement); err != nil {
		t.Fatal(err)
	}
	if requirement.ID != "R1" || requirement.TaskID != task.ID {
		t.Fatal("requirement target lost")
	}
	a.CallID = "update"
	update := map[string]any{"id": requirement.ID, "content": "功能", "acceptance": "新增验收", "base_revision": requirement.Revision}
	protocolCall(t, session, a, "requirement_save", update, false)
	a.CallID = "stale"
	conflict := protocolCall(t, session, a, "requirement_save", update, true)
	var latest ledger.Requirement
	if err := json.Unmarshal(conflict["latest"], &latest); err != nil || latest.Revision != 2 {
		t.Fatalf("conflict missing latest: %s %v", conflict["latest"], err)
	}
	a.CallID = "note"
	protocolCall(t, session, a, "note_add", map[string]any{"kind": "next_step", "text": "核对当前代码"}, false)
	viewData := protocolCall(t, session, a, "task_get", map[string]any{}, false)
	var view ledger.TaskView
	if err := json.Unmarshal(mustJSON(t, viewData), &view); err != nil {
		t.Fatal(err)
	}
	if view.Task.ID != task.ID || len(view.History) != 2 || len(view.Sources) == 0 || len(view.PendingLocations) != 1 || view.LatestCheck != "not_checked" || view.Settings.Revision == "" {
		t.Fatalf("incomplete task_get: %+v", view)
	}
	a.CallID = "check"
	checked := protocolCall(t, session, a, "check_run", map[string]any{"depth": "static"}, false)
	var record ledger.CheckRecord
	if err := json.Unmarshal(mustJSON(t, checked), &record); err != nil || record.InputVersion == "" || record.Status != ledger.CheckComplete {
		t.Fatal("check_run did not return fixed input", record, err)
	}
	a.CallID = "check-tests"
	tested := protocolCall(t, session, a, "check_run", map[string]any{"depth": "tests"}, false)
	var dynamic ledger.CheckRecord
	if err := json.Unmarshal(mustJSON(t, tested), &dynamic); err != nil || dynamic.Depth != "tests" || dynamic.Static == nil || dynamic.Static.ID != record.ID || dynamic.Report.Dynamic == nil || dynamic.Input.AnalysisSettings["analysis.tests"] != "off" {
		t.Fatal("dynamic MCP response lost stages or changed setting", dynamic, err)
	}
	var commandOutput bytes.Buffer
	if err := runCheck(t.Context(), []string{"--tests"}, platform.Env{DataDir: dataDir}, dir, &commandOutput); err != nil || !strings.Contains(commandOutput.String(), "tests 检查") || !strings.Contains(commandOutput.String(), "static 检查") {
		t.Fatal("dynamic CLI response lost stages", commandOutput.String(), err)
	}
	a.CallID = "finish"
	finished := protocolCall(t, session, a, "task_finish", map[string]any{}, false)
	var finish ledger.FinishResult
	if err := json.Unmarshal(mustJSON(t, finished), &finish); err != nil || finish.Task.State != ledger.TaskPending || finish.Static == nil || finish.Static.ID != record.ID {
		t.Fatal("task_finish did not return saved check", finish, err)
	}
	retried := protocolCall(t, session, a, "task_finish", map[string]any{}, false)
	if !bytes.Equal(mustJSON(t, finished), mustJSON(t, retried)) {
		t.Fatal("finish retry changed saved request")
	}
	nextMessage, err := service.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: a.Agent, SessionID: a.SessionID, RoundID: "new-round", MessageID: "new-prompt", Message: "方案按 A 做，直接改"})
	if err != nil {
		t.Fatal(err)
	}
	a.CallID = "start"
	protocolCall(t, session, a, "task_start", map[string]any{"title": "task"}, false)
	a.CallID = "old-round-write"
	protocolCall(t, session, a, "note_add", map[string]any{"kind": "next_step", "text": "过期轮次"}, true)
	a.RoundID, a.CallID = "new-round", "semi-start"
	protocolCall(t, session, a, "task_start", map[string]any{"task_id": task.ID, "workflow": "semi"}, false)
	a.CallID = "plan-save"
	planInput := ledger.PlanInput{Content: "原因：补充功能。成功标准：输出正确。做法：沿用 A。步骤：实现并验证。", Files: []ledger.PlanFile{{Path: "main.go", Change: "modify", Note: "实现本次功能"}}}
	savedPlan := protocolCall(t, session, a, "plan_save", planInput, false)
	var planResult ledger.PlanResult
	if err := json.Unmarshal(mustJSON(t, savedPlan), &planResult); err != nil || planResult.Plan == nil || planResult.Plan.Version != 1 {
		t.Fatal(planResult, err)
	}
	a.CallID = "plan-confirm"
	approved := protocolCall(t, session, a, "plan_confirm", ledger.PlanConfirmInput{Version: 1, SourceID: nextMessage.ID, Quote: nextMessage.Text}, false)
	if err := json.Unmarshal(mustJSON(t, approved), &planResult); err != nil || planResult.View.Effective == nil {
		t.Fatal(planResult, err)
	}
	a.CallID = "stale-plan"
	stalePlan := protocolCall(t, session, a, "plan_save", planInput, true)
	var latestPlans ledger.PlanView
	if err := json.Unmarshal(stalePlan["latest"], &latestPlans); err != nil || latestPlans.Revision != 2 {
		t.Fatal(latestPlans, err)
	}
	var status bytes.Buffer
	if err := runStatus(t.Context(), platform.Env{DataDir: dataDir}, dir, &status); err != nil || !strings.Contains(status.String(), planResult.View.Effective.Path) {
		t.Fatal(status.String(), err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMCPNonRepositoryReturnsToolError(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "not-created")
	session := mcpProcess(t, t.TempDir(), dataDir)
	result := protocolCall(t, session, ledger.Actor{}, "task_get", map[string]any{}, true)
	if !bytes.Contains(result["error"], []byte("git")) {
		t.Fatalf("not-workspace result: %s", result["error"])
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatal("non-repository MCP created ledger")
	}
}

func TestCodexNativeMCPMetadataMatchesRecordedHookRound(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	var output bytes.Buffer
	data := cliHookInput(t, dir, "UserPromptSubmit", map[string]any{"prompt": "实现当前功能。"})
	if err := runHook(t.Context(), []string{"codex", "user-prompt-submit"}, platform.Env{DataDir: dataDir}, bytes.NewReader(data), &output); err != nil {
		t.Fatal(err)
	}
	session := mcpProcess(t, dir, dataDir)
	meta := mcp.Meta{"sessionId": "s", "threadId": "not-session", "callId": "native-call", "x-codex-turn-metadata": map[string]any{"session_id": "s", "turn_id": "t", "thread_id": "not-session", "codex_version": "0.163.0"}}
	params := &mcp.CallToolParams{Name: "task_start", Arguments: map[string]any{"title": "当前功能"}, Meta: meta}
	first, err := session.CallTool(t.Context(), params)
	if err != nil || first.IsError {
		t.Fatal(first, err)
	}
	second, err := session.CallTool(t.Context(), params)
	if err != nil || second.IsError {
		t.Fatal(second, err)
	}
	var a, b map[string]json.RawMessage
	if err := json.Unmarshal(mustJSON(t, first.StructuredContent), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustJSON(t, second.StructuredContent), &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a["result"], b["result"]) {
		t.Fatal("native retry changed task")
	}
	meta["callId"] = "old-round-call"
	meta["x-codex-turn-metadata"] = map[string]any{"session_id": "s", "turn_id": "older"}
	params.Name, params.Arguments = "note_add", map[string]any{"kind": "next_step", "text": "过期轮次"}
	result, err := session.CallTool(t.Context(), params)
	if err != nil || !result.IsError {
		t.Fatal("stale native identity accepted", result, err)
	}
}

func TestCapturedClaudeMCPMetadataCannotInventRound(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "eval", "cases", "agents", "claude-2.1.309-mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Call struct {
			Params struct {
				Meta mcp.Meta `json:"_meta"`
			} `json:"params"`
		} `json:"call"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	actor, err := agentContext(hook.Tasks{}, "")(t.Context(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: fixture.Call.Params.Meta}})
	if err == nil || actor.Agent != "" || !strings.Contains(err.Error(), "CLAUDE_CODE_SESSION_ID") {
		t.Fatal(actor, err)
	}
}

func TestClaudeMCPReadsExactPreHookIdentityAndReplaysOldCalls(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "s")
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	env := platform.Env{DataDir: dataDir}
	var output bytes.Buffer
	prompt := cliHookInput(t, dir, "UserPromptSubmit", map[string]any{"prompt": "实现功能。"})
	if err := runHook(t.Context(), []string{"claude", "user-prompt-submit"}, env, bytes.NewReader(prompt), &output); err != nil {
		t.Fatal(err)
	}
	session := mcpProcess(t, dir, dataDir)
	params := &mcp.CallToolParams{Name: "task_start", Arguments: map[string]any{"title": "功能"}, Meta: mcp.Meta{"claudecode/toolUseId": "native-call", "progressToken": 2}}
	missing, err := session.CallTool(t.Context(), params)
	if err != nil || !missing.IsError || !bytes.Contains(mustJSON(t, missing.StructuredContent), []byte("前置身份")) {
		t.Fatal(missing, err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var before, after int
	if err := db.QueryRow("SELECT count(*) FROM observations").Scan(&before); err != nil {
		t.Fatal(err)
	}
	pre := cliHookInput(t, dir, "PreToolUse", map[string]any{"tool_name": "mcp__taskbelay__task_start", "tool_use_id": "native-call", "tool_input": map[string]any{"title": "功能"}})
	output.Reset()
	if err := runHook(t.Context(), []string{"claude", "pre-tool-use"}, env, bytes.NewReader(pre), &output); err != nil || output.Len() != 0 {
		t.Fatal(output.String(), err)
	}
	if err := db.QueryRow("SELECT count(*) FROM observations").Scan(&after); err != nil || after != before {
		t.Fatal("MCP identity hook captured a snapshot", before, after, err)
	}
	var raw string
	if err := db.QueryRow("SELECT raw_input FROM hook_events WHERE kind='mcp_use'").Scan(&raw); err != nil || raw != "" {
		t.Fatal("MCP identity hook retained business input", raw, err)
	}
	first, err := session.CallTool(t.Context(), params)
	if err != nil || first.IsError {
		t.Fatal(first, err)
	}
	newPrompt := cliHookInput(t, dir, "UserPromptSubmit", map[string]any{"prompt_id": "new-prompt", "prompt": "补充当前需求。"})
	if err := runHook(t.Context(), []string{"claude", "user-prompt-submit"}, env, bytes.NewReader(newPrompt), &output); err != nil {
		t.Fatal(err)
	}
	retry, err := session.CallTool(t.Context(), params)
	if err != nil || retry.IsError {
		t.Fatal(retry, err)
	}
	var a, b map[string]json.RawMessage
	if err := json.Unmarshal(mustJSON(t, first.StructuredContent), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustJSON(t, retry.StructuredContent), &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a["result"], b["result"]) {
		t.Fatal("retry rebound to new round")
	}
	params.Name, params.Arguments = "note_add", map[string]any{"kind": "next_step", "text": "不能借用另一工具的身份"}
	wrong, err := session.CallTool(t.Context(), params)
	if err != nil || !wrong.IsError {
		t.Fatal("tool mismatch accepted", wrong, err)
	}
	params.Meta = mcp.Meta{"claudecode/toolUseId": "late-call"}
	pre = cliHookInput(t, dir, "PreToolUse", map[string]any{"tool_name": "mcp__taskbelay__note_add", "tool_use_id": "late-call", "tool_input": params.Arguments})
	if err := runHook(t.Context(), []string{"claude", "pre-tool-use"}, env, bytes.NewReader(pre), &output); err != nil {
		t.Fatal(err)
	}
	late, err := session.CallTool(t.Context(), params)
	if err != nil || !late.IsError {
		t.Fatal("old round newly wrote", late, err)
	}
}
