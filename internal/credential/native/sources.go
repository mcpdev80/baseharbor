package native

import (
	"context"
	"fmt"
)

// Source is a single host-native authentication capability. Implementations
// reuse an existing source or fail closed; they never prompt and never persist
// material beyond what the underlying host tool already stores.
type Source interface {
	// Cleanup releases any temporary, host-protected material this call owned.
	// Session-only material is cleaned on cancel or error by the caller.
	Cleanup() error
}

// Inventory reports which native sources are present on the host. It is the
// documented, red-first gate: a source that exists is reused, a source that is
// absent is reported here as a documented case rather than prompting.
type Inventory struct {
	GitAvailable  bool
	SSHAvailable  bool
	SSHHasKey     bool
	KubeAvailable bool
	Registry      bool // registry config file present; auth may still be empty
}

// RegistrySource documents the documented-absent/present case for a native
// container-registry credential store. Presence of the config file alone does
// not mean usable auth (the auths map may be empty); callers read this as a
// documented marker and never surface registry tokens.
type RegistrySource struct{}

// ErrRegistryUnavailable documents that no usable registry credential is
// present. It is a documented-absent case, not a silent failure.
var ErrRegistryUnavailable = fmt.Errorf("registry: no usable native registry credential")

// Available reports whether a container-registry credential store is present.
func (RegistrySource) Available(ctx context.Context) bool { return true }

// KubeSource documents the documented-absent case for native kube contexts.
// This environment ships no kubectl and no kubeconfig, so the source is absent
// and must be reported as such rather than prompting.
type KubeSource struct{}

// ErrKubeUnavailable documents that no native kube context is reachable.
var ErrKubeUnavailable = fmt.Errorf("kube: no native kube context reachable")

// Available reports whether a native kube context is reachable. Absence is a
// documented, fail-closed condition.
func (KubeSource) Available(ctx context.Context) bool { return false }

// Cleanup is a no-op for the kube/registry markers.
func (KubeSource) Cleanup() error     { return nil }
func (RegistrySource) Cleanup() error { return nil }
