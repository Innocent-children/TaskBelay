// Package agent 定义 Agent 适配器的接口，并列出已有的适配器。每个 Agent 的特殊之处
// 都在自己的适配器里，其余代码只和统一的事件、决定打交道。
package agent

import (
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// Adapter 是一个 Agent 的适配器。
type Adapter interface {
	// Name 返回 Agent 的名字，也是 hook 命令里的 <agent>。
	Name() string
	// ParseEvent 把一次 hook 调用的输入转成统一事件。hookEvent 是 hook 命令里的事件名。
	ParseEvent(hookEvent string, input []byte) (event.Event, error)
	// RenderDecision 把决定写成这个 Agent 的 hook 输出，决定为空时返回 nil。
	RenderDecision(hookEvent string, d event.Decision) ([]byte, error)
	// Install 接入这个 Agent，返回要告诉用户的话。exe 是 taskbelay 程序的绝对路径。
	Install(env platform.Env, exe string) (string, error)
	// Uninstall 移除接入，返回要告诉用户的话。
	Uninstall(env platform.Env) (string, error)
}

// adapters 是已有的适配器。接入新的 Agent 时加在这里。
var adapters = []Adapter{claude.Adapter{}, codex.Adapter{}}

// Lookup 按名字找适配器。
func Lookup(name string) (Adapter, bool) {
	for _, a := range adapters {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}

// Names 返回所有适配器的名字。
func Names() []string {
	var names []string
	for _, a := range adapters {
		names = append(names, a.Name())
	}
	return names
}
