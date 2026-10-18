package domain

import "time"

type Change struct {
	Revision uint64    `json:"revision" description:"当前持久化修订。检查预留/结算都可能推进，不能自己加一。"`
	Kind     string    `json:"kind" description:"Saved change category: authorization, plan, budget, user_decision, accepted_failure or concern."`
	Summary  string    `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	UserRef  string    `json:"user_ref,omitempty" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	At       time.Time `json:"at" description:"实际保存时间，UTC。"`
}

/**
 * Retain the reasons needed to resume work without storing another Task snapshot
 * or workflow. Current restrictions, unresolved issues and lifetime usage stay on Task.
 */
func (t *Task) RecordChange(kind, summary, userRef string) {
	t.Changes = append(t.Changes, Change{Revision: t.Revision + 1, Kind: kind, Summary: summary, UserRef: userRef, At: time.Now().UTC()})
	if len(t.Changes) > 64 {
		t.Changes = append([]Change(nil), t.Changes[len(t.Changes)-64:]...)
	}
}
