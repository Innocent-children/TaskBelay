package testrun

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type Failure string

const (
	AssertionFailure   Failure = "assertion"
	CompileFailure     Failure = "compile"
	EnvironmentFailure Failure = "environment"
	Timeout            Failure = "timeout"
	Canceled           Failure = "canceled"
	SelectionFailure   Failure = "selection"
	CoverageFailure    Failure = "coverage"
)

type Result struct {
	Failure     Failure                   `json:"failure,omitempty"`
	Reason      string                    `json:"reason,omitempty"`
	ExitCode    int                       `json:"exit_code"`
	Duration    time.Duration             `json:"duration_ns"`
	Output      string                    `json:"output,omitempty"`
	ErrorOutput string                    `json:"error_output,omitempty"`
	Truncated   bool                      `json:"truncated,omitempty"`
	Process     *platform.ProcessEvidence `json:"process,omitempty"`
}

func (r Result) Passed() bool { return r.Failure == "" && r.ExitCode == 0 }

type Line struct {
	File   string `json:"file"`
	Number int    `json:"line"`
}

type Coverage struct {
	Valid        bool   `json:"valid"`
	CopyExecuted bool   `json:"copy_executed"`
	Lines        []Line `json:"lines"`
	Reason       string `json:"reason,omitempty"`
}

type Case struct {
	Source       codescan.TestCase `json:"-"`
	At           string            `json:"at"`
	Name         string            `json:"name"`
	Package      string            `json:"package"`
	Change       string            `json:"change"`
	Requirements []string          `json:"requirements"`
	AgentOnly    bool              `json:"agent_only"`
	Mixed        bool              `json:"mixed"`
	Retained     bool              `json:"retained"`
	Whole        bool              `json:"whole"`
	Expected     []string          `json:"expected_names,omitempty"`
	Selector     string            `json:"selector,omitempty"`
	Unknown      []string          `json:"unknown,omitempty"`
	Assertions   []string          `json:"assertions,omitempty"`
}

type RunResult struct {
	Result
	Selected []string `json:"selected"`
	Coverage Coverage `json:"coverage"`
	Profile  string   `json:"-"`
}

type Observation struct {
	Case     Case      `json:"case"`
	Baseline RunResult `json:"baseline"`
	Killed   []string  `json:"killed"`
}

type Mutation = codescan.Mutation

type MutationRun struct {
	Mutation string `json:"mutation"`
	Case     string `json:"case"`
	Result   Result `json:"result"`
}

type Group struct {
	Requirement string        `json:"requirement"`
	Coverage    []Line        `json:"coverage"`
	Cases       []string      `json:"cases"`
	Mutations   []Mutation    `json:"mutations"`
	Runs        []MutationRun `json:"runs"`
	Complete    bool          `json:"complete"`
	Keep        []string      `json:"keep,omitempty"`
	Candidates  []string      `json:"candidates,omitempty"`
	Reason      string        `json:"reason,omitempty"`
}

type Stage struct {
	Name   string `json:"name"`
	Result Result `json:"result"`
}

type Report struct {
	Rules        string        `json:"rules"`
	Tree         string        `json:"tree"`
	Complete     bool          `json:"complete"`
	Observations []Observation `json:"observations"`
	Groups       []Group       `json:"groups"`
	Stages       []Stage       `json:"stages"`
	Missing      []string      `json:"missing"`
	Duration     time.Duration `json:"duration_ns"`
	ExecutionID  string        `json:"execution_id"`
	Reusable     bool          `json:"reusable"`
}

type Config struct {
	Command, Prepare string
	Parallel         int
}

type Request struct {
	Snapshot     codescan.Snapshot
	Cases        []Case
	Requirements map[string][]Line
	Changed      map[string][]Line
	Config       map[string]Config
}

/**
 * Runner 只返回实际运行和覆盖证据；精简候选由公共流程根据完成情况产生。
 * 编译与单条运行分开，同一个固定源码版本不为每条用例重复编译。
 */
type Runner interface {
	Prepare(context.Context) Result
	Compile(context.Context) Result
	Enumerate(context.Context, []Case) ([]Case, Result)
	Run(context.Context, Case, string) RunResult
	ReadCoverage(context.Context, RunResult) Coverage
	Terminate()
}
