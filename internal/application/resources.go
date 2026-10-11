package application

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Keep evidence for different contexts so restoring checked code can reuse it.
// When the hot bound is reached, preserve every currently applicable result and
// fill remaining slots from recent history; lifetime runs and costs stay in SQL.
func (s *Service) boundEvidence(ctx context.Context, t *domain.Task) error {
	if len(t.Evidence) <= 192 {
		return nil
	}
	v, e := s.view(ctx, *t, true)
	if e != nil {
		return e
	}
	protected := map[string]bool{}
	for _, f := range v.Decision.Evidence {
		if f.Valid {
			protected[f.EvidenceID] = true
		}
	}
	retained := []domain.Evidence{}
	for _, record := range t.Evidence {
		if protected[record.ID] {
			retained = append(retained, record)
		}
	}
	if len(retained) > 192 {
		return domain.Fail("RESOURCE_LIMIT", "evidence", "applicable evidence exceeds the current snapshot bound")
	}
	for i := len(t.Evidence) - 1; i >= 0 && len(retained) < 192; i-- {
		if !protected[t.Evidence[i].ID] {
			retained = append(retained, t.Evidence[i])
		}
	}
	slices.SortStableFunc(retained, func(a, b domain.Evidence) int { return a.RecordedAt.Compare(b.RecordedAt) })
	t.Evidence = retained
	return nil
}

// compactReceipts removes only retired, owned execution artifacts after SQLite commit.
func (s *Service) compactReceipts(ctx context.Context) error {
	root := filepath.Join(s.Store.Directory, "receipts")
	entries, e := os.ReadDir(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasPrefix(id, "run-") || !domain.ID(id).IsValid() {
			continue
		}
		exists, e := s.Store.HasRun(ctx, id)
		if e != nil {
			return e
		}
		if exists {
			continue
		}
		p := filepath.Join(root, entry.Name())
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		var v struct {
			Run struct {
				RunID string `json:"run_id"`
			} `json:"run"`
		}
		if json.Unmarshal(b, &v) != nil || v.Run.RunID != id || !execution.OwnedReceipt(s.Store.Directory, id, b) {
			continue
		}
		if e = os.Remove(p); e != nil {
			return e
		}
	}
	return nil
}
