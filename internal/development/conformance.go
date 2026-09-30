package development

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

type ConformanceStatus string

const (
	ConformancePass ConformanceStatus = "pass"
	ConformanceFail ConformanceStatus = "fail"
)

type ConformanceCheck struct {
	Name    string            `json:"name"`
	Status  ConformanceStatus `json:"status"`
	Message string            `json:"message,omitempty"`
}

type ConformanceReport struct {
	AdapterID      string             `json:"adapter_id"`
	AdapterVersion string             `json:"adapter_version"`
	Status         ConformanceStatus  `json:"status"`
	Checks         []ConformanceCheck `json:"checks"`
}

func (r *ConformanceReport) Add(name string, err error) {
	check := ConformanceCheck{Name: name, Status: ConformancePass}
	if err != nil {
		check.Status = ConformanceFail
		check.Message = err.Error()
		r.Status = ConformanceFail
	}
	r.Checks = append(r.Checks, check)
}

func (r ConformanceReport) Passed() bool { return r.Status == ConformancePass }

func RunAdapterConformance(root string, request NewApplicationRequest, adapter Adapter) ConformanceReport {
	descriptor := adapter.Descriptor()
	report := ConformanceReport{
		AdapterID:      descriptor.ID,
		AdapterVersion: descriptor.Version,
		Status:         ConformancePass,
	}

	report.Add("descriptor", validateAdapterDescriptor(descriptor))
	registry, registryErr := NewRegistry(adapter)
	report.Add("contract-mapping", registryErr)
	if registryErr != nil {
		return report
	}

	bootstrap, err := BootstrapApplication(request, registry)
	report.Add("development-planning", err)
	if err != nil {
		return report
	}
	report.Add("binding-portability", validateBindingActions(bootstrap.Contract, bootstrap.Plan))
	report.Add("capability-integration", validateCapabilityActions(bootstrap.Contract, bootstrap.Plan))
	report.Add("secret-safety", validateGeneratedSecretSafety(request, bootstrap.Files))
	report.Add("no-provider-leakage", validatePortableContractProviderNeutrality(bootstrap.Contract))

	replayRequest := request
	replayRequest.ApplicationID = bootstrap.Manifest.ApplicationID
	second, secondErr := BootstrapApplication(replayRequest, registry)
	if secondErr != nil {
		report.Add("idempotency", secondErr)
	} else {
		report.Add("idempotency", compareGeneratedBootstrap(bootstrap, second))
	}

	cleanup := false
	if strings.TrimSpace(root) == "" {
		root, err = os.MkdirTemp("", "baseharbor-adapter-conformance-")
		if err != nil {
			report.Add("bootstrap", err)
			return report
		}
		cleanup = true
	} else {
		_ = os.RemoveAll(root)
	}
	if cleanup {
		defer os.RemoveAll(root)
	}
	if err := WriteGeneratedFiles(root, bootstrap.Files); err != nil {
		report.Add("bootstrap", err)
		return report
	}
	report.Add("bootstrap", nil)

	detection, err := adapter.Detect(root)
	if err == nil && !detection.Detected {
		err = fmt.Errorf("generated repository is not detected by its own adapter")
	}
	report.Add("detection", err)

	validation, validationErr := adapter.Validate(root, bootstrap.Contract, bootstrap.Profile.Components[0])
	if validationErr == nil && !validation.Satisfied {
		validationErr = fmt.Errorf("adapter validation is not satisfied: %s", strings.Join(validation.Diagnostics, "; "))
	}
	report.Add("evidence-validation", validationErr)

	inspection, inspectErr := repositoryinspect.Inspect(context.Background(), root)
	report.Add("inspect-round-trip", inspectErr)
	if inspectErr == nil {
		report.Add("round-trip-satisfied", validateDeclaredReconciliation(inspection))
	}
	return report
}

func validateAdapterDescriptor(descriptor extension.Metadata) error {
	return descriptor.Validate()
}

