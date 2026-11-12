package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func reconcileRemoteDevelopmentCredentials(ctx context.Context, target deployment.ResolvedTarget, credentialsUsername, credentialsPassword string, reset bool) error {
	if target.AccessProvider == "" || target.AccessProvider == "local" || credentialsUsername == "" || credentialsPassword == "" {
		return errors.New("remote developer credential reconciliation requires a remote Target and protected credentials")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return machine.Wrap(machine.ErrorAuthenticationFailed, err, "Configure and authenticate the pinned remote Core connection for this Target.", false)
	}
	input, err := json.Marshal(machineHTTPDevelopmentCredentialsInput{
		Target: target.Name, Username: credentialsUsername, Password: credentialsPassword, Reset: reset,
	})
	if err != nil {
		return errors.New("encode protected remote developer credential request")
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return errors.New("generate remote developer credential request identity")
	}
	key := hex.EncodeToString(keyBytes)
	selectors := machine.OperationContext{Target: target.Name, Environment: "dev"}
	admitted, err := core.requestMachineExecution(ctx, "dev.credentials", selectors, input, key)
	if err != nil {
		return err
	}
	observationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, readErr := core.confirmRemoteExecutionSuccess(observationCtx, admitted)
		if readErr == nil {
			var receipt struct {
				ContractVersion string `json:"contract_version"`
				Target          string `json:"target"`
				Environment     string `json:"environment"`
				Username        string `json:"username"`
				ProtectedFile   string `json:"protected_file"`
				Password        string `json:"password"`
			}
			decoder := json.NewDecoder(bytes.NewReader(result))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF ||
				receipt.ContractVersion != machine.ContractVersion || receipt.Target != target.Name ||
				receipt.Environment != "dev" || receipt.Username != credentialsUsername ||
				receipt.ProtectedFile == "" || receipt.Password != "" {
				return errors.New("remote Core developer credential receipt is invalid or exposes secret material")
			}
			return nil
		}
		if !errors.Is(readErr, errRemoteExecutionPending) {
			return readErr
		}
		select {
		case <-observationCtx.Done():
			return errors.New("remote Core developer credential operation did not complete; inspect the existing execution before retrying")
		case <-ticker.C:
		}
	}
}
