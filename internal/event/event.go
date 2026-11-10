package event

import "time"

type Kind string

const (
	SessionStart Kind = "session_start"
	UserPrompt   Kind = "user_prompt"
	CommandUse   Kind = "command_use"
	EditUse      Kind = "edit_use"
	MCPUse       Kind = "mcp_use"
	ToolEnd      Kind = "tool_end"
	Stop         Kind = "stop"
)

type StartMode string

const (
	NewSession StartMode = "new"
	Resume     StartMode = "resume"
	Clear      StartMode = "clear"
	Compact    StartMode = "compact"
	Fork       StartMode = "fork"
)

type Shell string

const (
	POSIX      Shell = "posix"
	PowerShell Shell = "powershell"
)

type Event struct {
	Kind       Kind
	Agent      string
	SessionID  string
	RoundID    string
	WorkingDir string
	ToolCallID string
	MCPTool    string
	StartMode  StartMode
	Message    string
	MessageID  string
	Command    *Command
	Edits      []FileEdit
	Result     *ToolResult
}

type Command struct {
	Text       string
	Shell      Shell
	Background bool
}

type FileEdit struct {
	Path         string
	Before       *string
	After        *string
	Patch        string
	WholeFile    bool
	Delete       bool
	ReplaceAll   bool
	Unverifiable string
	Hunks        []TextHunk
}

type TextHunk struct {
	Anchor *string
	Lines  []string
	AtEnd  bool
}

type ToolState string

const (
	Unknown     ToolState = "unknown"
	Running     ToolState = "running"
	Completed   ToolState = "completed"
	Failed      ToolState = "failed"
	Interrupted ToolState = "interrupted"
)

type ToolResult struct {
	State    ToolState
	Output   string
	ExitCode *int
	Edits    []FileEdit
}

type SessionRecord struct {
	At      time.Time
	TaskID  string
	Action  string
	Object  string
	Detail  string
	Sources []string
}

/**
 * Decision 的零值表示不干预；可同时提供上下文、拒绝工具调用、阻止结束和提示用户。
 * 没有强制允许字段，避免覆盖 Agent 自己的权限判断。
 */
type Decision struct {
	FeedbackKey       string
	Records           []SessionRecord `json:"-"`
	AdditionalContext string
	DenyTool          string
	BlockStop         string
	UserMessage       string
}

const (
	FastBudget        = 50 * time.Millisecond
	EditBudget        = 300 * time.Millisecond
	MessageBudget     = time.Second
	ObservationBudget = 5 * time.Second
	// MaintenanceBudget 是 session 开始后保留期清理的时间；输出已经交给 Agent，不影响提示送达。
	MaintenanceBudget = 2 * time.Second
	MCPBudget         = time.Second
	StaticBudget      = 20 * time.Second
	TestsBudget       = 120 * time.Second
)
