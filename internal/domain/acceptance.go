package domain

import "time"

type KnownFailure struct {
	CheckID              string    `json:"check_id" description:"当前已保存检查的稳定身份。"`
	FailedEvidenceID     string    `json:"failed_evidence_id" description:"Current actual failed regression result."`
	ComparisonEvidenceID string    `json:"comparison_evidence_id" description:"对应compares诊断检查的当前实际通过记录。"`
	Failures             []string  `json:"failures" description:"用户接受的具体已有失败名称，不能笼统写所有失败。"`
	UserRef              string    `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Reason               string    `json:"reason" description:"操作/处理的实际原因，不能替代所需用户决定。"`
	AcceptedAt           time.Time `json:"accepted_at" description:"UTC time when the actual user acceptance was recorded."`
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
