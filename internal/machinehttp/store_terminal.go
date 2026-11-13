package machinehttp

import (
	"errors"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// completeEphemeral commits installation.destroy's terminal view only in
// memory. Destroy removes the journal directory, so persisting this result
// would recreate BaseHarbor state after successful cleanup.
func (s *executionStore) completeEphemeral(execution machine.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[execution.ExecutionID]
	if !ok || record.execution.OperationID != "installation.destroy" || execution.State != machine.ExecutionSucceeded && execution.State != machine.ExecutionFailed {
		return errors.New("installation destroy execution cannot be completed in memory")
	}
	record.execution = execution
	kind := machine.EventOperationSucceeded
	event := machine.MachineEvent{
		ExecutionID: execution.ExecutionID,
		OperationID: execution.OperationID,
		Actor:       &execution.Actor,
		Context:     &execution.Context,
		State:       execution.State,
		Result:      execution.Result,
		Error:       execution.Error,
	}
	if execution.State == machine.ExecutionFailed {
		kind = machine.EventOperationFailed
	}
	event.Kind = kind
	s.emitLocked(record, event)
	return nil
}
