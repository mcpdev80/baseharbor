package keycloak

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

type Topology string

const (
	TopologySingle Topology = "single"
	TopologyHA     Topology = "ha"
)

type Member struct {
	Name    string
	Version string
	Ready   bool
}

type State struct {
	Version      string
	Owner        string
	Topology     Topology
	Healthy      bool
	DatabaseType string
	Members      []Member
}

type Ops interface {
	Inspect(context.Context) (State, error)
	CheckUpgradePath(context.Context, string, string) error
	CreateBackup(context.Context, string) (providerupgrade.BackupRef, error)
	VerifyBackup(context.Context, providerupgrade.BackupRef) error

	ApplySingle(context.Context, string, string, string) error
	StopAllMembers(context.Context) error
	ApplyAllMembers(context.Context, string, string, string) error
	ReplaceMember(context.Context, string, string, string, string) error
	WaitMemberReady(context.Context, string) error
	WaitAllReady(context.Context) error

	VerifyDatabase(context.Context) error
	VerifyRealmState(context.Context) error
	VerifyOIDCDiscovery(context.Context) error
	VerifyTokenFlow(context.Context) error

	RestoreBackup(context.Context, providerupgrade.BackupRef, string) error
}

type Adapter struct {
	ops Ops
}

func New(ops Ops) *Adapter {
	return &Adapter{ops: ops}
}

func (a *Adapter) Inventory(ctx context.Context) (providerupgrade.Inventory, error) {
	if a == nil || a.ops == nil {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak inventory", errors.New("operations are required"))
	}
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak inspect", err)
	}
	if strings.TrimSpace(state.Version) == "" {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak inspect", errors.New("version is not observable"))
	}
	if state.Topology != TopologySingle && state.Topology != TopologyHA {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", fmt.Errorf("unsupported topology %q", state.Topology))
	}
	if state.DatabaseType == "" {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak database", errors.New("database realization is not observable"))
	}
	if state.Topology == TopologyHA && len(state.Members) < 3 {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", errors.New("HA topology requires at least three members"))
	}
	if state.Topology == TopologySingle && len(state.Members) != 1 {
		return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", errors.New("single topology requires exactly one member"))
	}
	seen := make(map[string]struct{}, len(state.Members))
	for _, member := range state.Members {
		if strings.TrimSpace(member.Name) == "" {
			return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", errors.New("member name must not be empty"))
		}
		if _, duplicate := seen[member.Name]; duplicate {
			return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", errors.New("duplicate member in inventory"))
		}
		seen[member.Name] = struct{}{}
		if member.Version != state.Version {
			return providerupgrade.Inventory{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak topology", errors.New("inconsistent member versions require recovery before upgrade"))
		}
	}
	return providerupgrade.Inventory{
		Provider: providerupgrade.ProviderKeycloak,
		Version:  state.Version,
		Topology: string(state.Topology),
		Owner:    state.Owner,
		Healthy:  state.Healthy && allMembersReady(state),
		Details: map[string]string{
			"database_type": state.DatabaseType,
			"members":       strconv.Itoa(len(state.Members)),
		},
	}, nil
}

func allMembersReady(state State) bool {
	if state.Topology == TopologySingle {
		if len(state.Members) == 0 {
			return state.Healthy
		}
		return state.Members[0].Ready
	}
	if len(state.Members) == 0 {
		return false
	}
	for _, member := range state.Members {
		if !member.Ready {
			return false
		}
	}
	return true
}

