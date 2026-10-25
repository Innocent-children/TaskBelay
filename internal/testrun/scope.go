package testrun

import (
	"context"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

type VerificationScope struct {
	InputID          string   `json:"input_id"`
	ToolID           string   `json:"tool_id"`
	EnvironmentID    string   `json:"environment_id"`
	Complete         bool     `json:"complete"`
	EnvironmentKnown bool     `json:"environment_known"`
	SideEffects      bool     `json:"side_effects"`
	Unstable         bool     `json:"unstable"`
	Reason           string   `json:"reason,omitempty"`
	Suggestion       string   `json:"suggestion,omitempty"`
	Evidence         []string `json:"evidence,omitempty"`
}

/**
 * Runner 只有确认命令的实际输入、工具和运行环境后才能允许 V1 复用。
 * 范围建议与完整性分开，动态依赖或全量约定不能被建议的较小命令替代。
 */
type ScopeAnalyzer interface {
	VerificationScope(context.Context, gitstate.Workspace, shell.Script, string) (VerificationScope, error)
}
