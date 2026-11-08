package main

import (
	"errors"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Only an independently attested installation owner authorizes discovery of
// Core-only Targets. An invalid owner must never fall back to provider guessing.
func populateFullDestroyTargetFromDataRoot(dataRoot, name string, targets map[string]deployment.ResolvedTarget, results *[]fullDestroyResult) {
	if owned, err := deployment.ResolveDataRootTarget(name); err == nil {
		targets[name] = owned
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		targets[name] = deployment.ResolvedTarget{Name: name}
		*results = append(*results, fullDestroyResult{
			Status:     "FAILED",
			Target:     name,
			Resource:   "installation-owner",
			Detail:     err.Error() + "; foreign or unverifiable owner preserved",
			Diagnostic: "unverified-data-root-core",
		})
		return
	}
	provider := ""
	if records, listErr := deployment.ListDeployments(name); listErr == nil {
		for _, record := range records {
			candidate := strings.TrimSpace(record.Applied.RuntimeProvider)
			if candidate == "" {
				continue
			}
			if provider == "" {
				provider = candidate
			} else if provider != candidate {
				provider = ""
				break
			}
		}
	}
	// "compose" is not a native runtime provider. Never invent Docker.
	targets[name] = deployment.ResolvedTarget{Name: name, RuntimeProvider: provider}
	if provider == "" {
		detail := "target config is missing; runtime provider cannot be inferred and only BaseHarbor-owned local state can be removed safely"
		if name == "local" {
			switch statErr := localRuntimeOwnerStatErr(dataRoot, name); {
			case statErr == nil:
				detail = "preserved because local target configuration and runtime ownership are missing; recover the target binding before retrying destruction"
			case !errors.Is(statErr, os.ErrNotExist):
				detail = "preserved because local runtime ownership could not be verified: " + statErr.Error() + "; recover the target binding before retrying destruction"
			}
		}
		*results = append(*results, fullDestroyResult{Status: "SKIPPED", Target: name, Resource: "runtime-provider", Detail: detail, Diagnostic: "unregistered-target-runtime"})
	}
}
