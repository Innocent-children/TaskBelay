package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestControlOverheadCountsUntaskedCallsAndCompaction(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 0; i < 300; i++ {
		id := string(rune('A' + i))
		if e = s.BeginControlCall(ctx, id, 1, "test-owner"); e != nil {
			t.Fatal(e)
		}
		if e = s.EndControlCall(ctx, id, 2); e != nil {
			t.Fatal(e)
		}
	}
	u, e := s.ControlCost(ctx, nil)
	if e != nil || u.Calls != 300 || u.WallMS != 600 || u.Pending != 0 {
		t.Fatal("control cost lost during compaction", u, e)
	}
	s.BeginControlCall(ctx, "uncertain", 1, "test-owner")
	_, e = s.db.ExecContext(ctx, "UPDATE control_calls SET started_at=? WHERE call_id='uncertain'", time.Now().Add(-25*time.Millisecond).UTC().Format(time.RFC3339Nano))
	if e != nil {
		t.Fatal(e)
	}
	u, e = s.ControlCost(ctx, nil)
	if e != nil || u.Calls != 301 || u.Pending != 1 {
		t.Fatal("uncertain control call disappeared", u, e)
	}
	u, e = s.ControlCost(ctx, func(int, string) bool { return false })
	if e != nil || u.Calls != 301 || u.Pending != 0 || u.WallMS != 600 || u.UnknownMS < 25 {
		t.Fatal("dead call must retain conservative uncertainty separately from measured wall time", u, e)
	}
	settled := u.UnknownMS
	u, e = s.ControlCost(ctx, func(int, string) bool { return false })
	if e != nil || u.UnknownMS != settled {
		t.Fatal("dead call was charged more than once", u, e)
	}
}
