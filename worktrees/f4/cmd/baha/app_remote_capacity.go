package main

import (
	"context"
	"io"

	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func remoteApplicationProjectRuntime(ctx context.Context, resolved resolvedApplication) (*targetsession.ProjectRuntime, error) {
	transport, scope, err := remoteApplicationTransport(ctx, resolved, targetsession.PoolFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return targetsession.NewProjectRuntime(transport, scope)
}

func runApplicationMemoryPreflight(ctx context.Context, in io.Reader, out io.Writer, resolved resolvedApplication, estimate hostresource.MemoryEstimate, mutating bool) error {
	if resolved.Target.AccessProvider == "" || resolved.Target.AccessProvider == "local" {
		return runHostMemoryPreflight(ctx, in, out, bhruntime.ProviderKind(resolved.Target.RuntimeProvider), estimate, mutating)
	}
	runtime, err := remoteApplicationProjectRuntime(ctx, resolved)
	if err != nil {
		return err
	}
	evidence, err := runtime.NodeMemory(ctx)
	if err != nil {
		return &machine.Error{Code: machine.ErrorRuntimeUnavailable, CauseCode: "node_memory_evidence_unavailable", Message: "Fresh memory evidence from the selected execution node is unavailable.", Resource: "node memory", Next: "Verify the selected Connector session and node memory evidence before retrying.", Cause: err}
	}
	return runMemoryEvidencePreflight(ctx, in, out, hostresource.MemoryEvidence{TotalBytes: evidence.TotalBytes, AvailableBytes: evidence.AvailableBytes, SwapTotalBytes: evidence.SwapTotalBytes, SwapFreeBytes: evidence.SwapFreeBytes}, estimate, mutating)
}
