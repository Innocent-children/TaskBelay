package ledger

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/testrun"
)

const checkSchema = `
CREATE TABLE checks (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT NOT NULL REFERENCES tasks(id),
 depth TEXT NOT NULL, input_version TEXT NOT NULL, status TEXT NOT NULL, data TEXT NOT NULL
);
CREATE INDEX checks_input ON checks(task_id,depth,input_version,seq);
CREATE TABLE finish_requests (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), agent TEXT NOT NULL,
 session_id TEXT NOT NULL, round_id TEXT NOT NULL, task_id TEXT NOT NULL REFERENCES tasks(id),
 state TEXT NOT NULL, data TEXT NOT NULL,
 PRIMARY KEY(workspace_id,agent,session_id,round_id)
);
CREATE TABLE issues (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT NOT NULL REFERENCES tasks(id),
 identity TEXT NOT NULL, occurrence INTEGER NOT NULL, status TEXT NOT NULL, data TEXT NOT NULL,
 UNIQUE(task_id,identity,occurrence)
);
CREATE TABLE issue_blocks (
 issue_id TEXT NOT NULL, agent TEXT NOT NULL, session_id TEXT NOT NULL, round_id TEXT NOT NULL,
 feedback_id INTEGER NOT NULL, PRIMARY KEY(issue_id,agent,session_id)
);
CREATE TABLE check_feedback (
 id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id TEXT NOT NULL,
 agent TEXT NOT NULL, session_id TEXT NOT NULL, round_id TEXT NOT NULL
);
PRAGMA user_version=7;
`

type CheckInput struct {
	TaskID                string            `json:"task_id"`
	OriginTree            string            `json:"origin_tree"`
	Tree                  string            `json:"tree"`
	RequirementRevision   int64             `json:"requirement_revision"`
	PlanRevision          int64             `json:"plan_revision"`
	EditRevision          int64             `json:"edit_revision"`
	AuthorizationRevision int64             `json:"authorization_revision"`
	Decisions             []string          `json:"decisions"`
	AnalysisSettings      map[string]string `json:"analysis_settings"`
	Rules                 string            `json:"rules"`
	Capabilities          string            `json:"capabilities"`
}

