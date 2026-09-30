package contract

import (
	"context"
	"time"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

// InternalWorkloadProvider is BaseHarbor Core's in-process runtime lifecycle
// seam. It is deliberately internal and MUST NOT be treated as the public,
// versioned Runtime Provider protocol.
//
// External runtime providers require a separate language-neutral, versioned
// contract (gRPC/Protocol Buffers at a process boundary) and an adapter into
// this seam. Runtime, capability and delivery provider protocols remain
// independent concerns.
//
// This interface contains only the minimal workload lifecycle semantics not
// already defined by OCI, Compose or a concrete runtime API. Implementations
// translate the provider-neutral WorkloadPlan into their native runtime API.
type InternalWorkloadProvider interface {
	Provider

	TargetScope() string
	Apply(context.Context, runtimemodel.WorkloadPlan) error
	WaitReady(context.Context, runtimemodel.WorkloadPlan, time.Duration) error
	Observe(context.Context, string, string, string) (runtimemodel.Observation, error)
	Logs(context.Context, string, string, string, string, int) (string, error)
	Exec(context.Context, string, string, string, string, ...string) (string, error)
	Destroy(context.Context, string, string, string) error
}
