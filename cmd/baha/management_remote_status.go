package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// remoteApplicationStatus is an explicit dev-only, named-application
// projection. No remote Target is ever read from local Core runtime files.
func remoteApplicationStatus(ctx context.Context, target deployment.ResolvedTarget, args []string, out io.Writer) error {
	filtered, environment, err := extractApplicationEnvironment(args, "app status")
	if err != nil {
		return err
	}
	if len(filtered) != 1 || filtered[0] == "" {
		return usageError("remote app status needs an explicit application NAME",
			"Use baha app status NAME --environment dev while connected to the remote Core.")
	}
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" {
		return usageError("remote app status currently supports the dev operator session only",
			"Select --environment dev; non-dev remote sessions require separately verified OIDC scoping.")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return machine.Wrap(machine.ErrorAuthenticationFailed, err,
			"Configure the pinned remote Core profile and authenticate for this Target.", false)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return fmt.Errorf("create remote status request key: %w", err)
	}
	selectors := machine.OperationContext{
		Application: filtered[0], Environment: environment, Target: target.Name,
	}
	admitted, err := core.requestMachineExecution(ctx, "status", selectors, nil, hex.EncodeToString(id))
	if err != nil {
		return err
	}
	result, err := core.confirmRemoteExecutionSuccess(ctx, admitted)
	if err != nil {
		return err
	}
	if !json.Valid(result) {
		return errors.New("remote Core status result is invalid JSON")
	}
	var formatted json.RawMessage
	if err := json.Unmarshal(result, &formatted); err != nil {
		return errors.New("remote Core status result cannot be decoded")
	}
	return writeJSON(out, formatted)
}
