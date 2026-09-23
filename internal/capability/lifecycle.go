package capability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/reconciliation"
)

type Phase string

const (
	PhaseResolve   Phase = "resolve"
	PhasePreflight Phase = "preflight"
	PhaseObserve   Phase = "observe"
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

// ProviderOperationObservation is deliberately metadata-only. It gives
// observability adapters a stable hook without exposing workload bindings,
// credentials, endpoints or provider-specific configuration.
type ProviderOperationObservation struct {
	Phase       Phase         `json:"phase"`
	Status      Status        `json:"status"`
	Application string        `json:"application"`
	Capability  Kind          `json:"capability"`
	Resource    string        `json:"resource"`
	Provider    ProviderKind  `json:"provider"`
	Duration    time.Duration `json:"duration"`
}

type ProviderOperationObserver interface {
	ObserveProviderOperation(ProviderOperationObservation)
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

type MetricsBinding struct {
	Direction string `json:"direction"`
	Format    string `json:"format"`
	Service   string `json:"service"`
	Scheme    string `json:"scheme"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
}

type LogsBinding struct {
	Direction string `json:"direction"`
	Format    string `json:"format"`
	Service   string `json:"service"`
}

type Binding struct {
	Resource        Resource                `json:"resource"`
	Workload        string                  `json:"workload"`
	HTTPExposure    *HTTPExposureBinding    `json:"http_exposure,omitempty"`
	ObjectStorageS3 *ObjectStorageS3Binding `json:"object_storage_s3,omitempty"`
	TelemetryOTLP   *OTLPTelemetryBinding   `json:"telemetry_otlp,omitempty"`
	Metrics         *MetricsBinding         `json:"metrics,omitempty"`
	Logs            *LogsBinding            `json:"logs,omitempty"`
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

type ReconciliationResult struct {
	Resource Resource              `json:"resource"`
	Result   reconciliation.Result `json:"result"`
}

type StepResult struct {
	Phase       Phase        `json:"phase"`
	Status      Status       `json:"status"`
	Resource    Resource     `json:"resource"`
	Binding     Binding      `json:"binding"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Result struct {
	Application    string                 `json:"application"`
	Status         Status                 `json:"status"`
	Plan           Plan                   `json:"plan"`
	Steps          []StepResult           `json:"steps"`
	Reconciliation []ReconciliationResult `json:"reconciliation,omitempty"`
}

type Driver interface {
	Descriptor() Provider
	Preflight(context.Context, Resource, Binding) error
	Provision(context.Context, Resource, Binding) error
	Bind(context.Context, Resource, Binding) error
	Verify(context.Context, Resource, Binding) error
}

type ReconciliationDriver interface {
	DesiredState(Resource, Binding) reconciliation.Desired
	Observe(context.Context, Resource, Binding) (reconciliation.Observed, error)
}

type Request struct {
	Requirement     Requirement
	Workload        string
	HTTPExposure    *HTTPExposureBinding
	ObjectStorageS3 *ObjectStorageS3Binding
	TelemetryOTLP   *OTLPTelemetryBinding
	Metrics         *MetricsBinding
	Logs            *LogsBinding
	Security        *SecureBinding
	Driver          Driver
	Observer        ProviderOperationObserver
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
		if request.Requirement.Kind == Metrics && request.Metrics == nil {
			return Plan{}, fmt.Errorf("capability metrics binding for %s/%s is required", application, request.Requirement.Name)
		}
		if request.Metrics != nil {
			value := *request.Metrics
			value.Direction = strings.TrimSpace(value.Direction)
			value.Format = strings.TrimSpace(value.Format)
			value.Service = strings.TrimSpace(value.Service)
			value.Scheme = strings.TrimSpace(value.Scheme)
			value.Path = strings.TrimSpace(value.Path)
			if value.Scheme == "" {
				value.Scheme = "http"
			}
			if value.Direction != "provide" {
				return Plan{}, fmt.Errorf("capability metrics binding for %s/%s: direction must be provide", application, request.Requirement.Name)
			}
			if value.Format != "openmetrics" {
				return Plan{}, fmt.Errorf("capability metrics binding for %s/%s: unsupported format %q", application, request.Requirement.Name, value.Format)
			}
			if value.Scheme != "http" && value.Scheme != "https" {
				return Plan{}, fmt.Errorf("capability metrics binding for %s/%s: unsupported scheme %q", application, request.Requirement.Name, value.Scheme)
			}
			if value.Service == "" || value.Port < 1 || value.Port > 65535 || !strings.HasPrefix(value.Path, "/") {
				return Plan{}, fmt.Errorf("capability metrics binding for %s/%s is incomplete", application, request.Requirement.Name)
			}
			binding.Metrics = &value
		}
		if request.Requirement.Kind == Logs && request.Logs == nil {
			return Plan{}, fmt.Errorf("capability logs binding for %s/%s is required", application, request.Requirement.Name)
		}
		if request.Logs != nil {
			value := *request.Logs
			value.Direction = strings.TrimSpace(value.Direction)
			value.Format = strings.TrimSpace(value.Format)
			value.Service = strings.TrimSpace(value.Service)
			if value.Direction != "collect" {
				return Plan{}, fmt.Errorf("capability logs binding for %s/%s: direction must be collect", application, request.Requirement.Name)
			}
			if value.Format != "syslog-rfc5424" {
				return Plan{}, fmt.Errorf("capability logs binding for %s/%s: unsupported format %q", application, request.Requirement.Name, value.Format)
			}
			if value.Service == "" {
				return Plan{}, fmt.Errorf("capability logs binding for %s/%s: service is required", application, request.Requirement.Name)
			}
			binding.Logs = &value
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
	requests  []Request
	result    Result
	decisions []reconciliation.Result
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
	decisions := make([]reconciliation.Result, len(plan.Items))
	for i, item := range plan.Items {
		started := time.Now()
		if err := requests[i].Driver.Preflight(ctx, item.Resource, item.Binding); err != nil {
			observeProviderOperation(requests[i], item, PhasePreflight, StatusFailed, time.Since(started))
			result.Steps = append(result.Steps, failedStep(PhasePreflight, item, "provider-preflight-failed", err))
			result.Status = StatusFailed
			return nil, result, fmt.Errorf("capability preflight failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		observeProviderOperation(requests[i], item, PhasePreflight, StatusReady, time.Since(started))
		result.Steps = append(result.Steps, readyStep(PhasePreflight, item))

		if driver, ok := requests[i].Driver.(ReconciliationDriver); ok {
			started = time.Now()
			observed, err := driver.Observe(ctx, item.Resource, item.Binding)
			if err != nil {
				observeProviderOperation(requests[i], item, PhaseObserve, StatusFailed, time.Since(started))
				result.Steps = append(result.Steps, failedStep(PhaseObserve, item, "provider-observe-failed", err))
				result.Status = StatusFailed
				return nil, result, fmt.Errorf("capability observation failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
			}
			decision := reconciliation.Evaluate(driver.DesiredState(item.Resource, item.Binding), observed)
			decisions[i] = decision
			result.Reconciliation = append(result.Reconciliation, ReconciliationResult{Resource: item.Resource, Result: decision})
			if decision.Action == reconciliation.ActionBlocked {
				err := fmt.Errorf("%s", decision.Message)
				observeProviderOperation(requests[i], item, PhaseObserve, StatusFailed, time.Since(started))
				result.Steps = append(result.Steps, failedStep(PhaseObserve, item, "provider-reconciliation-blocked", err))
				result.Status = StatusFailed
				return nil, result, fmt.Errorf("capability reconciliation blocked for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
			}
			observeProviderOperation(requests[i], item, PhaseObserve, StatusReady, time.Since(started))
			result.Steps = append(result.Steps, readyStep(PhaseObserve, item))
		}
	}
	return &Execution{requests: requests, result: result, decisions: decisions}, result, nil
}

func (e *Execution) ProvisionAndBind(ctx context.Context) (Result, error) {
	if e == nil {
		return Result{Status: StatusFailed}, fmt.Errorf("capability execution is nil")
	}
	for i, item := range e.result.Plan.Items {
		request := e.requests[i]
		driver := request.Driver
		started := time.Now()
		decision := reconciliation.Result{}
		if i < len(e.decisions) {
			decision = e.decisions[i]
		}
		if decision.Action != reconciliation.ActionNoop && decision.Action != reconciliation.ActionObserve {
			if err := driver.Provision(ctx, item.Resource, item.Binding); err != nil {
				observeProviderOperation(request, item, PhaseApply, StatusFailed, time.Since(started))
				e.result.Steps = append(e.result.Steps, failedStep(PhaseApply, item, "provider-apply-failed", err))
				e.result.Status = StatusFailed
				return e.result, fmt.Errorf("capability apply failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
			}
		}
		observeProviderOperation(request, item, PhaseApply, StatusReady, time.Since(started))
		e.result.Steps = append(e.result.Steps, readyStep(PhaseApply, item))
		started = time.Now()
		if err := driver.Bind(ctx, item.Resource, item.Binding); err != nil {
			observeProviderOperation(request, item, PhaseBind, StatusFailed, time.Since(started))
			e.result.Steps = append(e.result.Steps, failedStep(PhaseBind, item, "provider-bind-failed", err))
			e.result.Status = StatusFailed
			return e.result, fmt.Errorf("capability bind failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		observeProviderOperation(request, item, PhaseBind, StatusReady, time.Since(started))
		e.result.Steps = append(e.result.Steps, readyStep(PhaseBind, item))
	}
	return e.result, nil
}

func (e *Execution) Verify(ctx context.Context) (Result, error) {
	if e == nil {
		return Result{Status: StatusFailed}, fmt.Errorf("capability execution is nil")
	}
	for i, item := range e.result.Plan.Items {
		request := e.requests[i]
		started := time.Now()
		if err := request.Driver.Verify(ctx, item.Resource, item.Binding); err != nil {
			observeProviderOperation(request, item, PhaseVerify, StatusFailed, time.Since(started))
			e.result.Steps = append(e.result.Steps, failedStep(PhaseVerify, item, "provider-verification-failed", err))
			e.result.Status = StatusFailed
			return e.result, fmt.Errorf("capability verification failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, err)
		}
		observeProviderOperation(request, item, PhaseVerify, StatusReady, time.Since(started))
		e.result.Steps = append(e.result.Steps, readyStep(PhaseVerify, item))
		if driver, ok := request.Driver.(ReconciliationDriver); ok {
			observed, observeErr := driver.Observe(ctx, item.Resource, item.Binding)
			if observeErr != nil {
				e.result.Status = StatusFailed
				return e.result, fmt.Errorf("capability post-verification observation failed for %s/%s: %w", item.Resource.Kind, item.Resource.Name, observeErr)
			}
			decision := reconciliation.Evaluate(driver.DesiredState(item.Resource, item.Binding), observed)
			if i < len(e.decisions) {
				e.decisions[i] = decision
			}
			for j := range e.result.Reconciliation {
				if e.result.Reconciliation[j].Resource == item.Resource {
					e.result.Reconciliation[j].Result = decision
				}
			}
			if decision.Ownership == reconciliation.OwnershipBaseHarbor && decision.State != reconciliation.StateInSync {
				e.result.Status = StatusFailed
				return e.result, fmt.Errorf("capability failed to converge for %s/%s: state=%s", item.Resource.Kind, item.Resource.Name, decision.State)
			}
		}
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

func observeProviderOperation(request Request, item PlanItem, phase Phase, status Status, duration time.Duration) {
	if request.Observer == nil {
		return
	}
	request.Observer.ObserveProviderOperation(ProviderOperationObservation{
		Phase:       phase,
		Status:      status,
		Application: item.Resource.Application,
		Capability:  item.Resource.Kind,
		Resource:    item.Resource.Name,
		Provider:    item.Resource.Provider,
		Duration:    duration,
	})
}
