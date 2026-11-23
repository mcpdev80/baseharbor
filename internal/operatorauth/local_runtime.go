package operatorauth

import (
	"errors"
	"sync/atomic"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/environment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// A local operation grant binds one successful runtime check to one policy
// decision. It prevents accidental internal reuse or substitution. It is not a
// security boundary against the local OS user, who controls this process; the
// OS user is the actual trust boundary for local runtime operations.
type localOperationGrant struct {
	operation     machine.Operation
	environmentID string
	applicationID string
	nonce         uint64
	used          atomic.Bool
}

// LocalOperationGrant is an opaque handle. Its unexported method prevents
// other packages from implementing or forging a grant value.
type LocalOperationGrant interface{ isLocalOperationGrant() }

func (*localOperationGrant) isLocalOperationGrant() {}

var localOperationNonce atomic.Uint64

// CheckLocalRuntimeGrant runs the local runtime check and, only on success,
// returns an opaque grant for one operation on one application/environment.
// The private grant type and fields prevent accidental construction or edits
// by callers. This deliberately does not attempt to defend against the local
// OS user, who can modify or replace this process.
func CheckLocalRuntimeGrant(operation machine.Operation, environmentID, applicationID string, check func() error) (LocalOperationGrant, error) {
	registered, ok := machine.OperationByID(operation.ID)
	if !ok || registered != operation || !LocalWorkloadOperation(operation.ID) {
		return nil, errors.New("local runtime grant requires a registered operation")
	}
	if environment.Validate(environmentID) != nil || application.ValidateApplicationID(applicationID) != nil || check == nil {
		return nil, errors.New("local runtime grant requires canonical identities and a runtime check")
	}
	if err := check(); err != nil {
		return nil, err
	}
	return &localOperationGrant{
		operation: operation, environmentID: environmentID,
		applicationID: applicationID, nonce: localOperationNonce.Add(1),
	}, nil
}

func LocalWorkloadOperation(operation string) bool {
	switch operation {
	case "app.create", "app.adopt", "app.configure", "app.new", "app.show", "app.environment", "app.connection", "app.preflight", "app.stop", "plan", "apply", "status", "doctor", "observe", "evidence", "update", "repair", "backup", "restore", "destroy", "workspace.show", "workspace.init", "workspace.map", "workspace.resolve", "workspace.status", "workspace.update":
		return true
	default:
		return false
	}
}

func localRuntimeGrantMatches(opaque LocalOperationGrant, request AuthorizationRequest) bool {
	grant, ok := opaque.(*localOperationGrant)
	if !ok || grant == nil || grant.nonce == 0 || grant.used.Load() {
		return false
	}
	return grant.operation == request.Operation &&
		grant.environmentID == request.EnvironmentID &&
		grant.environmentID == request.TargetEnvironmentID &&
		grant.applicationID != "" && grant.applicationID == request.ApplicationID &&
		grant.used.CompareAndSwap(false, true)
}
