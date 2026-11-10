package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type DynamicReport struct {
	State                string `json:"state"`
	Runs                 int    `json:"runs"`
	Confirmed            bool   `json:"confirmed"`
	Rollback, Kept, Gaps []string
	Details              json.RawMessage `json:"details"`
}
type VerificationInput struct {
	Hash, Content, Semantic, Tool, Environment         string
	Cases                                              []string
	Set                                                string
	Included, Excluded                                 int
	Complete, Validation, Unstable, EnvironmentChanged bool
	Reason, Suggestion                                 string
}
type CalibrationLabel struct {
	ID, ProjectID, IssueID, Kind, Reason string
	Signals                              map[string]float64
	Accepted                             bool
}

func (s *Store) AnalysisCache(ctx context.Context, kind, key string, target any) (bool, error) {
	var body storedBody
	found := false
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		err := q.QueryRowContext(ctx, "SELECT encoding,raw_bytes,sha256,body FROM analysis_cache_v12 WHERE kind=? AND cache_key=?", kind, key).Scan(&body.Encoding, &body.RawBytes, &body.SHA256, &body.Body)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil || !found {
		return found, err
	}
	raw, err := body.decode(ctx)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, target)
}

func (s *Store) SaveAnalysisCache(ctx context.Context, kind, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body, err := prepareBody(ctx, raw)
	if err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO analysis_cache_v12 VALUES(?,?,?,?,?,?) ON CONFLICT(kind,cache_key) DO NOTHING`, kind, key, body.Encoding, body.RawBytes, body.SHA256, body.Body)
		return err
	})
}

// 两种文件缓存按相同内容键互为替代；命中任一种就可以省去整份文件的编码。
func (s *Store) HasAnalysisCache(ctx context.Context, key, kind, alternate string) (bool, error) {
	var found bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM analysis_cache_v12 WHERE cache_key=? AND kind IN (?,?))", key, kind, alternate).Scan(&found)
	})
	return found, err
}

func (s *Store) AnalysisRecord(ctx context.Context, task, version, kind string, target any) (bool, error) {
	var body storedBody
	found := false
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		err := q.QueryRowContext(ctx, "SELECT data,encoding,raw_bytes,sha256 FROM analysis_records_v12 WHERE task_id=? AND input_version=? AND kind=?", task, version, kind).Scan(&body.Body, &body.Encoding, &body.RawBytes, &body.SHA256)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil || !found {
		return found, err
	}
	raw := body.Body
	if body.Encoding != "" {
		raw, err = body.decode(ctx)
		if err != nil {
			return false, err
		}
	}
	return true, json.Unmarshal(raw, target)
}

func (s *Store) SaveAnalysisRecord(ctx context.Context, task, version, kind, state string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body, err := prepareBody(ctx, raw)
	if err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO analysis_records_v12(task_id,input_version,kind,state,data,encoding,raw_bytes,sha256)
 VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(task_id,input_version,kind) DO UPDATE SET state=excluded.state,data=excluded.data,
 encoding=excluded.encoding,raw_bytes=excluded.raw_bytes,sha256=excluded.sha256`, task, version, kind, state, body.Body, body.Encoding, body.RawBytes, body.SHA256)
		return err
	})
}

func (s *Store) Calibration(ctx context.Context, project string) ([]CalibrationLabel, error) {
	var out []CalibrationLabel
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT data FROM feedback_labels_v12 WHERE ?='' OR project_id=? ORDER BY id", project, project)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			var label CalibrationLabel
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(raw), &label); err != nil {
				return err
			}
			out = append(out, label)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) SaveCalibration(ctx context.Context, label CalibrationLabel) error {
	raw, err := json.Marshal(label)
	if err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO feedback_labels_v12 VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", label.ID, label.ProjectID, label.IssueID, string(raw))
		return err
	})
}
func (s *Store) SaveWeights(ctx context.Context, project string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO analysis_weights_v12 VALUES(?,?) ON CONFLICT(project_id) DO UPDATE SET data=excluded.data", project, string(raw))
		return err
	})
}

func (s *Store) UpdateVerificationInput(ctx context.Context, id string, scope VerificationInput, after, output string) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var raw string
		if err := tx.QueryRowContext(ctx, "SELECT data FROM verifications_v12 WHERE id=?", id).Scan(&raw); err != nil {
			return err
		}
		var r VerificationRecord
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return err
		}
		r.Scope, r.AfterTree, r.Output = scope, after, output
		return saveVerification(ctx, tx, r)
	})
}

func (r *DeliveryReport) UnmarshalJSON(data []byte) error {
	type plain DeliveryReport
	var p struct {
		plain
		TestRisk json.RawMessage `json:"test_risk"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = DeliveryReport(p.plain)
	if len(p.TestRisk) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, old := fields["test_risk"]; old {
		r.Historical = append([]byte(nil), data...)
	}
	return nil
}
