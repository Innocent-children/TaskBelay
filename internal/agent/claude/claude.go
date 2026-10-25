package claude

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type Adapter struct{ Runner agent.Runner }

func (Adapter) Name() string { return "claude" }
func (a Adapter) Install(env platform.Env, exe string) (string, error) {
	return agent.Install(env, exe, a.Name(), a.Runner)
}
func (a Adapter) Uninstall(env platform.Env) (string, error) {
	return agent.Uninstall(env, a.Name(), a.Runner)
}
func (Adapter) RenderDecision(hook string, d event.Decision) ([]byte, error) {
	return hookjson.Render(hook, d, true)
}

func (Adapter) ParseEvent(hook string, data []byte) (event.Event, error) {
	in, err := hookjson.Read(hook, data)
	if err != nil {
		return event.Event{}, err
	}
	e, err := hookjson.Base(in, "claude", in.PromptID)
	if err != nil || e.Kind != "" {
		return e, err
	}
	if hook == "pre-tool-use" && strings.HasPrefix(in.Tool, "mcp__taskbelay__") {
		e.Kind, e.MCPTool = event.MCPUse, strings.TrimPrefix(in.Tool, "mcp__taskbelay__")
		if e.MCPTool == "" || e.RoundID == "" {
			return e, fmt.Errorf("MCP 前置身份缺少工具名或 prompt_id")
		}
		return e, nil
	}
	var tool struct {
		Command    string  `json:"command"`
		Background bool    `json:"run_in_background"`
		Path       string  `json:"file_path"`
		Old        *string `json:"old_string"`
		New        *string `json:"new_string"`
		Content    *string `json:"content"`
		ReplaceAll bool    `json:"replace_all"`
		Notebook   string  `json:"notebook_path"`
		Source     *string `json:"new_source"`
	}
	if err := json.Unmarshal(in.ToolInput, &tool); err != nil {
		return e, fmt.Errorf("读取 Claude Code 工具参数: %w", err)
	}
	switch in.Tool {
	case "Bash", "PowerShell":
		e.Kind = event.CommandUse
		shell := event.POSIX
		if in.Tool == "PowerShell" {
			shell = event.PowerShell
		}
		if tool.Command == "" {
			return e, fmt.Errorf("命令为空")
		}
		e.Command = &event.Command{Text: tool.Command, Shell: shell, Background: tool.Background}
	case "Edit":
		if tool.Path == "" || tool.Old == nil || tool.New == nil {
			return e, fmt.Errorf("Edit 缺少路径或替换原文")
		}
		e.Edits = []event.FileEdit{{Path: tool.Path, Before: tool.Old, After: tool.New, ReplaceAll: tool.ReplaceAll}}
	case "Write":
		if tool.Path == "" || tool.Content == nil {
			return e, fmt.Errorf("Write 缺少路径或写入内容")
		}
		e.Edits = []event.FileEdit{{Path: tool.Path, After: tool.Content, WholeFile: true}}
	case "NotebookEdit":
		if tool.Notebook == "" {
			return e, fmt.Errorf("NotebookEdit 缺少 notebook_path")
		}
		e.Edits = []event.FileEdit{{Path: tool.Notebook, After: tool.Source, Unverifiable: "NotebookEdit 未提供可核对的单元格前置内容；实际变化保留为工作区更新"}}
	default:
		return e, fmt.Errorf("未接入 Claude Code 工具 %q", in.Tool)
	}
	if len(e.Edits) > 0 {
		e.Kind = event.EditUse
	}
	if hook != "pre-tool-use" {
		e.Kind = event.ToolEnd
		e.Result = result(in, tool.Background)
		e.Result.Edits = e.Edits
	}
	return e, nil
}

var exitLine = regexp.MustCompile(`^Exit code (-?[0-9]+)(?:\r?\n|$)`)

func result(in hookjson.Input, background bool) *event.ToolResult {
	r := &event.ToolResult{State: event.Unknown, Output: string(in.Response)}
	var response struct {
		ExitCode    *int    `json:"exit_code"`
		Interrupted *bool   `json:"interrupted"`
		Background  *string `json:"backgroundTaskId"`
		Stdout      *string `json:"stdout"`
	}
	responseErr := json.Unmarshal(in.Response, &response)
	if in.Event == "PostToolUseFailure" {
		r.Output, r.State = in.Error, event.Failed
		if in.Interrupted {
			r.State = event.Interrupted
			return r
		}
		if in.Tool == "Bash" || in.Tool == "PowerShell" {
			if match := exitLine.FindStringSubmatch(in.Error); match != nil {
				if code, err := strconv.Atoi(match[1]); err == nil {
					r.ExitCode = &code
				}
			}
		}
		return r
	}
	if responseErr != nil {
		if background {
			r.State = event.Running
		}
		return r
	}
	if response.Interrupted != nil && *response.Interrupted {
		r.State = event.Interrupted
		return r
	}
	if response.Background != nil && strings.TrimSpace(*response.Background) != "" || background && response.ExitCode == nil {
		r.State = event.Running
		return r
	}
	r.State = event.Completed
	if in.Tool == "Bash" || in.Tool == "PowerShell" {
		if response.ExitCode == nil && (response.Interrupted == nil || response.Stdout == nil) {
			r.State = event.Unknown
			return r
		}
		// 成功事件本身能证明前台工具成功；后台启动不属于已完成的命令。
		code := 0
		r.ExitCode = &code
		if response.ExitCode != nil {
			r.ExitCode = response.ExitCode
			if *r.ExitCode != 0 {
				r.State = event.Failed
			}
		}
	}
	return r
}
