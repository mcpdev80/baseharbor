package model

import "github.com/mcpdev80/baseharbor/internal/workload"

// Target identifies the deployment-owned runtime scope selected for a
// workload. Scope is deliberately provider-neutral: Kubernetes realizes it as
// a namespace, while other runtimes may realize an equivalent isolation scope
// differently.
type Target struct {
	Scope string
}

// Binding is one resolved application-facing runtime value. Sensitive values
// remain protected runtime data and must never be persisted into portable
// application intent.
type Binding struct {
	Value     string
	Sensitive bool
}

// WorkloadPlan is the provider-neutral handoff from BaseHarbor Core to a
// runtime provider. Runtime-specific objects are derived after this boundary.
type WorkloadPlan struct {
	Application string
	Environment string
	Target      Target
	Workload    workload.Model

	// Images maps logical workload service -> resolved OCI image reference.
	// Source build configuration is resolved before the runtime boundary.
	Images map[string]string

	Bindings map[string]Binding
}
