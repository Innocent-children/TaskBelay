package domain

import "time"

type Change struct {
	Revision uint64    `json:"revision"`
	Kind     string    `json:"kind"`
	Summary  string    `json:"summary"`
	UserRef  string    `json:"user_ref,omitempty"`
	At       time.Time `json:"at"`
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
