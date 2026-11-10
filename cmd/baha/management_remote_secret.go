package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func remoteSetApplicationSecret(ctx context.Context, target deployment.ResolvedTarget, application, environment, key string, value []byte) (secretMutationResult, error) {
	if target.AccessProvider == "" || target.AccessProvider == "local" || strings.TrimSpace(application) == "" ||
		strings.TrimSpace(environment) == "" || strings.TrimSpace(key) == "" || len(value) == 0 || len(value) > maxRemoteApplicationSecretBytes {
		return secretMutationResult{}, errors.New("remote application secret mutation requires a remote Target, complete application scope, and a secret value within 700 KiB")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return secretMutationResult{}, machine.Wrap(machine.ErrorAuthenticationFailed, err,
			"Configure and authenticate the pinned remote Core connection for this Target.", false)
	}
	input, err := json.Marshal(machineHTTPApplicationSecretInput{
		Target: target.Name, Application: application, Environment: environment, Key: key,
		Value: append([]byte(nil), value...),
	})
	if err != nil {
		return secretMutationResult{}, errors.New("encode remote application secret request")
	}
	defer zeroBytes(input)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return secretMutationResult{}, errors.New("generate remote application secret request identity")
	}
	selectors := machine.OperationContext{Target: target.Name, Application: application, Environment: environment}
	admitted, err := core.requestMachineExecution(ctx, "secret.set", selectors, input, hex.EncodeToString(id))
	if err != nil {
		return secretMutationResult{}, err
	}

	observationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, readErr := core.confirmRemoteExecutionSuccess(observationCtx, admitted)
		if readErr == nil {
			var receipt secretMutationResult
			decoder := json.NewDecoder(bytes.NewReader(result))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || receipt.Application != application ||
				receipt.Environment != environment || receipt.Key != key || !receipt.Updated {
				return secretMutationResult{}, errors.New("remote Core secret receipt is invalid or exposes secret material")
			}
			return receipt, nil
		}
		if !errors.Is(readErr, errRemoteExecutionPending) {
			return secretMutationResult{}, readErr
		}
		select {
		case <-observationCtx.Done():
			return secretMutationResult{}, errors.New("remote Core application secret operation did not complete; inspect the existing execution before retrying")
		case <-ticker.C:
		}
	}
}
