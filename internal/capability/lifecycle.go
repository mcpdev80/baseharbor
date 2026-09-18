package capability

import (
	"context"
	"fmt"
	"strings"
)

type Phase string

const (
	PhaseResolve   Phase = "resolve"
	PhasePreflight Phase = "preflight"
	PhaseApply     Phase = "apply"
	PhaseBind      Phase = "bind"
	PhaseVerify    Phase = "verify"
)

type Status string

const (
	StatusReady  Status = "ready"
	StatusFailed Status = "failed"
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityError Severity = "error"
)

type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type Binding struct {
	Resource Resource `json:"resource"`
	Workload string   `json:"workload"`
}

type PlanItem struct {
	Resource Resource `json:"resource"`
	Binding  Binding  `json:"binding"`
}

type Plan struct {
	Application string     `json:"application"`
	Items       []PlanItem `json:"items"`
}

type StepResult struct {
	Phase       Phase        `json:"phase"`
	Status      Status       `json:"status"`
	Resource    Resource     `json:"resource"`
	Binding     Binding      `json:"binding"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Result struct {
	Application string       `json:"application"`
	Status      Status       `json:"status"`
	Plan        Plan         `json:"plan"`
	Steps       []StepResult `json:"steps"`
}

type Driver interface {
	Descriptor() Provider
	Preflight(context.Context, Resource, Binding) error
	Provision(context.Context, Resource) error
	Bind(context.Context, Resource, Binding) error
	Verify(context.Context, Resource, Binding) error
}

type Request struct {
	Requirement Requirement
	Workload    string
	Driver      Driver
}

func BuildPlan(application string, requests []Request) (Plan, error) {
	application = strings.TrimSpace(application)
	if application == "" {
		return Plan{}, fmt.Errorf("capability plan: application is required")
	}

	plan := Plan{Application: application}
	for _, request := range requests {
		if request.Driver == nil {
			return Plan{}, fmt.Errorf("capability plan for %s/%s: provider driver is required", application, request.Requirement.Name)
		}
		resource, err := Resolve(application, request.Requirement, request.Driver.Descriptor())
		if err != nil {
			return Plan{}, err
		}
		workload := strings.TrimSpace(request.Workload)
		if workload == "" {
			return Plan{}, fmt.Errorf("capability binding for %s/%s: workload is required", application, request.Requirement.Name)
		}
		binding := Binding{Resource: resource, Workload: workload}
		plan.Items = append(plan.Items, PlanItem{Resource: resource, Binding: binding})
	}
	return plan, nil
}

// Run executes one deterministic capability lifecycle. Every resource is
// resolved and every provider preflight is completed before the first
// provisioning mutation starts.
func Run(ctx context.Context, application string, requests []Request) (Result, error) {
	plan, err := BuildPlan(application, requests)
	if err != nil {
		return Result{Application: strings.TrimSpace(application), Status: StatusFailed}, err
	}
	result := Result{Application: plan.Application, Status: StatusReady, Plan: plan}

	for i, item := range plan.Items {
		if err := requests[i].Driver.Preflight(ctx, item.Resource, item.Binding); err != nil {
			step := failedStep(PhasePreflight, item, "provider-preflight-failed", err)
			result.Steps = append(result.Steps, step)
			result.Status = StatusFailed
			return result, fmt.Errorf("capability preflight failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		result.Steps = append(result.Steps, readyStep(PhasePreflight, item))
	}

	for i, item := range plan.Items {
		driver := requests[i].Driver
		if err := driver.Provision(ctx, item.Resource); err != nil {
			result.Steps = append(result.Steps, failedStep(PhaseApply, item, "provider-apply-failed", err))
			result.Status = StatusFailed
			return result, fmt.Errorf("capability apply failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		result.Steps = append(result.Steps, readyStep(PhaseApply, item))

		if err := driver.Bind(ctx, item.Resource, item.Binding); err != nil {
			result.Steps = append(result.Steps, failedStep(PhaseBind, item, "provider-bind-failed", err))
			result.Status = StatusFailed
			return result, fmt.Errorf("capability bind failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		result.Steps = append(result.Steps, readyStep(PhaseBind, item))

		if err := driver.Verify(ctx, item.Resource, item.Binding); err != nil {
			result.Steps = append(result.Steps, failedStep(PhaseVerify, item, "provider-verification-failed", err))
			result.Status = StatusFailed
			return result, fmt.Errorf("capability verification failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		result.Steps = append(result.Steps, readyStep(PhaseVerify, item))
	}

	return result, nil
}

func readyStep(phase Phase, item PlanItem) StepResult {
	return StepResult{Phase: phase, Status: StatusReady, Resource: item.Resource, Binding: item.Binding}
}

func failedStep(phase Phase, item PlanItem, code string, err error) StepResult {
	return StepResult{
		Phase:    phase,
		Status:   StatusFailed,
		Resource: item.Resource,
		Binding:  item.Binding,
		Diagnostics: []Diagnostic{{
			Code: code, Severity: SeverityError, Message: err.Error(),
		}},
	}
}
