package agent

import (
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type Adapter interface {
	Name() string
	ParseEvent(hookEvent string, input []byte) (event.Event, error)
	RenderDecision(hookEvent string, d event.Decision) ([]byte, error)
	Install(env platform.Env, exe string) (string, error)
	Uninstall(env platform.Env) (string, error)
}

type Registry map[string]Adapter

func (r Registry) Find(name string) (Adapter, error) {
	adapter, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("不支持 Agent %q；可使用 claude 或 codex", name)
	}
	return adapter, nil
}
