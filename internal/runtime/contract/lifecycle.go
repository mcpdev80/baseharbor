package contract

import (
	"context"
	"time"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

// InternalWorkloadProvider is BaseHarbor Core's in-process semantic runtime
// lifecycle seam. It is deliberately smaller than the existing local
// Docker/Podman project/file/container mechanics and MUST NOT be treated as the
// public external Runtime Provider protocol.
//
// External providers use the versioned language-neutral
// baseharbor.runtime.provider/v1 protocol and adapt into equivalent semantics.
type InternalWorkloadProvider interface {
	Provider

	TargetScope() string
	Preflight(context.Context, runtimemodel.WorkloadPlan) error
	Apply(context.Context, runtimemodel.WorkloadPlan) error
	WaitReady(context.Context, runtimemodel.WorkloadPlan, time.Duration) error
	Observe(context.Context, string, string, string) (runtimemodel.Observation, error)
	Logs(context.Context, string, string, string, string, int) (string, error)
	Exec(context.Context, string, string, string, string, ...string) (string, error)
	Destroy(context.Context, string, string, string) error
}
