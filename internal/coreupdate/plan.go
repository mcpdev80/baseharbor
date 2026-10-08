// Package coreupdate defines the provider-neutral, fail-closed Core update plan.
// Runtime mutation is injected through the existing Core/provider lifecycle.
package coreupdate

import (
 "context"
 "errors"
 "fmt"
 "strings"
)

type Classification string

const (
 NoChange Classification = "no_change"
 SafeReconcile Classification = "safe_reconcile"
 BackupRequired Classification = "backup_recovery_required"
 MigrationRequired Classification = "migration_required"
 Unsupported Classification = "unsupported"
)

type ProviderKind string
const (
 SQL ProviderKind = "sql"
 Secrets ProviderKind = "secrets"
 Identity ProviderKind = "identity"
)

type Desired struct {
 Kind ProviderKind `json:"kind"`
 Image string `json:"image"`
 Digest string `json:"digest"`
 Version string `json:"version"`
}

type Realization struct {
 Kind ProviderKind `json:"kind"`
 Installation string `json:"installation"`
 Scope string `json:"scope"`
 Instance string `json:"instance"`
 Owner string `json:"owner"`
 Image string `json:"image"`
 Digest string `json:"digest"`
 Version string `json:"version"`
}

type Delta struct {
 Installed Realization `json:"installed"`
 Desired Desired `json:"desired"`
 Classification Classification `json:"classification"`
 Reason string `json:"reason,omitempty"`
}

type Plan struct {
 Release string `json:"release"`
 Deltas []Delta `json:"deltas"`
}

func validKind(kind ProviderKind) bool {
 return kind == SQL || kind == Secrets || kind == Identity
}

func Build(release string, installed []Realization, desired []Desired) (Plan, error) {
 if strings.TrimSpace(release) == "" { return Plan{}, errors.New("Core update requires a pinned BaseHarbor release") }
 references := make(map[ProviderKind]Desired,3)
 for _, item := range desired {
  if !validKind(item.Kind) || references[item.Kind].Kind != "" {
   return Plan{}, fmt.Errorf("invalid or duplicated Core release provider kind %q", item.Kind)
  }
  if item.Version == "" || item.Image == "" || !strings.HasPrefix(item.Digest, "sha256:") || len(item.Digest) != 71 {
   return Plan{}, fmt.Errorf("Core release provider %s lacks immutable image/version identity", item.Kind)
  }
  references[item.Kind] = item
 }
 for _, kind := range []ProviderKind{SQL,Secrets,Identity} {
  if _, ok := references[kind]; !ok { return Plan{}, fmt.Errorf("Core release lacks mandatory %s provider",kind) }
 }
 seen := make(map[string]bool)
 plan := Plan{Release:release}
 for _, current := range installed {
  if !validKind(current.Kind) || current.Installation == "" || current.Scope == "" || current.Instance == "" {
   return Plan{}, errors.New("invalid Core realization identity")
  }
  key := current.Installation + "/" + current.Scope + "/" + current.Instance + "/" + string(current.Kind)
  if seen[key] { return Plan{}, fmt.Errorf("duplicate Core realization %s",key) }
  seen[key] = true
  if current.Owner != "baseharbor" { continue } // External providers cannot be mutated.
  target := references[current.Kind]
  delta := Delta{Installed:current, Desired:target}
  switch {
  case current.Digest != "" && current.Digest == target.Digest && current.Version == target.Version:
   delta.Classification = NoChange
  case current.Image == "" || current.Version == "":
   delta.Classification, delta.Reason = Unsupported, "installed provider identity is unverifiable"
  case current.Kind == SQL && strings.Split(current.Version, ".")[0] != strings.Split(target.Version, ".")[0]:
   delta.Classification, delta.Reason = Unsupported, "PostgreSQL major upgrade requires an explicit supported migration"
  case current.Kind == SQL || current.Kind == Secrets:
   delta.Classification = BackupRequired
  case current.Kind == Identity:
   delta.Classification = MigrationRequired
  default:
   delta.Classification = Unsupported
  }
  plan.Deltas = append(plan.Deltas,delta)
 }
 return plan,nil
}

type Hooks struct {
 Preflight func(context.Context, Plan) error
 RecoveryPoint func(context.Context, Delta) error
 Apply func(context.Context, Delta) error
 Verify func(context.Context, Delta) error
 Record func(context.Context, Delta, string) error
}

// Execute does not infer that a data migration can be rolled back. All
// classifications are checked before any provider receives a mutation.
func Execute(ctx context.Context, plan Plan, hooks Hooks) error {
 if hooks.Preflight == nil || hooks.Apply == nil || hooks.Verify == nil || hooks.Record == nil {
  return errors.New("Core update requires complete preflight, apply, verification and journal hooks")
 }
 needsRecovery := false
 for _, delta := range plan.Deltas {
  switch delta.Classification {
  case NoChange, SafeReconcile:
  case BackupRequired, MigrationRequired:
   needsRecovery = true
  default:
   return fmt.Errorf("Core provider %s update is unsupported: %s",delta.Installed.Kind,delta.Reason)
  }
 }
 if needsRecovery && hooks.RecoveryPoint == nil {
  return errors.New("Core provider update requires a verified recovery-point implementation")
 }
 if err := hooks.Preflight(ctx,plan); err != nil { return fmt.Errorf("Core update preflight: %w",err) }
 for _, delta := range plan.Deltas {
  if delta.Classification == NoChange { continue }
  if err := ctx.Err(); err != nil { return err }
  if delta.Classification == BackupRequired || delta.Classification == MigrationRequired {
   if err := hooks.RecoveryPoint(ctx,delta); err != nil { return fmt.Errorf("Core recovery point %s: %w",delta.Installed.Instance,err) }
  }
  if err := hooks.Record(ctx,delta,"applying"); err != nil { return err }
  if err := hooks.Apply(ctx,delta); err != nil {
   _ = hooks.Record(ctx,delta,"apply_failed")
   return fmt.Errorf("Core update %s: %w",delta.Installed.Instance,err)
  }
  if err := hooks.Verify(ctx,delta); err != nil {
   _ = hooks.Record(ctx,delta,"verify_failed")
   return fmt.Errorf("Core verification %s: %w",delta.Installed.Instance,err)
  }
  if err := hooks.Record(ctx,delta,"verified"); err != nil { return err }
 }
 return nil
}
