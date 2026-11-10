package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// remoteMachineRead executes existing read-only Machine operations on the
// selected Core. It keeps repository planning and diagnostics on the machine
// that owns the application runtime instead of trying to open a local or
// Connector runtime on the workstation.
func remoteMachineRead(ctx context.Context, target deployment.ResolvedTarget, operation, application, environment string, input any) (json.RawMessage, error) {
	switch operation {
	case "plan", "status", "doctor", "app.list":
	default:
		return nil, fmt.Errorf("unsupported remote read operation %q", operation)
	}
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" {
		return nil, machine.NewError(machine.ErrorPolicyDenied, "Remote application reads currently require the dev operator session.", "Select --environment dev; non-dev remote sessions require separately verified OIDC scoping.", false)
	}
	if operation != "app.list" && application == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Remote application read needs an explicit application name.", "Pass the application name or run inside a repository with baseharbor.yaml.", false)
	}
	var payload []byte
	if input == nil {
		payload = []byte("{}")
	} else {
		var err error
		payload, err = json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("encode remote read request: %w", err)
		}
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorAuthenticationFailed, err, "Configure and authenticate the pinned remote Core profile for this Target.", false)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("create remote read request key: %w", err)
	}
	selectors := machine.OperationContext{Application: application, Environment: environment, Target: target.Name}
	admitted, err := core.requestMachineExecution(ctx, operation, selectors, payload, hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, readErr := core.confirmRemoteExecutionSuccess(readCtx, admitted)
		if readErr == nil {
			if !json.Valid(result) {
				return nil, errors.New("remote Core read result is not valid JSON")
			}
			return result, nil
		}
		if !errors.Is(readErr, errRemoteExecutionPending) {
			return nil, readErr
		}
		select {
		case <-readCtx.Done():
			return nil, fmt.Errorf("remote Core %s read did not finish: %w", operation, readCtx.Err())
		case <-ticker.C:
		}
	}
}