func validateBindingActions(contract application.PortableContract, plan DevelopmentPlan) error {
	kinds := map[capability.Kind]bool{}
	for _, action := range plan.Actions {
		if action.Kind == ActionBinding {
			kinds[action.Capability] = true
		}
	}
	for _, requirement := range contract.Capabilities {
		switch requirement.Kind {
		case capability.Metrics, capability.Logs, capability.Traces:
			continue
		}
		if !kinds[requirement.Kind] {
			return fmt.Errorf("capability %s has no portable binding action", requirement.Kind)
		}
	}
	if contract.Secrets.Managed && !kinds[capability.Secrets] {
		return fmt.Errorf("managed secrets have no binding actions")
	}
	return nil
}

func validateCapabilityActions(contract application.PortableContract, plan DevelopmentPlan) error {
	seen := map[capability.Kind]bool{}
	for _, action := range plan.Actions {
		if action.Capability != "" {
			seen[action.Capability] = true
		}
	}
	for _, requirement := range contract.Capabilities {
		if !seen[requirement.Kind] {
			return fmt.Errorf("capability %s has no development action", requirement.Kind)
		}
	}
	if contract.Secrets.Managed && !seen[capability.Secrets] {
		return fmt.Errorf("managed secrets have no development action")
	}
	return nil
}

func validateGeneratedSecretSafety(request NewApplicationRequest, files []GeneratedFile) error {
	secretNames := append([]string(nil), request.Secrets...)
	if len(secretNames) == 0 {
		for _, kind := range request.Capabilities {
			if kind == capability.Secrets {
				secretNames = append(secretNames, "APP_SECRET")
				break
			}
		}
	}
	for _, file := range files {
		if filepath.Base(file.Path) == ".env" {
			return fmt.Errorf("adapter generated a real .env secret file")
		}
		if filepath.Base(file.Path) != ".env.example" {
			continue
		}
		for _, name := range secretNames {
			for _, line := range strings.Split(string(file.Content), "\n") {
				if strings.HasPrefix(line, name+"=") && strings.TrimPrefix(line, name+"=") != "" {
					return fmt.Errorf("adapter generated a value for secret %s", name)
				}
			}
		}
	}
	return nil
}

func validatePortableContractProviderNeutrality(contract application.PortableContract) error {
	data, err := json.Marshal(contract)
	if err != nil {
		return err
	}
	lower := strings.ToLower(string(data))
	for _, product := range []string{"postgresql", "valkey", "openbao", "caddy", "seaweedfs", "keycloak"} {
		if strings.Contains(lower, product) {
			return fmt.Errorf("portable contract leaks provider product %q", product)
		}
	}
	return nil
}

func compareGeneratedBootstrap(a, b BootstrapResult) error {
	if !reflect.DeepEqual(a.Manifest, b.Manifest) ||
		!reflect.DeepEqual(a.Contract, b.Contract) ||
		!reflect.DeepEqual(a.Profile, b.Profile) ||
		!reflect.DeepEqual(a.Plan, b.Plan) {
		return fmt.Errorf("repeated bootstrap produced different semantic results")
	}
	if !reflect.DeepEqual(normalizedGeneratedFiles(a.Files), normalizedGeneratedFiles(b.Files)) {
		return fmt.Errorf("repeated bootstrap produced different files")
	}
	return nil
}

func normalizedGeneratedFiles(files []GeneratedFile) []GeneratedFile {
	result := append([]GeneratedFile(nil), files...)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	for i := range result {
		result[i].Content = bytes.Clone(result[i].Content)
	}
	return result
}

func validateDeclaredReconciliation(result repositoryinspect.Result) error {
	if len(result.Declared) == 0 {
		return fmt.Errorf("inspection produced no declared capabilities")
	}
	for _, declared := range result.Declared {
		satisfied := false
		for _, item := range result.Reconciliation {
			if item.Capability != declared.Capability {
				continue
			}
			if item.Name != "" && declared.Name != "" && item.Name != declared.Name {
				continue
			}
			if item.State == repositoryinspect.ReconciliationSatisfied {
				satisfied = true
				break
			}
		}
		if !satisfied {
			return fmt.Errorf("declared capability %s/%s is not satisfied", declared.Capability, declared.Name)
		}
	}
	return nil
}
