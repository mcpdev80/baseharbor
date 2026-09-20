package providerconformance

import (
	"context"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
)

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

type Report struct {
	Provider capability.ProviderKind `json:"provider"`
	Status   Status                  `json:"status"`
	Checks   []Check                 `json:"checks"`
}

type Target struct {
	Descriptor       capability.IntegrationDescriptor
	Request          capability.Request
	Application      string
	UnsupportedScope capability.ProviderScope
}

type StateDigester interface {
	ConformanceStateDigest() string
}

type Destroyer interface {
	ConformanceDestroy(context.Context, capability.Resource) error
}

type Drifter interface {
	ConformanceDrift()
}

func Run(ctx context.Context, target Target) Report {
	report := Report{Provider: target.Descriptor.Provider.Kind, Status: Pass}
	add := func(name string, err error) {
		check := Check{Name: name, Status: Pass}
		if err != nil {
			check.Status = Fail
			check.Message = err.Error()
			report.Status = Fail
		}
		report.Checks = append(report.Checks, check)
	}

	add("provider-contract", target.Descriptor.Validate())
	if report.Status == Fail {
		return report
	}
	if target.Request.Driver == nil {
		add("driver", fmt.Errorf("provider driver is required"))
		return report
	}
	if target.Request.Driver.Descriptor().Kind != target.Descriptor.Provider.Kind {
		add("driver-descriptor", fmt.Errorf("driver provider %q does not match descriptor %q", target.Request.Driver.Descriptor().Kind, target.Descriptor.Provider.Kind))
		return report
	}
	add("driver-descriptor", nil)

	if target.UnsupportedScope != "" {
		placement := capability.ProviderPlacement{Scope: target.UnsupportedScope, Ownership: capability.OwnershipBaseHarbor}
		if target.UnsupportedScope == capability.ScopeExternal {
			placement.Ownership = capability.OwnershipExternal
			placement.ExternalReference = "conformance-external"
		}
		if err := capability.ValidateProviderPlacement(target.Descriptor, placement); err == nil {
			add("unsupported-placement-fails-closed", fmt.Errorf("unsupported placement %q was accepted", target.UnsupportedScope))
		} else {
			add("unsupported-placement-fails-closed", nil)
		}
	}

	before := digest(target.Request.Driver)
	execution, _, err := capability.Prepare(ctx, target.Application, []capability.Request{target.Request})
	add("preflight", err)
	if err != nil {
		return report
	}
	after := digest(target.Request.Driver)
	if before != "" && after != before {
		add("preflight-side-effect-free", fmt.Errorf("provider state changed during preflight"))
	} else {
		add("preflight-side-effect-free", nil)
	}

	result, err := execution.ProvisionAndBind(ctx)
	add("provision-bind", err)
	if err != nil {
		return report
	}
	if result.Status == capability.StatusFailed {
		add("binding-contract", fmt.Errorf("lifecycle reported failed binding"))
		return report
	}
	add("binding-contract", nil)

	_, err = execution.Verify(ctx)
	add("verify-readiness", err)
	if err != nil {
		return report
	}
	first := digest(target.Request.Driver)

	execution, _, err = capability.Prepare(ctx, target.Application, []capability.Request{target.Request})
	if err == nil {
		_, err = execution.ProvisionAndBind(ctx)
	}
	if err == nil {
		_, err = execution.Verify(ctx)
	}
	if err != nil {
		add("repeated-convergence", err)
	} else if first != "" && digest(target.Request.Driver) != first {
		add("repeated-convergence", fmt.Errorf("stable provider state changed on repeated convergence"))
	} else {
		add("repeated-convergence", nil)
	}

	return report
}

func Require(report Report) error {
	if report.Status == Pass {
		return nil
	}
	var failed []string
	for _, check := range report.Checks {
		if check.Status == Fail {
			failed = append(failed, check.Name+": "+check.Message)
		}
	}
	return fmt.Errorf("provider conformance failed: %s", strings.Join(failed, "; "))
}

func digest(driver capability.Driver) string {
	if state, ok := driver.(StateDigester); ok {
		return state.ConformanceStateDigest()
	}
	return ""
}
