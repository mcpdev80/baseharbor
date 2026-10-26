package development

import (
	"context"
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func ValidateRepositoryCapabilities(root string, contract application.PortableContract, supports func(capability.Requirement) bool) (Validation, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return Validation{}, err
	}
	required := map[capability.Kind]bool{}
	for _, requirement := range contract.Capabilities {
		if supports(requirement) {
			required[requirement.Kind] = false
		}
	}
	if len(contract.Secrets.Required) > 0 {
		required[capability.Secrets] = false
	}
	for _, item := range result.Reconciliation {
		kind := capability.Kind(item.Capability)
		if _, tracked := required[kind]; tracked && item.State == repositoryinspect.ReconciliationSatisfied {
			required[kind] = true
		}
	}
	validation := Validation{Satisfied: true, Capabilities: required}
	for kind, satisfied := range required {
		if !satisfied {
			validation.Satisfied = false
			validation.Diagnostics = append(validation.Diagnostics, fmt.Sprintf("%s is not satisfied by repository evidence", kind))
		}
	}
	sort.Strings(validation.Diagnostics)
	return validation, nil
}
