package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func runStatus(ctx context.Context, env platform.Env, dir string, output io.Writer) (resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, event.MCPBudget)
	defer cancel()
	var workspaceID string
	defer func() {
		if resultErr != nil {
			fmt.Fprintln(output, "TaskBelay 状态未完成："+resultErr.Error())
		}
		entries, err := recentErrors(env, workspaceID, time.Now())
		if err != nil {
			fmt.Fprintln(output, "最近日志无法读取："+err.Error())
			return
		}
		for _, entry := range entries {
			scope := "当前工作区"
			if entry.WorkspaceID == "" {
				scope = "本机未关联工作区"
			}
			fmt.Fprintf(output, "最近错误（%s，%s）：%s\n", scope, entry.Time.Format(time.RFC3339), entry.Error)
		}
	}()
	workspace, err := gitstate.Discover(ctx, dir)
	if err != nil {
		return err
	}
	workspaceID = workspace.ID
	store, err := ledger.Open(ctx, env.DataDir, ledger.NormalWait)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.RegisterGitWorkspace(ctx, workspace); err != nil {
		return err
	}
	service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: workspace}}
	maintain(ctx, service, env, ledger.NormalWait)
	status, err := service.Status(ctx)
	if err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintln(&text, status.Summary)
	fmt.Fprint(&text, check.FormatPlans(status.View.Plans))
	fmt.Fprintln(&text, "\n可恢复任务：")
	count := 0
	for _, candidate := range status.View.Candidates {
		if candidate.ID == status.View.CurrentTaskID || candidate.State == ledger.TaskCanceled {
			continue
		}
		fmt.Fprintf(&text, "%s %s；状态=%s；分支=%s\n", candidate.ID, strings.Join(strings.Fields(candidate.Title), " "), candidate.State, candidate.Context.Branch)
		count++
	}
	if count == 0 {
		fmt.Fprintln(&text, "暂无其他可恢复任务。")
	}
	fmt.Fprintln(&text, "\n有效设置：")
	var names []string
	for name := range status.View.Settings.Values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		setting := status.View.Settings.Values[name]
		fmt.Fprintf(&text, "%s = %s（%s", name, displayValue(name, setting.Value), scopeName(setting.Source))
		if setting.ScopeID != "" {
			fmt.Fprintf(&text, " %s", setting.ScopeID)
		}
		fmt.Fprintln(&text, "）")
	}
	if status.HookError != "" {
		fmt.Fprintln(&text, "\n最近记录的接入错误："+status.HookError)
	}
	for _, record := range status.Checks {
		fmt.Fprintln(&text, "\n"+check.Format(record, 0))
	}
	if unresolved := check.FormatIssues(status.View.Issues, 0); unresolved != "" {
		fmt.Fprintln(&text, "\n未解决问题：\n"+unresolved)
	}
	if pending, err := store.CleanupStatus(ctx, workspace.ID); err != nil {
		fmt.Fprintln(&text, "保留期清理状态未完成："+err.Error())
	} else if pending != "" {
		fmt.Fprintln(&text, "\n"+pending)
	}
	if len(status.View.Verifications) > 0 {
		fmt.Fprintln(&text, "\n验证记录：")
	}
	for _, record := range status.View.Verifications {
		state := record.State
		if record.Denied {
			state = "被拦截，未执行"
		}
		fmt.Fprintf(&text, "%s %s；%s；目录与参数=%v\n", record.ID, state, record.Command.Display, record.Command.Segments)
		fmt.Fprintln(&text, check.FormatVerification(record))
	}
	_, err = io.WriteString(output, text.String())
	return err
}
