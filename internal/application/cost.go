package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/execution"
)

// MeasureControl reserves every control entry, including untasked Host/API reads.
// The returned completion records measured time; crashes leave durable uncertainty.
func (s *Service) MeasureControl(ctx context.Context) (func(), error) {
	start := time.Now()
	if _, e := s.ControlCost(ctx); e != nil {
		var meterError *ControlMeterError
		if errors.As(e, &meterError) {
			return nil, e
		}
		return nil, &ControlMeterError{Stage: "storage_read", Err: e}
	}
	pid, owner, e := execution.Owner()
	if e != nil {
		return nil, &ControlMeterError{Stage: "process_identity", Err: e}
	}
	var b [16]byte
	if _, e = rand.Read(b[:]); e != nil {
		return nil, &ControlMeterError{Stage: "operation_identity", Err: e}
	}
	id := hex.EncodeToString(b[:])
	if e = s.Store.BeginControlCall(ctx, id, pid, owner); e != nil {
		return nil, &ControlMeterError{Stage: "storage_write", Err: e}
	}
	return func() { _ = s.Store.EndControlCall(context.WithoutCancel(ctx), id, time.Since(start).Milliseconds()) }, nil
}

// The phase belongs to the failing owner; transports preserve the actual cause.
type ControlMeterError struct {
	Stage string
	Err   error
}

func (e *ControlMeterError) Error() string {
	return fmt.Sprintf("control meter %s: %v", e.Stage, e.Err)
}
func (e *ControlMeterError) Unwrap() error { return e.Err }
