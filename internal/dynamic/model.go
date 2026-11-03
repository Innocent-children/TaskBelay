package dynamic

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"time"
)

type Command struct {
	ID, Program, Directory, Shell, Environment, Tool, Output string
	Args                                                     []string
	Cases                                                    []string
	Trusted                                                  bool
	Duration                                                 time.Duration
}
type Basis struct {
	ID, Command, Requirement, Source string
	Cases                            []string
	Primary, Existing                bool
}
type Input struct {
	Workspace     gitstate.Workspace
	Tree          string
	Before, After repostats.Repository
	Changes       changes.Result
	Signals       signals.Result
	Regions       []changes.Region
	Basis         []Basis
	Commands      []Command
	Runs          []repostats.Run
	Parallel      int
	Confidence    map[string]float64
	Resume        *Result
	Pause         func(context.Context) (bool, error)
}
type RunResult struct {
	ID, Command, Phase, State, Output, Reason string
	ExitCode                                  *int
	Started                                   bool
	Duration                                  time.Duration
	Resources                                 *platform.ProcessResources
	Cases                                     []repostats.CaseResult
}
type Scope struct {
	Command, Path, Key, State, Reason string
	Baseline, Sentinel                RunResult
}
type Patch struct {
	Path, Before, After string
	Remove, Create      bool
	Mode                string
}
type Trial struct {
	IDs           []string
	State, Reason string
	Runs          []RunResult
	Passed        bool
	Supplemental  []string
}
type Mutation struct {
	ID, Cluster, Path, Operator, Extension string
	Start, End                             int
	Before, After                          string
}
type Discovery struct {
	Mutation Mutation
	State    string
	Cases    []string
	Runs     []RunResult
}
type Operator struct {
	Valid, Invalid, Unknown int
	Disabled                bool
}
type Probe struct {
	Command, State string
	Added          []string
	Runs           []RunResult
}
type Result struct {
	State                   string
	Runs                    []RunResult
	Scopes                  []Scope
	Basis                   []Basis
	Probes                  []Probe
	Trials                  []Trial
	Rollback, Required      []string
	Patch                   []Patch
	UnifiedPatch            string
	Confirmed               bool
	Matrix                  []Discovery
	Operators               map[string]Operator
	Kept, Removed, TestGaps []string
	ReductionConfirmed      bool
	ReductionPatch          []Patch
	UnseenProbability       float64
	Gaps                    []string
	Errors, Incomplete      []string
	Clusters                []changes.Cluster
}

type runner struct {
	input      Input
	result     Result
	root, base string
	publish    func(Result) error
	model      repostats.ResultModel
	samples    []repostats.Run
	started    int
	search     context.Context
	ctx        context.Context
	merged     map[string]string
}
