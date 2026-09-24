package contract

import (
	"context"
	"time"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

// WorkloadProvider is the minimal semantic workload lifecycle not already
// covered by OCI, Compose or Kubernetes standards. Implementations translate
// the provider-neutral plan into their native runtime API.
type WorkloadProvider interface {
	Provider

	TargetScope() string
	Apply(context.Context, runtimemodel.WorkloadPlan) error
	WaitReady(context.Context, runtimemodel.WorkloadPlan, time.Duration) error
	Observe(context.Context, string, string, string) (runtimemodel.Observation, error)
	Logs(context.Context, string, string, string, string, int) (string, error)
	Exec(context.Context, string, string, string, string, ...string) (string, error)
	Destroy(context.Context, string, string, string) error
}
