package ledger

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
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

// 改变候选或判断规则时更新。纯展示调整沿用独立的反馈格式版本。
const checkRulesVersion = 6

type CheckInput struct {
	RulesVersion          int               `json:"rules_version,omitempty"`
	TaskID                string            `json:"task_id"`
	OriginTree            string            `json:"origin_tree"`
	Tree                  string            `json:"tree"`
	RequirementRevision   int64             `json:"requirement_revision"`
	PlanRevision          int64             `json:"plan_revision"`
	EditRevision          int64             `json:"edit_revision"`
	AuthorizationRevision int64             `json:"authorization_revision"`
	Verifications         string            `json:"verifications,omitempty"`
	Feedback              []string          `json:"feedback,omitempty"`
	PurposeNotes          []string          `json:"purpose_notes,omitempty"`
	Decisions             []string          `json:"decisions"`
	AnalysisSettings      map[string]string `json:"analysis_settings"`
	Algorithm             string            `json:"algorithm"`
}

func (input CheckInput) Version() string {
	data, _ := json.Marshal(input)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

/**
 * 输入比较只采用判定所需的值；设置来源、观察时间和普通笔记修订不参与。
 * 决定按生效目标及引用去重，避免重复记录同一决定导致分析输入版本变化。
 */
func NewCheckInput(task TaskRecord, tree string, settings EffectiveSettings, decisions []Note, algorithm string) CheckInput {
	input := CheckInput{RulesVersion: checkRulesVersion, TaskID: task.ID, OriginTree: task.OriginTree, Tree: tree,
		RequirementRevision: task.RequirementRevision, PlanRevision: task.PlanRevision,
		EditRevision: task.RecordRevision, AuthorizationRevision: task.AuthorizationRevision,
		AnalysisSettings: map[string]string{}, Decisions: []string{}, Algorithm: algorithm}
	for name, setting := range settings.Values {
		if name == "workflow" || name == "analysis.tests" || name == "analysis.parallel" {
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
	Kind      string `json:"kind,omitempty"`
	Count     int    `json:"count,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

func (g CheckGap) Size() int { return max(1, g.Count) }

/**
 * 固定输入的信息不足不会因重跑消失；复用保留原状态和缺口。
 * 执行错误、取消、超时及未知失败必须允许重新执行。
 */
func (r CheckRecord) ReusableStatic() bool {
	if r.Input.Algorithm == "" || r.Depth != "static" || r.Status != CheckComplete && r.Status != CheckPartial {
		return false
	}
	if r.Status == CheckPartial && len(r.Missing) == 0 {
		return false
	}
	for _, gap := range r.Missing {
		if gap.Kind != "unknown" {
			return false
		}
	}
	return true
}

func (r CheckRecord) GapCounts() map[string]int {
	counts := map[string]int{"unknown": 0, "error": 0, "incomplete": 0}
	for _, gap := range r.Missing {
		kind := gap.Kind
		if kind != "unknown" && kind != "error" {
			kind = "incomplete"
		}
		counts[kind] += gap.Size()
	}
	return counts
}

type Issue struct {
	HistoricalVersion string             `json:"historical_version,omitempty"`
	Historical        json.RawMessage    `json:"historical,omitempty"`
	ContentSignature  []uint64           `json:"content_signature,omitempty"`
	Selected          bool               `json:"selected"`
	Algorithm         string             `json:"algorithm,omitempty"`
	Confidence        float64            `json:"confidence"`
	Signals           map[string]float64 `json:"signals,omitempty"`
	BlockOnDelivery   bool               `json:"block_on_delivery,omitempty"`
	ReviewOnly        bool               `json:"review_only,omitempty"`
	ID                string             `json:"id"`
	Identity          string             `json:"identity"`
	Occurrence        int                `json:"occurrence"`
	Rule              string             `json:"rule"`
	Level             string             `json:"level"`
	At                string             `json:"at"`
	LogicalAt         string             `json:"logical_at"`
	FactKey           string             `json:"fact_key"`
	Facts             []string           `json:"facts"`
	Advice            string             `json:"advice"`
	Evidence          []string           `json:"evidence"`
	Status            string             `json:"status"`
	HandledBy         []string           `json:"handled_by,omitempty"`
	Blocked           []Actor            `json:"blocked_sessions,omitempty"`
}

func WithDelivery(record CheckRecord, delivery bool) CheckRecord {
	record.Issues = append([]Issue(nil), record.Issues...)
	for i := range record.Issues {
		if record.Issues[i].BlockOnDelivery {
			record.Issues[i].Level = "hint"
			if delivery && record.Status != CheckExpired && record.Status != CheckFailed {
				record.Issues[i].Level = "block"
			}
		}
	}
	record.Level = HighestLevel(record.Issues)
	if record.Level == "pass" {
		if record.Status != CheckComplete {
			record.Level = "unknown"
		} else {
			record.Level = "no_findings"
		}
	}
	return record
}

type CheckRecord struct {
	HistoricalVersion string            `json:"historical_version,omitempty"`
	FeedbackVersion   int               `json:"feedback_version,omitempty"`
	Reused            bool              `json:"-"`
	Static            *CheckRecord      `json:"static,omitempty"`
	Locations         []Requirement     `json:"-"`
	ID                int64             `json:"id"`
	Depth             string            `json:"depth"`
	Input             CheckInput        `json:"input"`
	InputVersion      string            `json:"input_version"`
	Status            CheckStatus       `json:"status"`
	ComputedStatus    CheckStatus       `json:"computed_status"`
	Level             string            `json:"level"`
	Issues            []Issue           `json:"issues"`
	Unconfirmed       []Issue           `json:"unconfirmed,omitempty"`
	Missing           []CheckGap        `json:"missing"`
	Evaluated         []string          `json:"evaluated"`
	Relocations       map[string]string `json:"relocations,omitempty"`
	Report            DeliveryReport    `json:"report"`
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        time.Time         `json:"finished_at"`
}

type DeliveryReport struct {
	Analysis             map[string]json.RawMessage `json:"analysis,omitempty"`
	Counts               map[string]int             `json:"counts"`
	Dynamic              *DynamicReport             `json:"dynamic,omitempty"`
	Historical           json.RawMessage            `json:"historical,omitempty"`
	ValidationNotes      []Note                     `json:"validation_notes"`
	Plans                PlanView                   `json:"plans"`
	Verifications        []VerificationRecord       `json:"verifications"`
	ChangesByRequirement map[string][]string        `json:"changes_by_requirement"`
	Requirements         []Requirement              `json:"requirements"`
	Unfinished           []Requirement              `json:"unfinished_requirements"`
	Revoked              []Requirement              `json:"revoked_requirements"`
	Changes              []string                   `json:"changes"`
	Decisions            []Note                     `json:"decisions"`
	WorkspaceUpdates     []FileChange               `json:"workspace_updates"`
	NotRun               []string                   `json:"not_run"`
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
