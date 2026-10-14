package domain

import "time"

type KnownFailure struct {
	CheckID              string    `json:"check_id"`
	FailedEvidenceID     string    `json:"failed_evidence_id"`
	ComparisonEvidenceID string    `json:"comparison_evidence_id"`
	Failures             []string  `json:"failures"`
	UserRef              string    `json:"user_ref"`
	Reason               string    `json:"reason"`
	AcceptedAt           time.Time `json:"accepted_at"`
}

func (t Task) AcceptedFailures(fresh []Freshness) []KnownFailure {
	accepted := []KnownFailure{}
	for _, x := range t.Exceptions {
		c, ok := t.Check(x.CheckID)
		if !ok || c.Purpose != "regression" {
			continue
		}
		failed, comparison := false, false
		for _, f := range fresh {
			for _, e := range t.Evidence {
				if e.ID != f.EvidenceID || e.Source != "core_execution" {
					continue
				}
				if e.ID == x.FailedEvidenceID && e.CheckID == c.ID && e.Status == "failed" && f.Applicable {
					failed = true
				}
				if e.ID == x.ComparisonEvidenceID && f.Valid {
					if cc, ok := t.Check(e.CheckID); ok && cc.Compares == c.ID {
						comparison = true
					}
				}
			}
		}
		if failed && comparison {
			accepted = append(accepted, x)
		}
	}
	return accepted
}
