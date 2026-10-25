package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func maintain(ctx context.Context, service hook.Tasks, env platform.Env, budget time.Duration) {
	if ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if err := pruneLogs(ctx, env, time.Now()); err != nil {
		_ = logWorkspaceError(env, service.Workspace.ID, fmt.Errorf("日志清理未完成: %w", err))
	}
	if err := service.Maintain(ctx, time.Now()); err != nil {
		_ = logWorkspaceError(env, service.Workspace.ID, fmt.Errorf("保留期清理未完成: %w", err))
	}
}