func (a *Adapter) Preflight(ctx context.Context, req providerupgrade.Request) (providerupgrade.Assessment, error) {
	if err := req.Validate(); err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak preflight", err)
	}
	inventory, err := a.Inventory(ctx)
	if err != nil {
		return providerupgrade.Assessment{}, err
	}
	if inventory.Owner != "baseharbor" {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak preflight", errors.New("foreign Keycloak realization must not be mutated"))
	}
	if inventory.Version != req.CurrentVersion {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak preflight", fmt.Errorf("observed version %q differs from planned %q", inventory.Version, req.CurrentVersion))
	}
	if !inventory.Healthy {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak preflight", errors.New("Keycloak must be healthy before upgrade"))
	}
	current, err := providerupgrade.ParseVersion(req.CurrentVersion)
	if err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak version", err)
	}
	target, err := providerupgrade.ParseVersion(req.TargetVersion)
	if err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak version", err)
	}
	switch current.Compare(target) {
	case 1:
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak preflight", errors.New("downgrade is not supported"))
	case 0:
		return providerupgrade.Assessment{Classification: providerupgrade.ClassificationNoChange, Reason: "Keycloak version unchanged"}, nil
	}
	if err := a.ops.CheckUpgradePath(ctx, req.CurrentVersion, req.TargetVersion); err != nil {
		return providerupgrade.Assessment{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak compatibility", err)
	}
	strategy := "recreate"
	if inventory.Topology == string(TopologyHA) && rollingPatchSupported(current, target) {
		strategy = "rolling"
	}
	return providerupgrade.Assessment{
		Classification: providerupgrade.ClassificationSupported,
		Reason:         strategy + " Keycloak upgrade with verified database/configuration backup",
		BackupRequired: true,
	}, nil
}

func rollingPatchSupported(current, target providerupgrade.Version) bool {
	min := providerupgrade.Version{Major: 26, Minor: 6, Patch: 0}
	return current.Compare(min) >= 0 && current.SameMinor(target) && current.Compare(target) < 0
}

func (a *Adapter) Backup(ctx context.Context, req providerupgrade.Request) (providerupgrade.BackupRef, error) {
	assessment, err := a.Preflight(ctx, req)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if assessment.Classification == providerupgrade.ClassificationNoChange {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupRequired, "keycloak backup", errors.New("backup is not required for an unchanged provider"))
	}
	backup, err := a.ops.CreateBackup(ctx, req.CurrentVersion)
	if err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak create backup", err)
	}
	if backup.Provider == "" {
		backup.Provider = providerupgrade.ProviderKeycloak
	}
	if backup.Version == "" {
		backup.Version = req.CurrentVersion
	}
	if err := backup.Validate(providerupgrade.ProviderKeycloak, req.CurrentVersion); err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak backup", err)
	}
	if backup.Metadata["database_verified"] != "true" {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak backup", errors.New("verified database backup is required"))
	}
	if backup.Metadata["configuration_verified"] != "true" {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak backup", errors.New("verified configuration backup is required"))
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.BackupRef{}, providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak verify backup", err)
	}
	return backup, nil
}

func (a *Adapter) Execute(ctx context.Context, req providerupgrade.Request, backup providerupgrade.BackupRef) error {
	assessment, err := a.Preflight(ctx, req)
	if err != nil {
		return err
	}
	if assessment.Classification == providerupgrade.ClassificationNoChange {
		return nil
	}
	if err := backup.Validate(providerupgrade.ProviderKeycloak, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorBackupRequired, "keycloak execute", err)
	}
	if backup.Metadata["database_verified"] != "true" || backup.Metadata["configuration_verified"] != "true" {
		return providerupgrade.Wrap(providerupgrade.ErrorBackupRequired, "keycloak execute", errors.New("database and configuration backups must be verified"))
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "keycloak execute", err)
	}

	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "keycloak execute inspect", err)
	}
	current, err := providerupgrade.ParseVersion(req.CurrentVersion)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak execute version", err)
	}
	target, err := providerupgrade.ParseVersion(req.TargetVersion)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak execute version", err)
	}

	switch state.Topology {
	case TopologySingle:
		if err := a.ops.ApplySingle(ctx, req.TargetVersion, req.TargetImage, req.TargetDigest); err != nil {
			return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak single apply", err)
		}
		if err := a.ops.WaitAllReady(ctx); err != nil {
			return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak single readiness", err)
		}
	case TopologyHA:
		if rollingPatchSupported(current, target) {
			for _, member := range state.Members {
				if !member.Ready {
					return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak rolling precondition", fmt.Errorf("member %s is not ready", member.Name))
				}
				if err := a.ops.ReplaceMember(ctx, member.Name, req.TargetVersion, req.TargetImage, req.TargetDigest); err != nil {
					return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak rolling replace "+member.Name, err)
				}
				if err := a.ops.WaitMemberReady(ctx, member.Name); err != nil {
					return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak rolling readiness "+member.Name, err)
				}
				observed, err := a.ops.Inspect(ctx)
				if err != nil {
					return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak rolling inspect", err)
				}
				if !quorumReady(observed) {
					return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak rolling quorum", errors.New("cluster lost ready quorum"))
				}
			}
		} else {
			if err := a.ops.StopAllMembers(ctx); err != nil {
				return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak recreate stop", err)
			}
			if err := a.ops.ApplyAllMembers(ctx, req.TargetVersion, req.TargetImage, req.TargetDigest); err != nil {
				return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak recreate apply", err)
			}
			if err := a.ops.WaitAllReady(ctx); err != nil {
				return providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "keycloak recreate readiness", err)
			}
		}
	default:
		return providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "keycloak execute", fmt.Errorf("unsupported topology %q", state.Topology))
	}
	return nil
}

