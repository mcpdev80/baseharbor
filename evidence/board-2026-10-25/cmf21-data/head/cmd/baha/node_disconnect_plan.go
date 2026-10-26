package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// Disconnect projects existing owners; revocation never stops native providers.
type nodeDisconnectPlan struct {
	CoreScope       *application.ProviderImpactScope     `json:"core_scope,omitempty"`
	Scope           application.ProviderImpactScope      `json:"scope"`
	TargetID        string                               `json:"target_id"`
	NodeID          string                               `json:"node_id"`
	Functions       []application.FunctionConsequence    `json:"functions"`
	Materials       []application.MaterialConsequence    `json:"materials"`
	Effects         []application.OperationalConsequence `json:"effects"`
	storageRevision string
}

func nodeDisconnectRevision(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func planNodeDisconnect(scope targetenrollment.Scope, status targetenrollment.NodeStatus) (nodeDisconnectPlan, error) {
	p := nodeDisconnectPlan{TargetID: scope.TargetID, NodeID: scope.NodeID, Functions: []application.FunctionConsequence{}, Materials: []application.MaterialConsequence{}}
	if scope.Validate() != nil {
		return p, targetenrollment.ErrDenied
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	if err := inspectNodeDisconnectOwner(configPath, false); err != nil {
		return p, err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return p, nodeDisconnectOwnerError()
	}
	definition, exists := cfg.Targets[scope.TargetID]
	access := cfg.Access[definition.Access.Reference]
	if !exists || definition.TenantID != scope.TenantID || definition.Runtime.Provider != scope.Runtime || access.Provider != nodeConnectorProvider || access.Reference != scope.NodeID {
		return p, targetenrollment.ErrDenied
	}
	root, err := deployment.TargetStateRoot(scope.TargetID)
	if err != nil {
		return p, err
	}
	if err := inspectNodeDisconnectOwner(root, true); err != nil {
		return p, err
	}
	registryPath := filepath.Join(root, "provider-registry.json")
	if err := inspectNodeDisconnectOwner(registryPath, true); err != nil {
		return p, err
	}
	registry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		return p, nodeDisconnectOwnerError()
	}
	deploymentDir := filepath.Join(root, "deployments")
	if err := inspectNodeDisconnectOwner(deploymentDir, true); err != nil {
		return p, err
	}
	entries, err := os.ReadDir(deploymentDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return p, nodeDisconnectOwnerError()
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return p, nodeDisconnectOwnerError()
		}
		if err := inspectNodeDisconnectOwner(filepath.Join(deploymentDir, entry.Name(), "deployment.json"), false); err != nil {
			return p, err
		}
	}
	records, err := deployment.ListDeployments(scope.TargetID)
	if err != nil {
		return p, nodeDisconnectOwnerError()
	}
	p.Materials = append(p.Materials, nodeDisconnectRetainedMaterial(scope.TargetID, "target-definition", configPath))
	for _, reference := range []struct{ kind, path string }{{"target-runtime-state", root}, {"provider-registration", registryPath}} {
		if _, err := os.Lstat(reference.path); err == nil {
			p.Materials = append(p.Materials, nodeDisconnectRetainedMaterial(scope.TargetID, reference.kind, reference.path))
		}
	}
	p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "target/" + scope.TargetID, Function: "outbound-mtls", Via: []string{scope.NodeID}, Required: true, Outcome: "unavailable", Behavior: "Authenticated Connector control access, including management/Console remote operations, is withdrawn. Native workloads are not stopped.", Evidence: "Core enrollment authority and per-request Connector admission"})
	ownerPaths := []string{configPath, registryPath}
	for _, record := range records {
		location, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return p, err
		}
		ownerPaths = append(ownerPaths, filepath.Join(location, "deployment.json"))
		p.Materials = append(p.Materials, nodeDisconnectRetainedMaterial(scope.TargetID, "deployment-recovery", location))
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + record.Identity.ApplicationID + "/" + record.Identity.Environment, Function: "remote-management", Via: []string{scope.NodeID}, Required: true, Outcome: "unavailable", Behavior: "Connector plan/apply/status/doctor, logs and terminal access cease. Ongoing native service/data behavior is unverified.", Evidence: "existing protected deployment owner"})
	}
	for _, binding := range registry.Bindings {
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + binding.ApplicationID + "/" + binding.Environment, Function: string(binding.Resource.Kind), Via: []string{binding.ProviderInstanceID}, Required: true, Outcome: "unknown", Behavior: "Provider configuration and data are unchanged. Ongoing service access, credentials, leases and renewal cannot be verified through the disconnected connector.", Evidence: "existing protected Target binding"})
	}
	p.Effects = []application.OperationalConsequence{
		{Aspect: "connector-access", Before: "The registered node may obtain authenticated outbound mTLS admission while enrolled and unrevoked.", After: "Node admission is revoked; the current issued certificate is submitted for issuer revocation. Reconnection requires explicitly authorized enrollment/renewal, not automatic trust bypass.", Evidence: "Core enrollment authority"},
		{Aspect: "runtime-data", Before: "Native workload/provider state is not inspected by disconnect.", After: "No container, volume, application credential, provider or foreign infrastructure operation is performed. Actual remote data locations and ongoing usability remain unknown; no handoff is claimed.", Evidence: "disconnect invokes only existing enrollment revocation"},
		{Aspect: "recovery", Before: "Target definition, provider bindings and deployment publication/recovery owners exist at the displayed locations.", After: "Existing owners remain at those locations for inspection, retry and authorized reconnection. Retention does not restore remote access.", Evidence: "protected owner projection"},
	}
	if !status.ExpiresAt.IsZero() {
		p.Effects = append(p.Effects, application.OperationalConsequence{Aspect: "certificate", Before: "The enrollment authority records current certificate expiry " + status.ExpiresAt.UTC().Format(time.RFC3339) + ".", After: "Admission denial is separate from issuer revocation; certificate acceptance by other consumers is unverified. No unrelated application certificate or CA is revoked.", Evidence: "current enrollment status"})
	}
	owners := map[string]string{}
	for _, path := range ownerPaths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && path == registryPath {
			owners[path] = "absent"
			continue
		}
		if err != nil {
			return p, nodeDisconnectOwnerError()
		}
		owners[path] = nodeDisconnectRevision(data)
	}
	p.storageRevision = nodeDisconnectRevision(owners)
	revision := nodeDisconnectRevision(struct {
		Storage string
		Scope   targetenrollment.Scope
		Status  targetenrollment.NodeStatus
		Serial  string
		Plan    nodeDisconnectPlan
	}{p.storageRevision, scope, status, status.Serial, p})
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "connector-enrollment", Reference: configPath, Subject: scope.Identity(), Revision: revision}, nil, false)
	return p, err
}

