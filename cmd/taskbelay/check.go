package main

import (
	"context"
	"fmt"
	"io"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func runCheck(ctx context.Context, args []string, env platform.Env, dir string, output io.Writer) error {
	options := check.RunInput{Depth: "static"}
	for _, arg := range args {
		switch {
		case arg == "--refresh" && !options.Refresh:
			options.Refresh = true
		case arg == "--tests" && options.Depth != "tests":
			options.Depth = "tests"
		default:
			return fmt.Errorf("用法: taskbelay check [--tests] [--refresh]")
		}
	}
	budget := event.StaticBudget
	if options.Depth == "tests" {
		budget += event.TestsBudget
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	workspace, err := gitstate.Discover(ctx, dir)
	if err != nil {
		return err
	}
	store, err := ledger.Open(ctx, env.DataDir, ledger.NormalWait)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.RegisterGitWorkspace(ctx, workspace); err != nil {
		return err
	}
	result, err := (hook.Tasks{Observer: hook.Observer{Store: store, Workspace: workspace}, ReportError: func(cause error) { _ = logWorkspaceError(env, workspace.ID, cause) }}).Check(ctx, ledger.Actor{}, options)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, check.Format(result, 5))
	return err
}