func quorumReady(state State) bool {
	if state.Topology != TopologyHA || len(state.Members) < 2 {
		return false
	}
	ready := 0
	for _, member := range state.Members {
		if member.Ready {
			ready++
		}
	}
	return ready >= len(state.Members)/2+1
}

func (a *Adapter) Verify(ctx context.Context, req providerupgrade.Request) error {
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak inspect", err)
	}
	if state.Version != req.TargetVersion {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak version", fmt.Errorf("running %q, expected %q", state.Version, req.TargetVersion))
	}
	if !state.Healthy || !allMembersReady(state) {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak readiness", errors.New("Keycloak is not fully ready"))
	}
	if state.Owner != "baseharbor" || (state.Topology != TopologySingle && state.Topology != TopologyHA) {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak ownership", errors.New("provider ownership or topology changed"))
	}
	if state.Topology == TopologySingle && len(state.Members) != 1 || state.Topology == TopologyHA && len(state.Members) < 3 {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak topology", errors.New("provider member count changed"))
	}
	seen := make(map[string]bool, len(state.Members))
	for _, member := range state.Members {
		if strings.TrimSpace(member.Name) == "" || seen[member.Name] || member.Version != req.TargetVersion {
			return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak member version", errors.New("provider member inventory is inconsistent with target"))
		}
		seen[member.Name] = true
	}
	if err := a.ops.VerifyDatabase(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak database", err)
	}
	if err := a.ops.VerifyRealmState(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak realms/clients/roles/users", err)
	}
	if err := a.ops.VerifyOIDCDiscovery(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak oidc discovery", err)
	}
	if err := a.ops.VerifyTokenFlow(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "keycloak token flow", err)
	}
	return nil
}

func (a *Adapter) Recover(ctx context.Context, req providerupgrade.Request, backup providerupgrade.BackupRef) error {
	if err := backup.Validate(providerupgrade.ProviderKeycloak, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recover", err)
	}
	if backup.Metadata["database_verified"] != "true" || backup.Metadata["configuration_verified"] != "true" {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recover", errors.New("verified database and configuration backups are required"))
	}
	if err := a.ops.VerifyBackup(ctx, backup); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recover", err)
	}
	if err := a.ops.RestoreBackup(ctx, backup, req.CurrentVersion); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak restore old installation and database", err)
	}
	state, err := a.ops.Inspect(ctx)
	if err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recovered inspect", err)
	}
	if state.Version != req.CurrentVersion || !state.Healthy || !allMembersReady(state) {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recovered state", errors.New("recovered Keycloak is not healthy on original version"))
	}
	if err := a.ops.VerifyRealmState(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recovered realm state", err)
	}
	if err := a.ops.VerifyOIDCDiscovery(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recovered oidc", err)
	}
	if err := a.ops.VerifyTokenFlow(ctx); err != nil {
		return providerupgrade.Wrap(providerupgrade.ErrorRecoveryFailed, "keycloak recovered token flow", err)
	}
	return nil
}
