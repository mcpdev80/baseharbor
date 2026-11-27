package application

import "time"

// StatusTimeoutMatrix is the single documented source of truth for the
// per-application status-probe timeouts the CLI uses when collecting runtime
// status. It replaces the previously scattered per-service literals in
// cmd/baha so every probe reads one value from one group instead of a
// duplicated magic number.
//
// The matrix is intentionally literal-only (no runtime-semantic change):
// each value is identical to the literal it supplants, so behaviour is
// preserved exactly. The objectstorage readiness group
// (providerReadinessTimeout / sharedProviderReconcileTimeout /
// existingProviderProbeTimeout in internal/objectstorage/seaweedfs.go) is the
// sibling matrix for provider-side probes; this group covers the CLI's own
// application-status probes.
//
// Semantic groups (one documented constant per group, behavior-neutral):
//
//	OverallTimeout — whole status collection (matches provider reconcile budget)
//	RequiredSecretTimeout — required-secret resolution
//	ManagementServiceTimeout — management services (OpenBao, RabbitMQ, MongoDB)
//	PostgresStatusTimeout — PostgreSQL probe
//	FastServiceTimeout — fast services (Broker, Valkey, Logs)
const (
	OverallTimeout           = 120 * time.Second
	RequiredSecretTimeout    = 40 * time.Second
	ManagementServiceTimeout = 30 * time.Second
	PostgresStatusTimeout    = 15 * time.Second
	FastServiceTimeout       = 10 * time.Second
)
