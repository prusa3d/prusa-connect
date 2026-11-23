package resource

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/extension/supervisor"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// SwitchOff is the record of the host switching a service module off because
// it failed too often inside the failure window, with the stops that caused
// it.
//
// The overlay stores it in the same JSON form the API returns, so the file
// and a read of the instance have the same keys.
type SwitchOff struct {
	At    time.Time
	Stops []supervisor.Stop
}

// SwitchOffByHost records that the supervisor stopped retrying a module. An
// instance the overlay holds is switched off in the overlay and stays off
// across restarts. The store does not write the operator's file, so an
// instance declared there is kept off in memory until the server restarts. An
// instance that is already off, or was removed, is not changed.
//
// It reads the inputs as a write does and checks no grant, because the host
// writes and no principal does. If a person saved the configuration during
// the write, the write is refused with a version conflict, and it is made
// once more from a new read.
func (s *Store) SwitchOffByHost(ctx context.Context, instance ref.Ref, record SwitchOff) error {
	err := s.switchOffByHost(ctx, instance, record)
	if fault.HasCode(err, httpfault.CodeVersionConflict()) {
		err = s.switchOffByHost(ctx, instance, record)
	}

	return err
}

func (s *Store) switchOffByHost(ctx context.Context, instance ref.Ref, record SwitchOff) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, held := s.hostHeld[instance]; held {
		return nil
	}

	read, err := readInputs(s.paths)
	if err != nil {
		return s.internal("read the configuration", err)
	}

	doc, err := document(read)
	if err != nil {
		return s.internal("parse the configuration", err)
	}

	declared, found := doc.Find(instance)
	if !found || !declared.Spec.Enabled {
		return nil
	}

	warn := func() {
		s.log.WarnContext(ctx, "the host switched a module off because it failed too often",
			"resource", instance.String(), "results", resultsOf(record.Stops), "source", declared.Source.String())
	}

	if declared.Source == SourceFile {
		previous := s.effective(s.current)
		s.hostHeld[instance] = record
		s.syncModuleOrigins(ctx)
		warn()

		if plan := Diff(previous, s.effective(s.current)); !plan.Empty() && s.plane != nil {
			if err = s.plane.Reconcile(ctx, plan); err != nil {
				s.reportRefused(ctx, err)
			}
		}

		return nil
	}

	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return s.internal("parse the overlay", err)
	}

	spec := declared.Spec
	spec.Enabled = false
	spec.SwitchedOff = &record

	if _, err = s.commit(ctx, withInstance(overlay, instance, declared.UID, spec), read); err != nil {
		return err
	}

	warn()

	return nil
}

// releaseHeld removes every hold the operator has acted on: the instance was
// removed from the operator's file, is declared in the overlay, or is
// switched off in the file. Switching it on again then starts it.
func (s *Store) releaseHeld(next Document) {
	for instance := range s.hostHeld {
		declared, found := next.Find(instance)
		if !found || declared.Source != SourceFile || !declared.Spec.Enabled {
			delete(s.hostHeld, instance)
		}
	}
}

// withoutHeld is doc with every instance the host holds off switched off. Its
// declaration still says enabled, because the operator's file says so.
func (s *Store) withoutHeld(doc Document) Document {
	if len(s.hostHeld) == 0 {
		return doc
	}

	masked := doc
	masked.Instances = slices.Clone(doc.Instances)

	for i := range masked.Instances {
		if _, held := s.hostHeld[masked.Instances[i].Ref]; held {
			masked.Instances[i].Spec.Enabled = false
		}
	}

	return masked
}

// withSwitchOff writes the host's switch-off into a service module's
// projection, after withBudget has given it a memory block.
func (s *Store) withSwitchOff(instance *httpapi.ResourceInstance, declared Declaration) {
	memory := instance.Status.Memory
	if memory == nil {
		return
	}

	record, held := s.hostHeld[declared.Ref]
	if declared.Spec.SwitchedOff != nil {
		record = *declared.Spec.SwitchedOff
	}

	if held || declared.Spec.SwitchedOff != nil {
		wire := record.wire()
		memory.SwitchedOff = &wire
	}

	memory.HeldUntilRestart = held
}

func resultsOf(stops []supervisor.Stop) []string {
	results := make([]string, 0, len(stops))
	for _, stop := range stops {
		results = append(results, stop.Result.String())
	}

	return results
}

// MarshalJSON writes the record in the API's shape.
func (r SwitchOff) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.wire())
}

// UnmarshalJSON reads the record from the API's shape.
func (r *SwitchOff) UnmarshalJSON(data []byte) error {
	var wire httpapi.ResourceSwitchOff
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	*r = SwitchOff{At: wire.At, Stops: make([]supervisor.Stop, 0, len(wire.Stops))}

	for _, stop := range wire.Stops {
		r.Stops = append(r.Stops, stopFromWire(stop))
	}

	return nil
}

func (r SwitchOff) wire() httpapi.ResourceSwitchOff {
	wire := httpapi.ResourceSwitchOff{At: r.At, Stops: make([]httpapi.ResourceStop, 0, len(r.Stops))}
	for _, stop := range r.Stops {
		wire.Stops = append(wire.Stops, StopToWire(stop))
	}

	return wire
}

// StopToWire converts a stop to the API type. It sets only the fields that
// belong to the stop's result.
func StopToWire(stop supervisor.Stop) httpapi.ResourceStop {
	wire := httpapi.ResourceStop{Result: stop.Result.String(), At: stop.At}

	switch stop.Result { //nolint:exhaustive // the other results carry no field of their own
	case supervisor.ResultOverMemory:
		wire.UsedBytes, wire.BudgetBytes = &stop.Used, &stop.Budget
	case supervisor.ResultExited:
		wire.Code = &stop.Code
	case supervisor.ResultSignal:
		wire.Signal = stop.Signal
	}

	return wire
}

func stopFromWire(wire httpapi.ResourceStop) supervisor.Stop {
	stop := supervisor.Stop{Result: supervisor.Result(wire.Result), At: wire.At, Signal: wire.Signal}

	if wire.UsedBytes != nil {
		stop.Used = *wire.UsedBytes
	}

	if wire.BudgetBytes != nil {
		stop.Budget = *wire.BudgetBytes
	}

	if wire.Code != nil {
		stop.Code = *wire.Code
	}

	return stop
}
