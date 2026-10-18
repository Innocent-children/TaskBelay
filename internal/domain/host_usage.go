package domain

import (
	"slices"
	"time"
)

type HostUsageTotals struct {
	Tokens   *int64 `json:"tokens" description:"Cumulative tokens for this non-overlapping Task source; null means unavailable, zero means measured zero."`
	ActiveMS *int64 `json:"active_ms" description:"Cumulative Host activity milliseconds for this source; null means unavailable."`
	WaitMS   *int64 `json:"wait_ms" description:"Cumulative Host waiting milliseconds; overlaps other timings and must not be blindly added."`
}
type HostUsageReport struct {
	SourceID        string          `json:"source_id" description:"Stable non-overlapping usage source within this Task; never report a whole inherited session again."`
	ObservedThrough time.Time       `json:"observed_through" description:"UTC RFC3339 observation time for these cumulative totals."`
	Totals          HostUsageTotals `json:"totals" description:"Cumulative source totals, not deltas; repeated samples are counted once."`
	Coverage        string          `json:"coverage" enum:"unreported,partial,reported" description:"reported requires all three totals through observed_through; partial permits null metrics; unreported requires every total to be null."`
}
type HostUsageSummary struct {
	Coverage        string     `json:"coverage" enum:"unreported,partial,reported" description:"unreported has no supplied totals; partial retains known totals without claiming full coverage; reported covers all registered sources at their reported cutoffs."`
	ObservedThrough *time.Time `json:"observed_through" description:"Latest source observation time in UTC; sources can have different cutoffs. Null means no report; later work is not assumed to cost zero."`
	Tokens          *int64     `json:"tokens" description:"覆盖范围内已报告token；null未知，0为已确认零值；不表示账单金额。"`
	ActiveMS        *int64     `json:"active_ms" description:"Host实际活动时间毫秒，null表示无法取得。"`
	WaitMS          *int64     `json:"wait_ms" description:"Host等待时间毫秒，null表示无法取得，不与重叠Core时间相加。"`
}

func (r HostUsageReport) Validate() error {
	if !ID(r.SourceID).IsValid() || validateUTC(r.ObservedThrough) != nil {
		return Invalid("host_usage", "requires a stable Task-specific source_id and UTC observed_through")
	}
	if !slices.Contains([]string{"unreported", "partial", "reported"}, r.Coverage) {
		return Invalid("host_usage.coverage", "requires unreported, partial or reported")
	}
	for _, v := range []*int64{r.Totals.Tokens, r.Totals.ActiveMS, r.Totals.WaitMS} {
		if v != nil && *v < 0 {
			return Invalid("host_usage.totals", "known cumulative totals must be nonnegative")
		}
		if r.Coverage == "reported" && v == nil {
			return Invalid("host_usage.totals", "reported coverage requires all metrics; use partial when a metric is unavailable")
		}
		if r.Coverage == "unreported" && v != nil {
			return Invalid("host_usage.totals", "unreported metrics must be null; use partial for known amounts")
		}
	}
	return nil
}

func (u Usage) UnavailableHostLimit(l Limits) string {
	if l.Tokens > 0 && (u.Host.Coverage != "reported" || u.Host.Tokens == nil) {
		return "tokens"
	}
	if l.HostMS > 0 && (u.Host.Coverage != "reported" || u.Host.ActiveMS == nil || u.Host.WaitMS == nil) {
		return "host_ms"
	}
	return ""
}
