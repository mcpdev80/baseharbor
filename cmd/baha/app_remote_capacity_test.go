package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestRemoteMemoryNeverUsesCoreHostWhenNodeIsUnbound(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	resolved := resolvedApplication{Target: node}
	for _, bound := range []bool{false, true} {
		check := ctx
		if bound {
			check = withCoreAuthority(ctx, core)
		}
		var out bytes.Buffer
		err := runApplicationMemoryPreflight(withMemoryPreflightOverride(withAssumeYes(check, true), true), strings.NewReader("y\n"), &out, resolved, hostresource.MemoryEstimate{}, true)
		if err == nil || out.Len() != 0 {
			t.Fatal("unbound node produced Core-host capacity evidence", err, out.String())
		}
		if bound && !errors.Is(err, targetsession.ErrUnavailable) {
			t.Fatal("missing session was not denied", err)
		}
	}
}

func TestRemoteRuntimeBindingRejectsChangedNodeSelection(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	node.AccessReference = "foreign-node"
	_, err := remoteApplicationProjectRuntime(withCoreAuthority(ctx, core), resolvedApplication{Target: node})
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
		t.Fatal("changed node binding admitted", err)
	}
}

func TestSelectedMemoryEvidenceFailsClosedEvenWithApproval(t *testing.T) {
	ctx, _, _ := remoteApplicationCoreFixture(t)
	var out bytes.Buffer
	err := runMemoryEvidencePreflight(withMemoryPreflightOverride(withAssumeYes(ctx, true), true), strings.NewReader("y\n"), &out,
		hostresource.MemoryEvidence{TotalBytes: 1024, AvailableBytes: 0}, hostresource.MemoryEstimate{Confidence: hostresource.ConfidenceUnknown}, true)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorHostResourceInsufficient {
		t.Fatal("exhausted selected node capacity was approved", err)
	}
}
