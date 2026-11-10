package check

import (
	"context"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type Background struct {
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	wake      chan struct{}
	mu        sync.Mutex
	actor     ledger.Actor
	store     *ledger.Store
	workspace gitstate.Workspace
	report    func(error)
}

func NewBackground(ctx context.Context, s *ledger.Store, w gitstate.Workspace, report func(error)) *Background {
	ctx, cancel := context.WithCancel(ctx)
	b := &Background{ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), store: s, workspace: w, report: report}
	go b.loop()
	return b
}
func (b *Background) Close() { b.cancel(); <-b.done }
func (b *Background) Schedule(actor ledger.Actor) {
	if b == nil || actor.Agent == "" || actor.SessionID == "" {
		return
	}
	b.mu.Lock()
	b.actor = actor
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}
func (b *Background) loop() {
	defer close(b.done)
	timer := time.NewTicker(BackgroundPoll)
	defer timer.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.wake:
			b.warm()
		case <-timer.C:
		}
		b.mu.Lock()
		actor := b.actor
		b.mu.Unlock()
		if actor.Agent == "" {
			continue
		}
		if err := b.run(actor); err != nil && b.ctx.Err() == nil && b.report != nil {
			b.report(err)
		}
	}
}

/**
 * warm 在 MCP 调用后为当前任务准备分析索引。从头学习起点耗时超过 hook 的预算，
 * 只在这里和交付检查中进行；hook 使用已发布的结果。
 */
func (b *Background) warm() {
	ctx, cancel := context.WithTimeout(b.ctx, event.StaticBudget+event.TestsBudget)
	defer cancel()
	task, _, err := b.store.CommandContext(ctx, b.workspace.ID)
	if err == nil && task.ID != "" {
		_, err = taskIndex(ctx, b.store, b.workspace, task)
	}
	if err != nil && b.ctx.Err() == nil && b.report != nil {
		b.report(err)
	}
}

func (b *Background) run(actor ledger.Actor) error {
	ctx, cancel := context.WithTimeout(b.ctx, event.StaticBudget+event.TestsBudget)
	defer cancel()
	task, observation, err := b.store.CommandContext(ctx, b.workspace.ID)
	if err != nil || task.ID == "" {
		return err
	}
	settings, err := b.store.Settings(ctx, b.workspace.ID)
	if err != nil {
		return err
	}
	if settings.Values["analysis.tests"].Value != "on" {
		return nil
	}
	view, err := b.store.TaskView(ctx, b.workspace.ID, task.ID, actor)
	if err != nil {
		return err
	}
	if view.Settings.Values["analysis.tests"].Value != "on" || view.Task.State != ledger.TaskActive {
		return nil
	}
	input, err := b.store.CheckInput(ctx, b.workspace.ID, task.ID, actor, Algorithm)
	if err != nil {
		return err
	}
	var saved struct{ State string }
	if found, err := b.store.AnalysisRecord(ctx, task.ID, input.Version(), "dynamic", &saved); err != nil {
		return err
	} else if found && saved.State != "running" {
		return nil
	}
	_, err = Run(ctx, b.store, b.workspace, actor, view, observation, RunInput{Depth: "tests"})
	return err
}
