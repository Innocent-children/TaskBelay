package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type Goal struct {
	ID        string   `json:"id"`
	Statement string   `json:"statement"`
	Checks    []string `json:"checks"`
}
type Constraint struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Statement string   `json:"statement"`
	Paths     []string `json:"paths"`
}
type Pin struct {
	Path   string `json:"path"`
	Digest Digest `json:"digest"`
}
type DependencyPolicy struct {
	Path    string   `json:"path"`
	Allowed []string `json:"allowed"`
}
type Limits struct {
	Runs        int64 `json:"runs"`
	ExecutionMS int64 `json:"execution_ms"`
	CoreMS      int64 `json:"core_ms"`
	HostMS      int64 `json:"host_ms"`
	Tokens      int64 `json:"tokens"`
}
type Authorization struct {
	UserRef          string             `json:"user_ref"`
	Request          string             `json:"request"`
	Goals            []Goal             `json:"goals"`
	Constraints      []Constraint       `json:"constraints,omitempty"`
	ExpectedPaths    []string           `json:"expected_paths,omitempty"`
	AllowedPaths     []string           `json:"allowed_paths,omitempty"`
	ForbiddenPaths   []string           `json:"forbidden_paths,omitempty"`
	Pins             []Pin              `json:"pins,omitempty"`
	Dependencies     []DependencyPolicy `json:"dependencies,omitempty"`
	Limits           Limits             `json:"limits,omitempty"`
	ReviewAfterLines int                `json:"review_after_lines,omitempty"`
}
type Check struct {
	ID            string        `json:"id"`
	Kind          string        `json:"kind"`
	Argv          []string      `json:"argv,omitempty"`
	RepositoryKey RepositoryKey `json:"repository_key,omitempty"`
	Cwd           string        `json:"cwd,omitempty"`
	Inputs        []string      `json:"inputs,omitempty"`
	InputBasis    string        `json:"input_basis,omitempty"`
	Environment   []string      `json:"environment,omitempty"`
	Conditions    string        `json:"conditions"`
	ConstraintIDs []string      `json:"constraint_ids,omitempty"`
	TimeoutMS     int64         `json:"timeout_ms,omitempty"`
	Retry         string        `json:"retry,omitempty"`
	Rationale     string        `json:"rationale"`
	GoalIDs       []string      `json:"goal_ids,omitempty"`
	Purpose       string        `json:"purpose,omitempty"`
	FullSuite     bool          `json:"full_suite,omitempty"`
	Compares      string        `json:"compares,omitempty"`
}
type Usage struct {
	Runs          int64 `json:"runs"`
	ExecutionMS   int64 `json:"execution_ms"`
	ReservedMS    int64 `json:"reserved_ms"`
	UncertainMS   int64 `json:"uncertain_ms"`
	CoreCalls     int64 `json:"core_calls"`
	CoreMS        int64 `json:"core_ms"`
	ObservationMS int64 `json:"observation_ms"`
	HostMS        int64 `json:"host_reported_ms"`
	WaitMS        int64 `json:"host_reported_wait_ms"`
	Tokens        int64 `json:"host_reported_tokens"`
}
type PathJudgment struct {
	Path        string   `json:"path"`
	GoalIDs     []string `json:"goal_ids"`
	Basis       string   `json:"basis"`
	ScopeDigest Digest   `json:"scope_digest,omitempty"`
	WorkID      string   `json:"work_id"`
	Necessity   string   `json:"necessity"`
}
type Judgment struct {
	ID          string `json:"id"`
	Verdict     string `json:"verdict"`
	Basis       string `json:"basis"`
	Uncertainty string `json:"uncertainty"`
	ScopeDigest Digest `json:"scope_digest,omitempty"`
}
type Assessment struct {
	AuthorizationDigest Digest         `json:"authorization_digest"`
	ContentDigest       Digest         `json:"content_digest"`
	Paths               []PathJudgment `json:"paths"`
	Constraints         []Judgment     `json:"constraints"`
	Facts               []Judgment     `json:"facts"`
	Summary             string         `json:"summary"`
	Review              *DesignReview  `json:"review,omitempty"`
}
type Progress struct {
	GoalID     string `json:"goal_id"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	GoalDigest Digest `json:"goal_digest,omitempty"`
}
type Issue struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Options []string `json:"options"`
}
type Fact struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Detail   string `json:"detail"`
	Blocking bool   `json:"blocking"`
}
type Attempt struct {
	RunID   string    `json:"run_id"`
	CheckID string    `json:"check_id"`
	Status  string    `json:"status"`
	Summary string    `json:"summary"`
	At      time.Time `json:"at"`
}
type Evidence struct {
	ID                string    `json:"id"`
	CheckID           string    `json:"check_id"`
	Source            string    `json:"source"`
	Status            string    `json:"status"`
	CheckDigest       Digest    `json:"check_digest"`
	InputsDigest      Digest    `json:"inputs_digest"`
	EnvironmentDigest Digest    `json:"environment_digest"`
	Summary           string    `json:"summary"`
	UserRef           string    `json:"user_ref,omitempty"`
	RecordedAt        time.Time `json:"recorded_at"`
}
type Task struct {
	TaskID                string                 `json:"task_id"`
	Host                  Host                   `json:"host"`
	Status                string                 `json:"status"`
	Revision              uint64                 `json:"revision"`
	Authorization         Authorization          `json:"authorization"`
	Plan                  Plan                   `json:"plan"`
	ProposedContentDigest Digest                 `json:"proposed_content_digest"`
	Approval              *PlanApproval          `json:"approval"`
	Exceptions            []KnownFailure         `json:"exceptions"`
	Archived              bool                   `json:"archived"`
	Checks                []Check                `json:"checks"`
	Repositories          []RepositoryScopeEntry `json:"repositories"`
	Assessment            *Assessment            `json:"assessment"`
	Progress              []Progress             `json:"progress"`
	Issue                 *Issue                 `json:"issue"`
	Evidence              []Evidence             `json:"evidence"`
	Attempts              []Attempt              `json:"attempts"`
	Facts                 []Fact                 `json:"facts"`
	Usage                 Usage                  `json:"usage"`
	Acceptance            []Acceptance           `json:"acceptance"`
	Summary               string                 `json:"summary"`
	Risks                 []string               `json:"risks"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}
