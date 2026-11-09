package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// remoteCreateTargetManagedApplication registers target-managed Application
// Intent on the verified Core. It does not transfer workstation source files:
// repository-owned sources require a separate authorized source admission.
func remoteCreateTargetManagedApplication(ctx context.Context, target deployment.ResolvedTarget, intent application.Manifest, selected ...*development.SourceModel) (applicationAdoptionResult, error) {
	if len(selected) > 1 {
		return applicationAdoptionResult{}, errors.New("only one source model may be supplied")
	}
	var source *development.SourceModel
	if len(selected) == 1 {
		source = selected[0]
	}
	if source != nil {
		if source.Application != intent.Name {
			return applicationAdoptionResult{}, errors.New("remote source model belongs to another application")
		}
		if err := development.ValidateImmutableRemoteSources(*source); err != nil {
			return applicationAdoptionResult{}, err
		}
	}
	if intent.ApplicationID != "" || strings.TrimSpace(intent.Name) == "" || strings.TrimSpace(intent.Environment) == "" {
		return applicationAdoptionResult{}, errors.New("remote app create requires explicit intent without a client-assigned application ID")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return applicationAdoptionResult{}, machine.Wrap(machine.ErrorAuthenticationFailed, err,
			"Configure and authenticate the pinned remote Core connection for this Target.", false)
	}
	raw, err := json.Marshal(machineApplicationCreateInput{Target: target.Name, Intent: intent, SourceModel: source})
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	randomKey := make([]byte, 16)
	if _, err := rand.Read(randomKey); err != nil {
		return applicationAdoptionResult{}, fmt.Errorf("generate remote create request identity: %w", err)
	}
	key := hex.EncodeToString(randomKey)
	selectors := machine.OperationContext{Target: target.Name, Application: intent.Name, Environment: intent.Environment}
	admitted, err := core.requestMachineExecution(ctx, "app.create", selectors, raw, key)
	if err != nil {
		return applicationAdoptionResult{}, fmt.Errorf(
			"remote app create request identity %s has unknown outcome; inspect the Core before any retry: %w", key, err)
	}

	// Admission is not creation success. Poll only the read operation so an
	// ambiguous response cannot cause a second identity allocation.
	observationCtx, cancel := context.WithTimeout(ctx, 31*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var result json.RawMessage
	for {
		result, err = core.confirmRemoteExecutionSuccess(observationCtx, admitted)
		if err == nil {
			break
		}
		if !errors.Is(err, errRemoteExecutionPending) {
			return applicationAdoptionResult{}, err
		}
		select {
		case <-observationCtx.Done():
			return applicationAdoptionResult{}, fmt.Errorf("remote app creation %s is not terminal; inspect before retry: %w",
				admitted.ExecutionID, observationCtx.Err())
		case <-ticker.C:
		}
	}

	var created applicationAdoptionResult
	if err := json.Unmarshal(result, &created); err != nil ||
		created.Application != intent.Name || created.Environment != intent.Environment ||
		strings.TrimSpace(created.ApplicationID) == "" || strings.TrimSpace(created.Manifest) == "" {
		return applicationAdoptionResult{}, errors.New("remote Core app creation receipt is invalid or belongs to another application")
	}
	return created, nil
}