func (input CheckInput) Version() string {
	data, _ := json.Marshal(input)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

/**
 * 输入比较只采用判定所需的值；设置来源、观察时间和普通笔记修订不参与。
 * 决定按生效目标及引用去重，避免重复记同一决定改变分析身份。
 */
func NewCheckInput(task TaskRecord, tree string, settings EffectiveSettings, decisions []Note, rules, capabilities string) CheckInput {
	input := CheckInput{TaskID: task.ID, OriginTree: task.OriginTree, Tree: tree,
		RequirementRevision: task.RequirementRevision, PlanRevision: task.PlanRevision,
		EditRevision: task.RecordRevision, AuthorizationRevision: task.AuthorizationRevision,
		AnalysisSettings: map[string]string{}, Decisions: []string{}, Rules: rules, Capabilities: capabilities}
	for name, setting := range settings.Values {
		if name == "workflow" || name == "analysis.tests" || strings.HasPrefix(name, "test.") {
			input.AnalysisSettings[name] = setting.Value
		}
	}
	seen := map[string]bool{}
	for _, note := range decisions {
		key := decisionKey(note)
		if !seen[key] {
			input.Decisions = append(input.Decisions, key)
			seen[key] = true
		}
	}
	sort.Strings(input.Decisions)
	return input
}

func decisionKey(note Note) string {
	data, _ := json.Marshal(struct {
		Target *DecisionTarget
		Source *Citation
	}{note.Target, note.Citation})
	return string(data)
}

type CheckStatus string

const (
	CheckComplete CheckStatus = "complete"
	CheckPartial  CheckStatus = "partial"
	CheckFailed   CheckStatus = "failed"
	CheckExpired  CheckStatus = "expired"
)

type CheckGap struct {
	At        string `json:"at,omitempty"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
}

type Issue struct {
	BlockOnDelivery bool     `json:"block_on_delivery,omitempty"`
	ID              string   `json:"id"`
	Identity        string   `json:"identity"`
	Occurrence      int      `json:"occurrence"`
	Rule            string   `json:"rule"`
	Level           string   `json:"level"`
	At              string   `json:"at"`
	LogicalAt       string   `json:"logical_at"`
	FactKey         string   `json:"fact_key"`
	Facts           []string `json:"facts"`
	Advice          string   `json:"advice"`
	Evidence        []string `json:"evidence"`
	Status          string   `json:"status"`
	HandledBy       []string `json:"handled_by,omitempty"`
	Blocked         []Actor  `json:"blocked_sessions,omitempty"`
}

func WithDelivery(record CheckRecord, delivery bool) CheckRecord {
	record.Issues = append([]Issue(nil), record.Issues...)
	for i := range record.Issues {
		switch record.Issues[i].Rule {
		case "T1", "T2", "T3", "T4", "T5", "T6":
			record.Issues[i].Level = "hint"
			record.Issues[i].BlockOnDelivery = false
			continue
		}
		if record.Issues[i].BlockOnDelivery {
			record.Issues[i].Level = "hint"
			if delivery && record.Status != CheckExpired && record.Status != CheckFailed {
				record.Issues[i].Level = "block"
			}
		}
	}
	record.Level = HighestLevel(record.Issues)
	return record
}

type CheckRecord struct {
	Static         *CheckRecord      `json:"static,omitempty"`
	Locations      []Requirement     `json:"-"`
	ID             int64             `json:"id"`
	Depth          string            `json:"depth"`
	Input          CheckInput        `json:"input"`
	InputVersion   string            `json:"input_version"`
	Status         CheckStatus       `json:"status"`
	ComputedStatus CheckStatus       `json:"computed_status"`
	Level          string            `json:"level"`
	Issues         []Issue           `json:"issues"`
	Missing        []CheckGap        `json:"missing"`
	Evaluated      []string          `json:"evaluated"`
	Relocations    map[string]string `json:"relocations,omitempty"`
	Report         DeliveryReport    `json:"report"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at"`
}

type DeliveryReport struct {
	Dynamic              *testrun.Report      `json:"dynamic,omitempty"`
	Tests                []TestFact           `json:"tests"`
	TestRisk             []TestRisk           `json:"test_risk"`
	ValidationNotes      []Note               `json:"validation_notes"`
	Plans                PlanView             `json:"plans"`
	Verifications        []VerificationRecord `json:"verifications"`
	ChangesByRequirement map[string][]string  `json:"changes_by_requirement"`
	Requirements         []Requirement        `json:"requirements"`
	Unfinished           []Requirement        `json:"unfinished_requirements"`
	Revoked              []Requirement        `json:"revoked_requirements"`
	Changes              []string             `json:"changes"`
	Decisions            []Note               `json:"decisions"`
	WorkspaceUpdates     []FileChange         `json:"workspace_updates"`
	NotRun               []string             `json:"not_run"`
}

type FinishInput struct {
	Outcome  string `json:"outcome,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	Quote    string `json:"quote,omitempty"`
}

type FinishRequest struct {
	TaskID       string     `json:"task_id"`
	Actor        Actor      `json:"actor"`
	Input        CheckInput `json:"input"`
	InputVersion string     `json:"input_version"`
	State        string     `json:"state"`
	Feedback     bool       `json:"feedback"`
	CheckID      int64      `json:"check_id,omitempty"`
	At           time.Time  `json:"at"`
}

type FinishResult struct {
	Task    TaskRecord     `json:"task"`
	Request *FinishRequest `json:"request,omitempty"`
	Static  *CheckRecord   `json:"static,omitempty"`
	Dynamic *CheckRecord   `json:"dynamic,omitempty"`
}

func CheckScope(rule, at string) string { return rule + "\x00" + at }

func HighestLevel(issues []Issue) string {
	level := "pass"
	for _, issue := range issues {
		if issue.Status != "open" {
			continue
		}
		if issue.Level == "block" {
			return "block"
		}
		level = "hint"
	}
	return level
}