type Acceptance struct {
	GoalID      string   `json:"goal_id"`
	EvidenceIDs []string `json:"evidence_ids"`
	Basis       string   `json:"basis"`
}

func (a Authorization) Digest() Digest { return Hash(a) }
func (t Task) ContentDigest() Digest {
	vs := []string{}
	for _, r := range t.Repositories {
		vs = append(vs, string(r.Key)+":"+string(r.Binding.ContentDigest))
	}
	return Hash(vs)
}
func (t Task) Goal(id string) (Goal, bool) {
	for _, g := range t.Authorization.Goals {
		if g.ID == id {
			return g, true
		}
	}
	return Goal{}, false
}
func (t Task) Check(id string) (Check, bool) {
	for _, c := range t.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}
func (t Task) Repository(key RepositoryKey) (RepositoryScopeEntry, bool) {
	if key == "" {
		key = "primary"
	}
	for _, r := range t.Repositories {
		if r.Key == key {
			return r, true
		}
	}
	return RepositoryScopeEntry{}, false
}
func (t Task) CheckDigest(c Check) Digest {
	// Approval still binds the full definition; these fields do not change what a result proves.
	c.TimeoutMS, c.Rationale, c.InputBasis, c.Retry, c.FullSuite = 0, "", "", "", false
	goals := []Goal{}
	constraints := []Constraint{}
	for _, g := range t.Authorization.Goals {
		if slices.Contains(g.Checks, c.ID) {
			goals = append(goals, g)
		}
	}
	for _, v := range t.Authorization.Constraints {
		if len(c.ConstraintIDs) == 0 || slices.Contains(c.ConstraintIDs, v.ID) {
			constraints = append(constraints, v)
		}
	}
	return Hash(struct {
		Check       Check
		Goals       []Goal
		Constraints []Constraint
	}{c, goals, constraints})
}
func DefaultLimits() Limits {
	return Limits{Runs: 20, ExecutionMS: 600000, CoreMS: 600000, HostMS: 3600000, Tokens: 1000000}
}
func MatchPath(scope, p string) bool {
	return scope == "." || scope == p || strings.HasPrefix(p, strings.TrimSuffix(scope, "/")+"/")
}
func (a Authorization) Validate() error {
	if e := Text("authorization.user_ref", a.UserRef); e != nil {
		return e
	}
	if e := Text("authorization.request", a.Request); e != nil {
		return e
	}
	if len(a.Goals) == 0 || len(a.Goals) > 64 {
		return Invalid("authorization.goals", "must contain 1..64 goals")
	}
	ids := []string{}
	for i, g := range a.Goals {
		p := fmt.Sprintf("authorization.goals[%d]", i)
		ids = append(ids, g.ID)
		if e := Text(p+".statement", g.Statement); e != nil {
			return e
		}
		if len(g.Checks) == 0 {
			return Invalid(p+".checks", "every goal requires at least one check")
		}
		if e := IDs(p+".checks", g.Checks); e != nil {
			return e
		}
	}
	if e := IDs("authorization.goals.id", ids); e != nil {
		return e
	}
	ids = nil
	for i, c := range a.Constraints {
		p := fmt.Sprintf("authorization.constraints[%d]", i)
		ids = append(ids, c.ID)
		if !slices.Contains([]string{"non_goal", "interface", "data", "dependency", "design"}, c.Kind) {
			return Invalid(p+".kind", "must be non_goal, interface, data, dependency or design")
		}
		if e := Text(p+".statement", c.Statement); e != nil {
			return e
		}
		if e := Paths(p+".paths", c.Paths); e != nil {
			return e
		}
	}
	if e := IDs("authorization.constraints.id", ids); e != nil {
		return e
	}
	if e := Paths("authorization.expected_paths", a.ExpectedPaths); e != nil {
		return e
	}
	if e := Paths("authorization.allowed_paths", a.AllowedPaths); e != nil {
		return e
	}
	if e := Paths("authorization.forbidden_paths", a.ForbiddenPaths); e != nil {
		return e
	}
	if len(a.Pins) > 64 || len(a.Dependencies) > 64 {
		return Invalid("authorization", "at most 64 pins and dependency policies")
	}
	for i, p := range a.Pins {
		if ScopedPath(p.Path) != nil || !p.Digest.IsValid() {
			return Invalid(fmt.Sprintf("authorization.pins[%d]", i), "requires a repository path and observed SHA-256 digest")
		}
	}
	for i, d := range a.Dependencies {
		if ScopedPath(d.Path) != nil || len(d.Allowed) > 64 {
			return Invalid(fmt.Sprintf("authorization.dependencies[%d]", i), "requires a repository manifest path and at most 64 dependency names")
		}
	}
	l := a.Limits
	if l.Runs < 1 || l.Runs > 10000 || l.ExecutionMS < 1 || l.ExecutionMS > 86400000 || l.CoreMS < 1 || l.HostMS < 1 || l.Tokens < 1 {
		return Invalid("authorization.limits", "positive cumulative limits required; runs <=10000 and execution_ms <=86400000")
	}
	if a.ReviewAfterLines < 0 {
		return Invalid("authorization.review_after_lines", "must be nonnegative")
	}
	return nil
}
func ScopedPath(v string) error {
	if key, relative, ok := strings.Cut(v, "::"); ok {
		if !RepositoryKey(key).IsValid() {
			return ErrInvalidArgument
		}
		return ValidatePath(relative)
	}
	return ValidatePath(v)
}
func Paths(p string, vs []string) error {
	if len(vs) > 256 {
		return Invalid(p, "at most 256 paths")
	}
	seen := map[string]bool{}
	for i, v := range vs {
		if ScopedPath(v) != nil || seen[v] {
			return Invalid(fmt.Sprintf("%s[%d]", p, i), "must be a unique relative path or directory (use . for the repository)")
		}
		seen[v] = true
	}
	return nil
}
func (t Task) Validate() error {
	if !ID(t.TaskID).IsValid() || !t.Host.IsValid() || t.Revision == 0 || !slices.Contains([]string{"active", "done", "cancelled"}, t.Status) {
		return Invalid("task", "invalid identity, host, revision or lifecycle")
	}
	if e := t.Authorization.Validate(); e != nil {
		return e
	}
	if e := t.ValidatePlan(); e != nil {
		return e
	}
	if len(t.Checks) == 0 || len(t.Checks) > 64 || len(t.Repositories) == 0 || len(t.Repositories) > 8 || len(t.Evidence) > 192 || len(t.Attempts) > 64 || len(t.Progress) > 64 {
		return Invalid("task", "current snapshot exceeds its bounded collections")
	}
	ids := []string{}
	for i, c := range t.Checks {
		if c.Kind == "command" && c.Retry != "repeatable" && c.Retry != "requires_user" {
			return Invalid(fmt.Sprintf("checks[%d].retry", i), "must be repeatable or requires_user")
		}
		p := fmt.Sprintf("checks[%d]", i)
		if !slices.Contains([]string{"acceptance", "regression", "diagnostic"}, c.Purpose) {
			return Invalid(p+".purpose", "requires acceptance, regression or diagnostic")
		}
		if len(t.CheckGoals(c)) == 0 {
			return Invalid(p+".goal_ids", "every check must serve an authorized goal")
		}
		if e := IDs(p+".goal_ids", c.GoalIDs); e != nil {
			return e
		}
		for _, id := range c.GoalIDs {
			if _, ok := t.Goal(id); !ok {
				return Invalid(p+".goal_ids", "goal must exist")
			}
		}
		if c.Compares != "" {
			compared, ok := t.Check(c.Compares)
			if !ok || compared.ID == c.ID || compared.Kind != "command" || compared.Purpose != "regression" || c.Kind != "command" {
				return Invalid(p+".compares", "comparison must reference another regression command")
			}
		}
		ids = append(ids, c.ID)
		if c.Kind != "command" && c.Kind != "human" {
			return Invalid(p+".kind", "must be command or human")
		}
		if _, ok := t.Repository(c.RepositoryKey); !ok {
			return Invalid(p+".repository_key", "must name a bound repository")
		}
		if ValidatePath(c.Cwd) != nil {
			return Invalid(p+".cwd", "must be a relative directory")
		}
		if e := Paths(p+".inputs", c.Inputs); e != nil {
			return e
		}
		for _, input := range c.Inputs {
			if key, _, qualified := strings.Cut(input, "::"); qualified {
				if _, ok := t.Repository(RepositoryKey(key)); !ok {
					return Invalid(p+".inputs", "input repository must be bound to this task")
				}
			}
		}
		if len(c.Inputs) == 0 {
			return Invalid(p+".inputs", "must declare inputs, default .")
		}
		if len(c.Inputs) != 1 || c.Inputs[0] != "." {
			if e := Text(p+".input_basis", c.InputBasis); e != nil {
				return e
			}
		}
		if e := Text(p+".rationale", c.Rationale); e != nil {
			return e
		}
		if e := Text(p+".conditions", c.Conditions); e != nil {
			return e
		}
		if c.Kind == "command" && (len(c.Argv) == 0 || len(c.Argv) > 128 || c.TimeoutMS < 1 || c.TimeoutMS > 3600000) {
			return Invalid(p, "command requires argv and timeout_ms in 1..3600000")
		}
		for j, v := range c.Argv {
			if strings.ContainsRune(v, 0) || len(v) > 8192 || j == 0 && v == "" {
				return Invalid(p+".argv", "invalid command argument")
			}
		}
		if e := IDs(p+".constraint_ids", c.ConstraintIDs); e != nil {
			return e
		}
		for _, id := range c.ConstraintIDs {
			found := false
			for _, v := range t.Authorization.Constraints {
				found = found || v.ID == id
			}
			if !found {
				return Invalid(p+".constraint_ids", "constraint must exist")
			}
		}
		for _, name := range c.Environment {
			if name == "" || strings.ContainsAny(name, "=\x00") {
				return Invalid(p+".environment", "requires environment variable names")
			}
		}
	}
	if e := IDs("checks.id", ids); e != nil {
		return e
	}
	for _, g := range t.Authorization.Goals {
		for _, id := range g.Checks {
			if _, ok := t.Check(id); !ok {
				return Invalid("authorization.goals.checks", "references an unknown check")
			}
		}
	}
	keys := []string{}
	for _, r := range t.Repositories {
		keys = append(keys, string(r.Key))
		if e := r.Validate(); e != nil {
			return Invalid("repositories", "invalid observed binding")
		}
	}
	if e := IDs("repositories.key", keys); e != nil {
		return e
	}
	return nil
}
