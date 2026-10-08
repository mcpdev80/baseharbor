package keycloak

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

// CoreHooks are owned by the Core update orchestrator. No fallback may
// invent SQL snapshots or mutate unowned database instances.
type CoreHooks struct {
	Inspect func(context.Context) (State, error)
	Compatibility func(context.Context, string, string) error
	Backup func(context.Context, string) (providerupgrade.BackupRef, error)
	VerifyBackup func(context.Context, providerupgrade.BackupRef) error
	ApplySingle func(context.Context, string, string, string) error
	StopHA func(context.Context) error
	ApplyHA func(context.Context, string, string, string) error
	ReplaceMember func(context.Context, string, string, string, string) error
	WaitMember func(context.Context, string) error
	WaitAll func(context.Context) error
	VerifySQL func(context.Context) error
	VerifyRealms func(context.Context) error
	VerifyTokens func(context.Context) error
	Restore func(context.Context, providerupgrade.BackupRef, string) error
}

// NativeOps binds the existing Core identity semantic verification to the
// isolated adapter. Mutations and backing SQL recovery remain Core-owned.
type NativeOps struct {
	DataDir string
	Namespace string
	InstallationID string
	ExpectedIssuer string
	Hooks CoreHooks
}

var _ Ops = (*NativeOps)(nil)

func (n *NativeOps) Inspect(ctx context.Context) (State, error) {
	if n == nil || n.Hooks.Inspect == nil { return State{}, errors.New("Keycloak Core inventory is unavailable") }
	return n.Hooks.Inspect(ctx)
}
func (n *NativeOps) CheckUpgradePath(ctx context.Context, from, to string) error {
	if n.Hooks.Compatibility == nil { return errors.New("Keycloak upgrade path lacks reviewed compatibility evidence") }
	return n.Hooks.Compatibility(ctx, from, to)
}
func (n *NativeOps) CreateBackup(ctx context.Context, from string) (providerupgrade.BackupRef, error) {
	if n.Hooks.Backup == nil { return providerupgrade.BackupRef{}, errors.New("Keycloak SQL/configuration backup unavailable") }
	return n.Hooks.Backup(ctx, from)
}
func (n *NativeOps) VerifyBackup(ctx context.Context, backup providerupgrade.BackupRef) error {
	if n.Hooks.VerifyBackup == nil { return errors.New("Keycloak backup verification unavailable") }
	return n.Hooks.VerifyBackup(ctx, backup)
}
func (n *NativeOps) ApplySingle(ctx context.Context, version, image, digest string) error {
	if n.Hooks.ApplySingle == nil { return errors.New("Keycloak single-node upgrade lifecycle unavailable") }
	return n.Hooks.ApplySingle(ctx, version, image, digest)
}
func (n *NativeOps) StopAllMembers(ctx context.Context) error {
	if n.Hooks.StopHA == nil { return errors.New("Keycloak HA stop lifecycle unavailable") }
	return n.Hooks.StopHA(ctx)
}
func (n *NativeOps) ApplyAllMembers(ctx context.Context, version, image, digest string) error {
	if n.Hooks.ApplyHA == nil { return errors.New("Keycloak HA upgrade lifecycle unavailable") }
	return n.Hooks.ApplyHA(ctx, version, image, digest)
}
func (n *NativeOps) ReplaceMember(ctx context.Context, member, version, image, digest string) error {
	if n.Hooks.ReplaceMember == nil { return errors.New("Keycloak rolling member replacement unavailable") }
	return n.Hooks.ReplaceMember(ctx, member, version, image, digest)
}
func (n *NativeOps) WaitMemberReady(ctx context.Context, member string) error {
	if n.Hooks.WaitMember == nil { return errors.New("Keycloak member readiness verification unavailable") }
	return n.Hooks.WaitMember(ctx, member)
}
func (n *NativeOps) WaitAllReady(ctx context.Context) error {
	if n.Hooks.WaitAll == nil { return errors.New("Keycloak readiness verification unavailable") }
	return n.Hooks.WaitAll(ctx)
}
func (n *NativeOps) VerifyDatabase(ctx context.Context) error {
	if n.Hooks.VerifySQL == nil { return errors.New("Keycloak backing SQL verification unavailable") }
	return n.Hooks.VerifySQL(ctx)
}
func (n *NativeOps) VerifyRealmState(ctx context.Context) error {
	if n.Hooks.VerifyRealms == nil { return errors.New("Keycloak realm/client/user verification unavailable") }
	return n.Hooks.VerifyRealms(ctx)
}
func (n *NativeOps) VerifyOIDCDiscovery(ctx context.Context) error {
	if n.DataDir == "" || n.InstallationID == "" || n.ExpectedIssuer == "" {
		return errors.New("Keycloak verified installation and issuer are required")
	}
	if err := identityprovider.VerifyCoreIdentity(ctx, n.DataDir, n.Namespace, n.InstallationID, n.ExpectedIssuer); err != nil {
		return fmt.Errorf("verify installed Keycloak Core OIDC issuer: %w", err)
	}
	return nil
}
func (n *NativeOps) VerifyTokenFlow(ctx context.Context) error {
	if n.Hooks.VerifyTokens == nil { return errors.New("Keycloak token grant verification unavailable") }
	return n.Hooks.VerifyTokens(ctx)
}
func (n *NativeOps) RestoreBackup(ctx context.Context, backup providerupgrade.BackupRef, version string) error {
	if n.Hooks.Restore == nil { return errors.New("Keycloak coordinated SQL/config restore unavailable") }
	return n.Hooks.Restore(ctx, backup, version)
}
