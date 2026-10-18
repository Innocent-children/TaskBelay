package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
)

/**
 * Reserve mutating calls before work. Saved-state reads remain independent of
 * process inspection and metering writes so recovery information stays accessible.
 */
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

/**
 * The phase belongs to the failing owner; transports preserve the actual cause.
 */
type ControlMeterError struct {
	Stage string
	Err   error
}

func (e *ControlMeterError) Error() string {
	return fmt.Sprintf("control meter %s: %v", e.Stage, e.Err)
}
func (e *ControlMeterError) Unwrap() error { return e.Err }

func (s *Service) SavedControlCost(ctx context.Context) (store.ControlUsage, error) {
	return s.Store.ControlCost(ctx, nil)
}

func (s *Service) ControlCost(ctx context.Context) (store.ControlUsage, error) {
	return s.controlCost(ctx, execution.OwnerStatus)
}

func (s *Service) controlCost(ctx context.Context, probe func(execution.Run) (bool, error)) (store.ControlUsage, error) {
	var ownerError error
	usage, err := s.Store.ControlCost(ctx, func(pid int, id string) bool {
		alive, queryErr := probe(execution.Run{OwnerPID: pid, OwnerIdentity: id})
		if queryErr != nil {
			ownerError = queryErr
			return true
		} // Query denial is not proof of death.
		return alive
	})
	if err != nil {
		return usage, err
	}
	if ownerError != nil {
		return usage, &ControlMeterError{Stage: "process_identity", Err: ownerError}
	}
	return usage, nil
}
