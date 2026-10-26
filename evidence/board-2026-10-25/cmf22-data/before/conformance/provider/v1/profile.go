// Package provider exposes the existing Provider Integration Contract harness
// to external Go modules. Behavioral profiles are independent of artifact trust.
package provider

import (
	"context"
	"encoding/json"
	"io"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
)

const ProfileID = "provider-contract/v1"
const ReportVersion = "baseharbor.provider-conformance/v1"

type Target = providerconformance.Target
type Check = providerconformance.Check
type Status = providerconformance.Status

const (
	Pass = providerconformance.Pass
	Fail = providerconformance.Fail
)

type Profile struct {
	ID                  string   `json:"id"`
	Contract            string   `json:"contract"`
	ReportVersion       string   `json:"report_version"`
	FixtureRequirements []string `json:"fixture_requirements"`
}

func Describe() Profile {
	return Profile{ID: ProfileID, Contract: capability.ProviderProtocolV1, ReportVersion: ReportVersion,
		FixtureRequirements: []string{"disposable application resource", "valid integration descriptor", "provider driver", "state fingerprint hook", "immutable application/resource identity", "full acceptance: typed reconciliation, drift/fault/ownership/destroy hooks"}}
}

type Report struct {
	SchemaVersion string `json:"schema_version"`
	Profile       string `json:"profile"`
	Contract      string `json:"contract"`
	Suite         string `json:"suite"`
	providerconformance.Report
}

// Run uses the original harness without replacing its lifecycle implementation.
// The caller must provide isolated test infrastructure; this provisions data.
func Run(ctx context.Context, target Target) Report {
	if _, ok := target.Request.Driver.(StateDigester); !ok {
		return Report{SchemaVersion: ReportVersion, Profile: ProfileID, Contract: ProtocolVersion, Suite: "lifecycle",
			Report: providerconformance.Report{Provider: target.Descriptor.Provider.Kind, Status: Fail,
				Checks: []Check{{Name: "fixture-state-fingerprint", Status: Fail, Message: "Implement ConformanceStateDigest to prove preflight and repeated-convergence state."}}}}
	}
	report := providerconformance.Run(ctx, target)
	for index := range report.Checks {
		if report.Checks[index].Status == Fail {
			// Provider error text may include connection strings or credentials.
			report.Checks[index].Message = "Conformance check failed; review the implementation and protected fixture diagnostics."
		}
	}
	return Report{SchemaVersion: ReportVersion, Profile: ProfileID, Contract: capability.ProviderProtocolV1, Suite: "lifecycle", Report: report}
}

// RunFull exercises the retained fault/recovery and ownership suite in addition
// to the core lifecycle. Missing fixture hooks fail; they never become skips.
func RunFull(ctx context.Context, target Target) Report {
	report := providerconformance.RunScenarios(ctx, target)
	for i := range report.Checks {
		if report.Checks[i].Status == Fail {
			report.Checks[i].Message = "Conformance check failed; review protected fixture diagnostics."
		}
	}
	return Report{SchemaVersion: ReportVersion, Profile: ProfileID, Contract: ProtocolVersion, Suite: "full", Report: report}
}

// WriteJSON emits one deterministic report without timestamps or terminal text.
func WriteJSON(out io.Writer, report Report) error { return json.NewEncoder(out).Encode(report) }

// ExitCode is suitable for the caller's conformance runner process.
func ExitCode(report Report) int {
	if report.Status == Pass {
		return 0
	}
	return 1
}
