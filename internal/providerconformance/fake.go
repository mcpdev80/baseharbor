package providerconformance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/reconciliation"
)

type FailureMode string

const (
	FailureNone             FailureMode = ""
	FailureUnavailable      FailureMode = "unavailable"
	FailureProvision        FailureMode = "provision"
	FailureMalformedBinding FailureMode = "malformed-binding"
	FailureVerify           FailureMode = "verify"
)

type FakeDriver struct {
	mu          sync.Mutex
	provider    capability.Provider
	mode        FailureMode
	owner       string
	resource    capability.Resource
	created     bool
	drifted     bool
	bound       bool
	verified    bool
	creates     int
	noops       int
	repairs     int
	destroys    int
	credential  string
	lastBinding capability.Binding
	reconciliationOwner reconciliation.Ownership
	conflict bool
	unsupported bool
	degraded bool
}

func NewFakeDriver(provider capability.Provider) *FakeDriver {
	return &FakeDriver{provider: provider, credential: "fake-secret-never-diagnostic", reconciliationOwner: reconciliation.OwnershipBaseHarbor}
}

func (d *FakeDriver) Descriptor() capability.Provider { return d.provider }

func (d *FakeDriver) DesiredState(resource capability.Resource, binding capability.Binding) reconciliation.Desired {
	return reconciliation.Desired{
		Exists: true,
		Digest: desiredDigest(resource, binding),
		Owner:  reconciliation.OwnershipBaseHarbor,
	}
}

func (d *FakeDriver) Observe(_ context.Context, resource capability.Resource, binding capability.Binding) (reconciliation.Observed, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	observed := reconciliation.Observed{
		Exists:      d.created,
		Owner:       d.reconciliationOwner,
		Conflict:    d.conflict,
		Unsupported: d.unsupported,
		Degraded:    d.degraded,
	}
	if d.created {
		if d.drifted {
			observed.Digest = "drifted"
		} else {
			observed.Digest = desiredDigest(resource, binding)
		}
	}
	return observed, nil
}

func (d *FakeDriver) SetReconciliationOwnership(owner reconciliation.Ownership) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reconciliationOwner = owner
}

func (d *FakeDriver) SetConflict(value bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conflict = value
}

func (d *FakeDriver) SetUnsupported(value bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unsupported = value
}

func (d *FakeDriver) SetDegraded(value bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.degraded = value
}

func desiredDigest(resource capability.Resource, binding capability.Binding) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", resource.Application, resource.Kind, resource.Name, resource.Provider, binding.Workload)
}

func (d *FakeDriver) SetFailure(mode FailureMode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mode = mode
}

func (d *FakeDriver) Preflight(context.Context, capability.Resource, capability.Binding) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mode == FailureUnavailable {
		return errors.New("provider unavailable")
	}
	return nil
}

func (d *FakeDriver) Provision(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mode == FailureUnavailable || d.mode == FailureProvision {
		return errors.New("provider provisioning failed")
	}
	if !d.created {
		d.created = true
		d.owner = resource.Application
		d.resource = resource
		d.creates++
		d.drifted = false
		return nil
	}
	if d.drifted {
		d.drifted = false
		d.repairs++
		return nil
	}
	d.noops++
	return nil
}

func (d *FakeDriver) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mode == FailureMalformedBinding {
		return errors.New("binding rejected: malformed provider binding")
	}
	if !d.created || d.resource != resource {
		return errors.New("binding requested before owned resource exists")
	}
	d.bound = true
	d.lastBinding = binding
	return nil
}

func (d *FakeDriver) Verify(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mode == FailureVerify {
		return errors.New("provider verification failed")
	}
	if !d.created || d.drifted || !d.bound || d.resource != resource {
		return errors.New("provider resource is not ready")
	}
	d.verified = true
	return nil
}

func (d *FakeDriver) ConformanceDrift() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.created {
		d.drifted = true
		d.verified = false
	}
}

func (d *FakeDriver) ConformanceDestroy(_ context.Context, resource capability.Resource) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.created {
		return nil
	}
	if resource.Application != d.owner || resource != d.resource {
		return fmt.Errorf("refusing to destroy resource not owned by application %q", resource.Application)
	}
	d.created = false
	d.bound = false
	d.verified = false
	d.destroys++
	return nil
}

func (d *FakeDriver) ConformanceStateDigest() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Operational counters are intentionally excluded: idempotency is about
	// stable resource identity/state, not the number of observed reconciles.
	return fmt.Sprintf("owner=%s|resource=%s/%s/%s|created=%t|drifted=%t|bound=%t|verified=%t",
		d.owner, d.resource.Application, d.resource.Kind, d.resource.Name, d.created, d.drifted, d.bound, d.verified)
}

type FakeStats struct {
	Creates  int
	Noops    int
	Repairs  int
	Destroys int
}

func (d *FakeDriver) Stats() FakeStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return FakeStats{Creates: d.creates, Noops: d.noops, Repairs: d.repairs, Destroys: d.destroys}
}

func (d *FakeDriver) DiagnosticSafe(err error) bool {
	if err == nil {
		return true
	}
	return !strings.Contains(err.Error(), d.credential)
}
