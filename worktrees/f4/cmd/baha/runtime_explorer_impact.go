package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

type runtimeMutationPlan struct {
	Scope           application.ProviderImpactScope      `json:"scope"`
	Resource        runtimeexplorer.Resource             `json:"resource"`
	Operation       runtimeexplorer.Operation            `json:"operation"`
	Functions       []application.FunctionConsequence    `json:"functions"`
	Materials       []application.MaterialConsequence    `json:"materials"`
	Effects         []application.OperationalConsequence `json:"effects"`
	storageRevision string
}

// Project existing Target owners. A bounded native command is not proof of
// continued SQL, login, secret delivery, renewal, HA or recovery behavior.
func planRuntimeMutation(resource runtimeexplorer.Resource, operation runtimeexplorer.Operation) (runtimeMutationPlan, error) {
	p := runtimeMutationPlan{Resource: resource, Operation: operation, Functions: []application.FunctionConsequence{}, Materials: []application.MaterialConsequence{}}
	if resource.Validate() != nil || resource.Ownership != runtimeexplorer.OwnershipManaged || resource.Relationship.ApplicationID == "" || resource.Relationship.DeploymentID == "" {
		return p, runtimeMutationOwnerError()
	}
	if operation != runtimeexplorer.OperationStart && operation != runtimeexplorer.OperationStop && operation != runtimeexplorer.OperationRestart {
		return p, fmt.Errorf("runtime consequence planning requires start, stop or restart")
	}
	root, err := deployment.TargetStateRoot(resource.Ref.Target)
	if err != nil {
		return p, err
	}
	registryPath := filepath.Join(root, "provider-registry.json")
	registry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		return p, runtimeMutationOwnerError()
	}
	records, err := deployment.ListDeployments(resource.Ref.Target)
	if err != nil {
		return p, runtimeMutationOwnerError()
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	configBytes, err := runtimeMutationOwnerBytes(configPath, false)
	if err != nil {
		return p, err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return p, runtimeMutationOwnerError()
	}
	target, err := cfg.ResolveTarget(resource.Ref.Target, "")
	if err != nil || target.RuntimeProvider != resource.Ref.Provider {
		return p, runtimeMutationOwnerError()
	}
	owners := map[string]string{configPath: runtimeMutationRevision(configBytes)}
	matched := false
	for _, record := range records {
		location, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return p, err
		}
		path := filepath.Join(location, "deployment.json")
		data, err := runtimeMutationOwnerBytes(path, false)
		if err != nil {
			return p, runtimeMutationOwnerError()
		}
		owners[path] = runtimeMutationRevision(data)
		if record.Identity.DeploymentID == resource.Relationship.DeploymentID && record.Identity.ApplicationID == resource.Relationship.ApplicationID && record.Identity.Environment == resource.Relationship.Environment {
			matched = true
		}
		p.Materials = append(p.Materials, runtimeRetainedOwnerMaterial(resource.Ref.Target, record.Identity.DeploymentID, path))
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + record.Identity.ApplicationID + "/" + record.Identity.Environment, Function: "runtime-availability", Via: []string{resource.Ref.ResourceID}, Outcome: "unknown", Behavior: "The selected container changes state. Dependency, reconnection, session and data effects for this Target consumer are unverified; no secure continuation or recovery is claimed.", Evidence: "existing canonical deployment owner"})
	}
	if !matched {
		return p, runtimeMutationOwnerError()
	}
	for _, binding := range registry.Bindings {
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + binding.ApplicationID + "/" + binding.Environment, Function: string(binding.Resource.Kind), Via: []string{binding.ProviderInstanceID}, Outcome: "unknown", Behavior: "Actual provider function, access, credentials, leases and future renewal during this container operation are unverified. Other consumers may be affected; retained state does not prove continuity.", Evidence: "existing Target provider binding"})
	}
	for _, path := range []string{registryPath, filepath.Join(root, "runtime", "installation.json")} {
		data, err := runtimeMutationOwnerBytes(path, true)
		if err == nil && data == nil {
			owners[path] = "absent"
			continue
		}
		if err != nil {
			return p, runtimeMutationOwnerError()
		}
		owners[path] = runtimeMutationRevision(data)
		p.Materials = append(p.Materials, runtimeRetainedOwnerMaterial(resource.Ref.Target, filepath.Base(path), path))
	}
	if owners[filepath.Join(root, "runtime", "installation.json")] != "absent" {
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "management/" + resource.Ref.Target, Function: "management-console-access", Via: []string{resource.Ref.ResourceID}, Outcome: "unknown", Behavior: "Management/Console dependency and availability effects are unverified. Authentication, TLS and authorization are not replaced or bypassed.", Evidence: "selected Target; independent management use remains unverified"})
	}

	p.Effects = []application.OperationalConsequence{{Aspect: "runtime-lifecycle", Before: "Selected observed container state: " + resource.State.Observed, After: "Requested " + string(operation) + " of this exact native container. A stop is temporary runtime withdrawal, not persistent data deletion. A restart can interrupt work; a start does not prove restored application readiness.", Evidence: "bounded native container operation"}, {Aspect: "data-and-recovery", Before: "Protected owners remain at the displayed paths. Actual transient state, in-flight queues/work, volumes, backups and access destinations are unverified.", After: "No explicit volume, file, secret or deployment-owner deletion is requested. Native/application shutdown effects, retained-data usability and transient losses remain unknown; no invented destination or handoff.", Evidence: "only native start/stop/restart is invoked"}}
	p.storageRevision = runtimeMutationRevision(owners)
	revision := runtimeMutationRevision(struct {
		Plan    runtimeMutationPlan
		Storage string
	}{p, p.storageRevision})
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "runtime-container", Reference: root, Subject: resource.Ref.ResourceID, Revision: revision}, nil, false)
	return p, err
}

func runtimeMutationRevision(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func runtimeRetainedOwnerMaterial(target, id, path string) application.MaterialConsequence {
	location := &application.ResourceLocation{Owner: "target/" + target, Target: target, Kind: "path", Reference: path}
	return application.MaterialConsequence{Before: application.ProviderMaterial{ID: id, Kind: "recovery-owner", Ownership: capability.OwnershipBaseHarbor, Location: location, Verified: true, Evidence: "existing protected deployment/registry owner"}, After: application.MaterialDisposition{MaterialID: id, Disposition: "retained", Destination: location, Supported: true}, Meaning: "Remains at its existing owner; no claim of uninterrupted function or usable data recovery."}
}
