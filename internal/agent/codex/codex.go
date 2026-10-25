package codex

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type Adapter struct{ Runner agent.Runner }

func (Adapter) Name() string { return "codex" }
func (a Adapter) Install(env platform.Env, exe string) (string, error) {
	return agent.Install(env, exe, a.Name(), a.Runner)
}
func (a Adapter) Uninstall(env platform.Env) (string, error) {
	return agent.Uninstall(env, a.Name(), a.Runner)
}
func (Adapter) RenderDecision(hook string, d event.Decision) ([]byte, error) {
	return hookjson.Render(hook, d, false)
}

func (Adapter) ParseEvent(hook string, data []byte) (event.Event, error) {
	if hook == "post-tool-use-failure" {
		return event.Event{}, fmt.Errorf("Codex 不提供 PostToolUseFailure")
	}
	in, err := hookjson.Read(hook, data)
	if err != nil {
		return event.Event{}, err
	}
	e, err := hookjson.Base(in, "codex", in.TurnID)
	if err != nil || e.Kind != "" {
		return e, err
	}
	var tool struct {
		Command string `json:"command"`
		Cmd     string `json:"cmd"`
		Workdir string `json:"workdir"`
		Shell   string `json:"shell"`
	}
	if err := json.Unmarshal(in.ToolInput, &tool); err != nil {
		return e, fmt.Errorf("读取 Codex 工具参数: %w", err)
	}
	if tool.Workdir != "" {
		e.WorkingDir = tool.Workdir
		if !filepath.IsAbs(e.WorkingDir) {
			e.WorkingDir = filepath.Join(in.CWD, e.WorkingDir)
		}
	}
	switch in.Tool {
	case "Bash", "exec_command":
		e.Kind = event.CommandUse
		command := tool.Command
		if command == "" {
			command = tool.Cmd
		}
		if command == "" {
			return e, fmt.Errorf("命令为空")
		}
		shell := platform.CodexShell()
		if tool.Shell != "" {
			base := strings.ToLower(filepath.Base(tool.Shell))
			switch base {
			case "bash", "sh", "zsh", "bash.exe", "sh.exe", "zsh.exe":
				shell = event.POSIX
			case "pwsh", "powershell", "pwsh.exe", "powershell.exe":
				shell = event.PowerShell
			default:
				shell = ""
			}
		}
		e.Command = &event.Command{Text: command, Shell: shell}
	case "apply_patch":
		e.Kind = event.EditUse
		e.Edits, err = parsePatch(tool.Command)
		if err != nil {
			return e, err
		}
	default:
		return e, fmt.Errorf("未接入 Codex 工具 %q", in.Tool)
	}
	if hook == "post-tool-use" {
		e.Kind = event.ToolEnd
		e.Result = &event.ToolResult{State: event.Unknown, Output: string(in.Response)}
		if e.Command != nil {
			e.Result = commandResult(in.Response)
		}
		e.Result.Edits = e.Edits
	}
	return e, nil
}
