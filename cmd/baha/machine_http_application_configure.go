package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func (e *bahaMachineExecutor) executeHTTPAppConfigure(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	var input machineApplicationConfigureInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	input.Name, err = bindHTTPSelector("application", operationContext.Application, input.Name)
	if err != nil {
		return nil, err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.SourceSnapshotDigest) == "" || e.sourceTransfers == nil {
		return nil, machine.NewError(machine.ErrorSourceMissing, "Remote application configuration needs the current verified repository source.", "Run baha app init from the matching application repository.", false)
	}
	manifest, root, err := e.sourceTransfers.Current(input.Target, input.Name, input.Environment)
	if err != nil || manifest.Digest != input.SourceSnapshotDigest {
		return nil, machine.NewError(machine.ErrorSourceMissing, "The selected application source snapshot is missing or no longer current.", "Upload the exact local repository source and retry app init.", false)
	}
	selection, err := application.ResolveRepositoryEnvironment(root, input.Environment)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorValidationFailed, err, "The uploaded application source needs a valid baseharbor.yaml.", false)
	}
	resolved, err := resolveApplication(withTargetOverride(ctx, input.Target), e.store, machineApplicationArgs(input.Name, input.Environment), "init")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	resolved, err = prepareVerifiedHTTPSource(ctx, resolved, selection)
	if err != nil {
		return nil, err
	}
	ctx = withVerifiedHTTPSource(withTargetOverride(ctx, input.Target), resolved)
	options := repositoryInitOptions{
		Hostname: strings.TrimSpace(input.Hostname),
		TLSMode:  strings.TrimSpace(input.TLSMode),
		CertDir:  strings.TrimSpace(input.CertificateDirectory),
		Yes:      true,
	}
	if err := runRepositoryRuntimeInitResolved(ctx, resolved, root, options, io.Discard); err != nil {
		return nil, err
	}
	state, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
	if err != nil || state.Hostname == "" || state.TLSMode == "" || string(state.RuntimeProvider) != resolved.Target.RuntimeProvider {
		return nil, machine.NewError(machine.ErrorVerificationFailed, "Core did not verify the configured application deployment inputs.", "Run baha app init again after inspecting the selected Target and Core state.", false)
	}
	return struct {
		ContractVersion string `json:"contract_version"`
		Operation       string `json:"operation"`
		Verified        bool   `json:"verified"`
		Target          string `json:"target"`
		Application     string `json:"application"`
		Environment     string `json:"environment"`
		Hostname        string `json:"hostname"`
		TLSMode         string `json:"tls_mode"`
	}{machine.ContractVersion, "app.configure", true, input.Target, input.Name, input.Environment, state.Hostname, state.TLSMode}, nil
}
