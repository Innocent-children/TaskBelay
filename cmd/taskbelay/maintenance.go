package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func maintain(ctx context.Context, service hook.Tasks, env platform.Env) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ledger.NormalWait)
	defer cancel()
	if err := pruneLogs(ctx, env, time.Now()); err != nil {
		_ = logWorkspaceError(env, service.Workspace.ID, fmt.Errorf("日志清理未完成: %w", err))
	}
	if err := service.Maintain(ctx, time.Now()); err != nil {
		_ = logWorkspaceError(env, service.Workspace.ID, fmt.Errorf("保留期清理未完成: %w", err))
	}
}
