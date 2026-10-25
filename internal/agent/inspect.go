package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/skills"
)

type Installation struct {
	Command    string
	Hooks      int
	Skills     int
	MCP        bool
	ForeignMCP bool
}

func Inspect(ctx context.Context, env platform.Env, name string, run Runner) (result Installation, resultErr error) {
	config, skillPath, err := installPaths(env, name)
	if err != nil {
		return result, err
	}
	result.Command, _ = exec.LookPath(name)
	if run == nil {
		if err := checkConfigHome(env, name); err != nil {
			return result, err
		}
		run = runCommand
	}
	data, err := os.ReadFile(config)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	if len(data) > 0 {
		var root struct {
			Hooks map[string][]struct {
				Hooks []json.RawMessage `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(data, &root); err != nil {
			return result, fmt.Errorf("读取 %s: %w", config, err)
		}
		for _, groups := range root.Hooks {
			for _, group := range groups {
				for _, data := range group.Hooks {
					object, err := hookjson.ParseObject(data)
					if err != nil {
						return result, err
					}
					if ownedHook(name)(object) {
						result.Hooks++
					}
				}
			}
		}
	}
	for _, skill := range skills.Names {
		data, err := os.ReadFile(filepath.Join(skillPath, skill, ".taskbelay"))
		if err != nil && !os.IsNotExist(err) {
			return result, err
		}
		if string(data) == skillMarker {
			result.Skills++
		}
	}
	// 没有用户配置时无需启动 Agent，首次安装前也能查看状态和卸载程序。
	mcpConfig := filepath.Join(env.HomeDir, ".claude.json")
	if name == "codex" {
		mcpConfig = filepath.Join(env.HomeDir, ".codex", "config.toml")
	}
	if _, err := os.Stat(mcpConfig); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	exists, owned, err := mcpOwned(ctx, env, name, run)
	result.MCP, result.ForeignMCP = exists && owned, exists && !owned
	return result, err
}
