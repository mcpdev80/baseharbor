package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineHTTPDevelopmentCredentialsInput struct {
	Target   string `json:"target,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
	Reset    bool   `json:"reset,omitempty"`
}

func executeHTTPDevelopmentCredentials(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineHTTPDevelopmentCredentialsInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	if target == "" || operationContext.Environment != "dev" || strings.TrimSpace(input.Username) == "" || input.Password == "" || len(input.Password) > 64<<10 || strings.ContainsAny(input.Username, "\r\n=") || strings.ContainsAny(input.Password, "\r\n") {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Developer credentials are invalid or outside the authorized development Target.", "Provide one development Target credential over the verified Core connection.", false)
	}
	ctx = withTargetOverride(ctx, target)
	previous, previousErr := devaccess.Load(target, "dev")
	candidate := devaccess.Credentials{Username: strings.TrimSpace(input.Username), Password: input.Password}
	if !input.Reset && previousErr == nil && previous != candidate {
		return nil, machine.NewError(machine.ErrorConflict, "Target developer credentials differ from the Workstation's protected copy.", "Use 'baha dev credentials --reset' to rotate the Target credential explicitly.", false)
	}
	if input.Reset || errors.Is(previousErr, os.ErrNotExist) {
		if _, err := devaccess.Configure(target, "dev", candidate.Username, candidate.Password); err != nil {
			return nil, err
		}
	} else if previousErr != nil {
		return nil, previousErr
	}
	rollback := func() {
		if previousErr == nil {
			_, _ = devaccess.Configure(target, "dev", previous.Username, previous.Password)
		} else if path, pathErr := devaccess.Path(target, "dev"); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	authoritative, err := reconcileDeveloperCredentialAuthority(ctx, target, "dev", candidate, input.Reset)
	if err != nil {
		rollback()
		return nil, err
	}
	if authoritative != candidate {
		rollback()
		return nil, machine.NewError(machine.ErrorConflict, "The managed Target credential differs from the Workstation's protected copy.", "Use 'baha dev credentials --reset' to rotate the Target credential explicitly.", false)
	}
	path, err := devaccess.Path(target, "dev")
	if err != nil {
		rollback()
		return nil, err
	}
	return struct {
		ContractVersion string `json:"contract_version"`
		Target          string `json:"target"`
		Environment     string `json:"environment"`
		Username        string `json:"username"`
		ProtectedFile   string `json:"protected_file"`
	}{machine.ContractVersion, target, "dev", candidate.Username, path}, nil
}