func nodeDisconnectRetainedMaterial(target, kind, path string) application.MaterialConsequence {
	location := &application.ResourceLocation{Owner: "target/" + target, Target: target, Kind: "path", Reference: path}
	id := target + "/" + kind + "/" + filepath.Base(path)
	return application.MaterialConsequence{Before: application.ProviderMaterial{ID: id, Kind: kind, Ownership: capability.OwnershipBaseHarbor, Location: location, Recovery: []application.ResourceLocation{}, Verified: true, Evidence: "existing protected owner"}, After: application.MaterialDisposition{MaterialID: id, Disposition: "retained", Destination: location, Supported: true}, Meaning: "Remains at its existing owner location; remote access is not continued by retention."}
}

func nodeDisconnectOwnerError() error {
	return machine.NewError(machine.ErrorOwnershipAmbiguous, "Node disconnect requires unchanged protected registration and deployment owners.", "Recover their existing ownership and protection before requesting a new impact plan.", false)
}

func inspectNodeDisconnectOwner(path string, optional bool) error {
	for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) && optional {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nodeDisconnectOwnerError()
		}
	}
	for _, current := range []string{filepath.Dir(path), path} {
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && optional {
			continue
		}
		if err != nil {
			return nodeDisconnectOwnerError()
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0022 != 0 || (!info.IsDir() && info.Mode().Perm()&0077 != 0) {
			return nodeDisconnectOwnerError()
		}
	}
	return nil
}
