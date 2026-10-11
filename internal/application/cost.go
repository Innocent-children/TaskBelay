package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/Innocent-children/taskbelay/internal/execution"
)

// MeasureControl reserves every control entry, including untasked Host/API reads.
// The returned completion records measured time; crashes leave durable uncertainty.
func (s *Service) MeasureControl(ctx context.Context) (func(), error) {
	start := time.Now()
	if _, e := s.ControlCost(ctx); e != nil {
		return nil, e
	}
	pid, owner, e := execution.Owner()
	if e != nil {
		return nil, e
	}
	var b [16]byte
	if _, e = rand.Read(b[:]); e != nil {
		return nil, e
	}
	id := hex.EncodeToString(b[:])
	if e = s.Store.BeginControlCall(ctx, id, pid, owner); e != nil {
		return nil, e
	}
	return func() { _ = s.Store.EndControlCall(context.WithoutCancel(ctx), id, time.Since(start).Milliseconds()) }, nil
}
