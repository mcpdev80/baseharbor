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

func remoteDevelopmentDomain(ctx context.Context, target deployment.ResolvedTarget, domain string) (string, error) {
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return "", err
	}
	defer core.client.CloseIdleConnections()
	input, err := json.Marshal(machineHTTPDevelopmentDomainInput{Target: target.Name, Domain: domain})
	if err != nil {
		return "", err
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	admitted, err := core.requestMachineExecution(ctx, "dev.domain", machine.OperationContext{Target: target.Name, Environment: "dev"}, input, hex.EncodeToString(key))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := core.confirmRemoteExecutionSuccess(ctx, admitted)
		if err == nil {
			var result machineHTTPDevelopmentDomainResult
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.ContractVersion != machine.ContractVersion || result.Target != target.Name || result.Environment != "dev" || result.Domain == "" {
				return "", errors.New("invalid remote development domain receipt")
			}
			if domain != "" && result.Domain != strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".") {
				return "", errors.New("Core did not apply the requested development domain")
			}
			return result.Domain, nil
		}
		if !errors.Is(err, errRemoteExecutionPending) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", errors.New("development domain result unknown; inspect the selected Core before retrying")
		case <-ticker.C:
		}
	}
}
