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

type HTTPExposureBinding struct {
	Service    string `json:"service"`
	TargetPort int    `json:"target_port"`
	Protocol   string `json:"protocol"`
	Visibility string `json:"visibility"`
}

type ObjectStorageS3Binding struct {
	Bucket string `json:"bucket"`
}

type OTLPTelemetryBinding struct {
	Direction string   `json:"direction"`
	Protocol  string   `json:"protocol"`
	Signals   []string `json:"signals"`
}

type Binding struct {
	Resource        Resource                `json:"resource"`
	Workload        string                  `json:"workload"`
	HTTPExposure    *HTTPExposureBinding    `json:"http_exposure,omitempty"`
	ObjectStorageS3 *ObjectStorageS3Binding `json:"object_storage_s3,omitempty"`
	TelemetryOTLP   *OTLPTelemetryBinding   `json:"telemetry_otlp,omitempty"`
	Security        *SecureBinding          `json:"security,omitempty"`
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
	Provision(context.Context, Resource, Binding) error
	Bind(context.Context, Resource, Binding) error
	Verify(context.Context, Resource, Binding) error
}

type Request struct {
	Requirement     Requirement
	Workload        string
	HTTPExposure    *HTTPExposureBinding
	ObjectStorageS3 *ObjectStorageS3Binding
	TelemetryOTLP   *OTLPTelemetryBinding
	Security        *SecureBinding
	Driver          Driver
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
		if request.HTTPExposure != nil {
			value := *request.HTTPExposure
			binding.HTTPExposure = &value
		}
		if request.ObjectStorageS3 != nil {
			value := *request.ObjectStorageS3
			if strings.TrimSpace(value.Bucket) == "" {
				return Plan{}, fmt.Errorf("capability S3 binding for %s/%s: bucket is required", application, request.Requirement.Name)
			}
			binding.ObjectStorageS3 = &value
		}
		if request.Requirement.Kind == TelemetryOTLP && request.TelemetryOTLP == nil {
			return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s is required", application, request.Requirement.Name)
		}
		if request.TelemetryOTLP != nil {
			value := *request.TelemetryOTLP
			value.Direction = strings.TrimSpace(value.Direction)
			value.Protocol = strings.TrimSpace(value.Protocol)
			if value.Direction != "export" {
				return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s: direction must be export", application, request.Requirement.Name)
			}
			if value.Protocol != "http/protobuf" {
				return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s: unsupported protocol %q", application, request.Requirement.Name, value.Protocol)
			}
			if len(value.Signals) == 0 {
				return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s: at least one signal is required", application, request.Requirement.Name)
			}
			seenSignals := map[string]struct{}{}
			for _, signal := range value.Signals {
				signal = strings.TrimSpace(signal)
				switch signal {
				case "traces", "metrics", "logs":
				default:
					return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s: unsupported signal %q", application, request.Requirement.Name, signal)
				}
				if _, exists := seenSignals[signal]; exists {
					return Plan{}, fmt.Errorf("capability OTLP binding for %s/%s: duplicate signal %q", application, request.Requirement.Name, signal)
				}
				seenSignals[signal] = struct{}{}
			}
			value.Signals = append([]string(nil), value.Signals...)
			binding.TelemetryOTLP = &value
		}
		if request.Security != nil {
			value := *request.Security
			if err := value.Validate(); err != nil {
				return Plan{}, fmt.Errorf("capability secure binding for %s/%s: %w", application, request.Requirement.Name, err)
			}
			binding.Security = &value
		}
		plan.Items = append(plan.Items, PlanItem{Resource: resource, Binding: binding})
	}
	return plan, nil
}

// Execution is one prepared provider lifecycle. Prepare performs all
// resolution/preflight work without mutation. ProvisionAndBind and Verify can
// then be coordinated around runtime/workload convergence without duplicating
// provider lifecycle semantics.
type Execution struct {
	requests []Request
	result   Result
}

func Prepare(ctx context.Context, application string, requests []Request) (*Execution, Result, error) {
	plan, err := BuildPlan(application, requests)
	if err != nil {
		result := Result{Application: strings.TrimSpace(application), Status: StatusFailed}
		return nil, result, err
	}
	result := Result{Application: plan.Application, Status: StatusReady, Plan: plan}
	for _, item := range plan.Items {
		result.Steps = append(result.Steps, readyStep(PhaseResolve, item))
	}
	for i, item := range plan.Items {
		if err := requests[i].Driver.Preflight(ctx, item.Resource, item.Binding); err != nil {
			result.Steps = append(result.Steps, failedStep(PhasePreflight, item, "provider-preflight-failed", err))
			result.Status = StatusFailed
			return nil, result, fmt.Errorf("capability preflight failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		result.Steps = append(result.Steps, readyStep(PhasePreflight, item))
	}
	return &Execution{requests: requests, result: result}, result, nil
}

func (e *Execution) ProvisionAndBind(ctx context.Context) (Result, error) {
	if e == nil {
		return Result{Status: StatusFailed}, fmt.Errorf("capability execution is nil")
	}
	for i, item := range e.result.Plan.Items {
		driver := e.requests[i].Driver
		if err := driver.Provision(ctx, item.Resource, item.Binding); err != nil {
			e.result.Steps = append(e.result.Steps, failedStep(PhaseApply, item, "provider-apply-failed", err))
			e.result.Status = StatusFailed
			return e.result, fmt.Errorf("capability apply failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		e.result.Steps = append(e.result.Steps, readyStep(PhaseApply, item))
		if err := driver.Bind(ctx, item.Resource, item.Binding); err != nil {
			e.result.Steps = append(e.result.Steps, failedStep(PhaseBind, item, "provider-bind-failed", err))
			e.result.Status = StatusFailed
			return e.result, fmt.Errorf("capability bind failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		e.result.Steps = append(e.result.Steps, readyStep(PhaseBind, item))
	}
	return e.result, nil
}

func (e *Execution) Verify(ctx context.Context) (Result, error) {
	if e == nil {
		return Result{Status: StatusFailed}, fmt.Errorf("capability execution is nil")
	}
	for i, item := range e.result.Plan.Items {
		if err := e.requests[i].Driver.Verify(ctx, item.Resource, item.Binding); err != nil {
			e.result.Steps = append(e.result.Steps, failedStep(PhaseVerify, item, "provider-verification-failed", err))
			e.result.Status = StatusFailed
			return e.result, fmt.Errorf("capability verification failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		e.result.Steps = append(e.result.Steps, readyStep(PhaseVerify, item))
	}
	e.result.Status = StatusReady
	return e.result, nil
}

// Run executes one deterministic capability lifecycle. It is implemented in
// terms of the staged API so application orchestration and provider tests share
// exactly one lifecycle implementation.
func Run(ctx context.Context, application string, requests []Request) (Result, error) {
	execution, result, err := Prepare(ctx, application, requests)
	if err != nil {
		return result, err
	}
	result, err = execution.ProvisionAndBind(ctx)
	if err != nil {
		return result, err
	}
	return execution.Verify(ctx)
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
