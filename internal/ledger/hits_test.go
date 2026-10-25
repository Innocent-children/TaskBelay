package ledger

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestHitsExportEvidenceBlocksAndCurrentHandlingWithoutNewChecks(t *testing.T) {
	s, actor, input, _, _ := checkFixture(t)
	requestFinishFixture(t, s, actor, input)
	check := saveCheckFixture(t, s, actor, input, []Issue{{Rule: "D3", At: "flag:extra", FactKey: "entry", Facts: []string{"新增未授权入口"}, Evidence: []string{"main.go:4"}, Level: "block", BlockOnDelivery: true, Status: "open"}})
	if delivered, err := s.Deliver(t.Context(), "w", actor, input.Tree, check); err != nil || len(delivered.Blocked) != 1 {
		t.Fatal(delivered, err)
	}
	issue := check.Issues[0]
	if err := s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		issue.Status = "resolved"
		return saveIssue(ctx, tx, input.TaskID, issue)
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Hits(t.Context(), "w", time.Now().Add(-14*24*time.Hour))
	if err != nil || len(hits) != 1 {
		t.Fatal(hits, err)
	}
	hit := hits[0]
	if hit.Issue.Status != "resolved" || len(hit.Blocks) != 1 || len(hit.Observations) != 1 || hit.Blocks[0].Actor != (Actor{Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID}) || len(hit.Issue.Evidence) != 1 || hit.Observations[0].InputVersion != check.InputVersion {
		t.Fatal(hit)
	}
	if empty, err := s.Hits(t.Context(), "w", time.Now().Add(time.Hour)); err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	if empty, err := s.Hits(t.Context(), "another-workspace", time.Time{}); err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM checks").Scan(&count); err != nil || count != 1 {
		t.Fatal("export reran check", count, err)
	}
}
