package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

/**
 * Source high-water marks and Task totals are saved in the caller's CAS
 * transaction. Retrying a sample under another operation ID cannot add it twice.
 */
func saveHostUsage(ctx context.Context, tx *sql.Tx, taskID string, report *domain.HostUsageReport) ([3]int64, error) {
	delta := [3]int64{}
	if report == nil {
		return delta, nil
	}
	if err := report.Validate(); err != nil {
		return delta, err
	}
	var old [3]sql.NullInt64
	var observed, coverage string
	err := tx.QueryRowContext(ctx, "SELECT observed_through,tokens,active_ms,wait_ms,coverage FROM host_usage_sources WHERE task_id=? AND source_id=?", taskID, report.SourceID).Scan(&observed, &old[0], &old[1], &old[2], &coverage)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return delta, err
	}
	exists := err == nil
	values := []*int64{report.Totals.Tokens, report.Totals.ActiveMS, report.Totals.WaitMS}
	if exists {
		at, e := time.Parse(time.RFC3339Nano, observed)
		if e != nil {
			return delta, domain.Fail("STORAGE_CORRUPT", "host_usage", "saved source timestamp is invalid")
		}
		if report.ObservedThrough.Before(at) {
			return delta, domain.Fail("HOST_USAGE_REGRESSION", "host_usage.observed_through", "retain the newest source sample; older observations cannot replace it")
		}
		if report.ObservedThrough.Equal(at) {
			equal := coverage == report.Coverage
			for i, v := range values {
				equal = equal && (v == nil && !old[i].Valid || v != nil && old[i].Valid && *v == old[i].Int64)
			}
			if !equal {
				return delta, domain.Fail("HOST_USAGE_CONFLICT", "host_usage", "the same source observation time already has different totals or coverage")
			}
			return delta, nil
		}
	}
	var totals [3]int64
	if err := tx.QueryRowContext(ctx, "SELECT tokens,host_ms,wait_ms FROM tasks WHERE task_id=?", taskID).Scan(&totals[0], &totals[1], &totals[2]); err != nil {
		return delta, err
	}
	for i, value := range values {
		if old[i].Valid && (value == nil || *value < old[i].Int64) {
			return delta, domain.Fail("HOST_USAGE_REGRESSION", "host_usage.totals", "known source totals cannot decrease or become unknown; retain measured amounts and mark incomplete coverage partial")
		}
		if value != nil {
			delta[i] = *value - old[i].Int64
			if delta[i] > math.MaxInt64-totals[i] {
				return delta, domain.Invalid("host_usage.totals", "cumulative usage exceeds the supported integer range")
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO host_usage_sources(task_id,source_id,observed_through,tokens,active_ms,wait_ms,coverage) VALUES (?,?,?,?,?,?,?)
	ON CONFLICT(task_id,source_id) DO UPDATE SET observed_through=excluded.observed_through,tokens=excluded.tokens,active_ms=excluded.active_ms,wait_ms=excluded.wait_ms,coverage=excluded.coverage`, taskID, report.SourceID, report.ObservedThrough.Format(time.RFC3339Nano), report.Totals.Tokens, report.Totals.ActiveMS, report.Totals.WaitMS, report.Coverage)
	return delta, err
}

func hostUsage(ctx context.Context, q query, taskID string) (domain.HostUsageSummary, error) {
	result := domain.HostUsageSummary{Coverage: "unreported"}
	rows, err := q.QueryContext(ctx, "SELECT observed_through,tokens,active_ms,wait_ms,coverage FROM host_usage_sources WHERE task_id=? ORDER BY source_id", taskID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	allReported, anyKnown := true, false
	var totals [3]int64
	var known [3]bool
	for rows.Next() {
		var observed, coverage string
		var values [3]sql.NullInt64
		if err := rows.Scan(&observed, &values[0], &values[1], &values[2], &coverage); err != nil {
			return result, err
		}
		at, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil || coverage != "reported" && coverage != "partial" && coverage != "unreported" {
			return result, domain.Fail("STORAGE_CORRUPT", "host_usage", "saved Host usage source is invalid")
		}
		if result.ObservedThrough == nil || result.ObservedThrough.Before(at) {
			result.ObservedThrough = &at
		}
		allReported = allReported && coverage == "reported"
		for i, v := range values {
			if v.Valid {
				if v.Int64 < 0 || v.Int64 > math.MaxInt64-totals[i] {
					return result, domain.Fail("STORAGE_CORRUPT", "host_usage", "saved Host totals are invalid")
				}
				totals[i] += v.Int64
				known[i], anyKnown = true, true
			}
			if coverage == "reported" && !v.Valid || coverage == "unreported" && v.Valid {
				return result, domain.Fail("STORAGE_CORRUPT", "host_usage", "saved coverage disagrees with its metrics")
			}
		}
	}
	for i, dest := range []**int64{&result.Tokens, &result.ActiveMS, &result.WaitMS} {
		if known[i] {
			*dest = &totals[i]
		}
	}
	if anyKnown {
		result.Coverage = "partial"
		if allReported {
			result.Coverage = "reported"
		}
	}
	return result, rows.Err()
}
