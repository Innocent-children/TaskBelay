package ledger

import (
	"fmt"
	"time"
)

const planSchema = `
CREATE TABLE plans (
 task_id TEXT NOT NULL REFERENCES tasks(id), version INTEGER NOT NULL,
 data TEXT NOT NULL, PRIMARY KEY(task_id,version)
);
PRAGMA user_version=9;
`

type PlanFile struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	Note   string `json:"note"`
}
type PlanEntry struct {
	At   string `json:"at"`
	Note string `json:"note"`
}
type PlanDependency struct {
	Module string `json:"module"`
	Reason string `json:"reason"`
}
type PlanInput struct {
	Content      string           `json:"content"`
	Files        []PlanFile       `json:"files"`
	Entries      []PlanEntry      `json:"entries"`
	Dependencies []PlanDependency `json:"deps"`
	BaseRevision int64            `json:"base_revision"`
}
type PlanConfirmInput struct {
	Version  int64  `json:"version"`
	SourceID string `json:"source_id"`
	Quote    string `json:"quote"`
}
type PlanConfirmation struct {
	Citation     Citation  `json:"citation"`
	Message      Message   `json:"message"`
	Method       string    `json:"method"`
	AuthorizedAt time.Time `json:"authorized_at"`
	RecordedAt   time.Time `json:"recorded_at"`
	Author       Actor     `json:"author"`
	SourceID     string    `json:"source_id"`
	Revision     int64     `json:"revision"`
}
type PlanRevocation struct {
	At       string    `json:"at,omitempty"`
	Citation Citation  `json:"citation"`
	Time     time.Time `json:"time"`
}
type PlanRecord struct {
	Body             string            `json:"body"`
	ID               string            `json:"id"`
	TaskID           string            `json:"task_id"`
	Version          int64             `json:"version"`
	Revision         int64             `json:"revision"`
	Content          string            `json:"content"`
	Path             string            `json:"path"`
	Files            []PlanFile        `json:"files"`
	Entries          []PlanEntry       `json:"entries"`
	Dependencies     []PlanDependency  `json:"deps"`
	Changes          []string          `json:"changes"`
	Author           Actor             `json:"author"`
	SubmittedAt      time.Time         `json:"submitted_at"`
	ObservationID    int64             `json:"observation_id"`
	RequestMessageID string            `json:"request_message_id"`
	Confirmation     *PlanConfirmation `json:"confirmation,omitempty"`
	Revocations      []PlanRevocation  `json:"revocations,omitempty"`
}
type PlanView struct {
	Revision  int64        `json:"revision"`
	Versions  []PlanRecord `json:"versions"`
	Effective *PlanRecord  `json:"effective,omitempty"`
	Pending   *PlanRecord  `json:"pending,omitempty"`
}
type PlanResult struct {
	Workflow     string        `json:"workflow"`
	Plan         *PlanRecord   `json:"plan,omitempty"`
	View         PlanView      `json:"plans"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Message      string        `json:"message,omitempty"`
}
type PlanConflict struct{ Latest PlanView }

func (e *PlanConflict) Error() string {
	return fmt.Sprintf("方案已更新到修订 %d，请读取最新方案后合并", e.Latest.Revision)
}

type WorkflowChange struct {
	From          string    `json:"from"`
	To            string    `json:"to"`
	At            time.Time `json:"at"`
	ObservationID int64     `json:"observation_id"`
	Tree          string    `json:"tree"`
}

type PlanEditContext struct {
	TaskID                string
	Workflow              string
	SettingsRevision      string
	PlanRevision          int64
	AuthorizationRevision int64
	Plans                 PlanView
	Decisions             []Note
}
type PlanEditCheck struct {
	CheckedAt             time.Time
	TaskID                string
	Workflow              string
	SettingsRevision      string
	PlanRevision          int64
	AuthorizationRevision int64
	Paths                 []string
	Issues                []Issue
	Blocked               bool
}
