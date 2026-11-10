package check

import (
	"context"
	"fmt"
	"runtime/debug"
	"runtime/metrics"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type Background struct {
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	wake        chan struct{}
	mu          sync.Mutex
	actor       ledger.Actor
	store       *ledger.Store
	workspace   gitstate.Workspace
	report      func(error)
	lastRelease uint64 // 只由后台循环访问
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
	// Agent 尚未发起有身份的工具调用时，没有可轮询的任务。
	select {
	case <-b.ctx.Done():
		return
	case <-b.wake:
	}
	timer := time.NewTicker(BackgroundPoll)
	defer timer.Stop()
	b.prepare(true)
	for {
		b.mu.Lock()
		actor := b.actor
		b.mu.Unlock()
		if err := b.run(actor); err != nil && b.ctx.Err() == nil && b.report != nil {
			b.report(err)
		}
		select {
		case <-b.ctx.Done():
			return
		case <-b.wake:
			b.prepare(false)
		case <-timer.C:
		}
	}
}

func (b *Background) prepare(first bool) {
	b.warm()
	if first {
		b.prune()
	}
	// 建索引和检查可在短时间分配数 GiB，空闲的 stdio 进程随后可能很久不再触发 GC。
	// 同时核对堆占用和上次回收以来的分配量，不让轻量调用反复触发全堆回收。
	const releaseBytes = 128 << 20
	stats := [...]metrics.Sample{{Name: "/gc/heap/allocs:bytes"}, {Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/heap/free:bytes"}}
	metrics.Read(stats[:])
	allocated := stats[0].Value.Uint64()
	if b.ctx.Err() == nil && allocated-b.lastRelease >= releaseBytes && stats[1].Value.Uint64()+stats[2].Value.Uint64() >= releaseBytes {
		debug.FreeOSMemory()
		b.lastRelease = allocated
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
		ctx = context.WithValue(ctx, backgroundIndexKey{}, true)
		_, err = taskIndex(ctx, b.store, b.workspace, task)
	}
	if err != nil && b.ctx.Err() == nil && b.report != nil {
		b.report(err)
	}
}

/**
 * prune 在每个 MCP 进程第一次空闲时清理一次分析数据。清理和收缩账本文件不占用 hook 的时间。
 */
func (b *Background) prune() {
	ctx, cancel := context.WithTimeout(b.ctx, event.StaticBudget+event.TestsBudget)
	defer cancel()
	if err := Prune(ctx, b.store); err != nil && b.ctx.Err() == nil && b.report != nil {
		b.report(fmt.Errorf("分析数据清理未完成: %w", err))
	}
}

func (b *Background) run(actor ledger.Actor) error {
	ctx, cancel := context.WithTimeout(b.ctx, event.StaticBudget+event.TestsBudget)
	defer cancel()
	enabled, err := b.store.DynamicTestsEnabled(ctx, b.workspace.ID)
	if err != nil || !enabled {
		return err
	}
	task, observation, err := b.store.CommandContext(ctx, b.workspace.ID)
	if err != nil || task.ID == "" || task.State != ledger.TaskActive {
		return err
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
	view, err := b.store.TaskView(ctx, b.workspace.ID, task.ID, actor)
	if err != nil {
		return err
	}
	if view.CurrentTaskID != task.ID || view.Settings.Values["analysis.tests"].Value != "on" || view.Task.State != ledger.TaskActive {
		return nil
	}
	_, err = Run(ctx, b.store, b.workspace, actor, view, observation, RunInput{Depth: "tests"})
	return err
}
