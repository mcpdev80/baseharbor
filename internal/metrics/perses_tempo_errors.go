package metrics

import "fmt"

// Tempo selection / ownership / authorization / shape fail-closed errors. Each
// names the concrete reason a Tempo path cannot yield a Perses datasource, so
// callers and tests assert the exact verdict rather than a fabricated resource.

// TempoSurfaceNotSelectedError is returned when Perses is not selected.
type TempoSurfaceNotSelectedError struct{}

func (*TempoSurfaceNotSelectedError) Error() string {
	return "the Perses Tempo datasource is not configured: the observability management UI and services.perses must both be selected, so this fails closed rather than fabricating a datasource"
}

// TempoNotSelectedError is returned when the Traces signal is not selected.
type TempoNotSelectedError struct{}

func (*TempoNotSelectedError) Error() string {
	return "the Perses Tempo datasource cannot be built: the Traces signal is not selected, so no Tempo-owned traces path exists to visualise"
}

// TempoSourceAbsentError is returned when the Tempo provider is not provisioned.
type TempoSourceAbsentError struct{}

func (*TempoSourceAbsentError) Error() string {
	return "the Perses Tempo datasource cannot be built: no materialized Tempo provider exists for this Target, so no datasource may be fabricated"
}

// TempoSourceExternalError is returned for an externally-owned Tempo source.
type TempoSourceExternalError struct{}

func (*TempoSourceExternalError) Error() string {
	return "the Perses Tempo datasource cannot be built: the selected Tempo placement is external, and BaseHarbor owns no authorization boundary over it"
}

// TempoEndpointAbsentError is returned when the Tempo endpoint is not
// materialized or is not a usable URL.
type TempoEndpointAbsentError struct {
	Detail string
}

func (e *TempoEndpointAbsentError) Error() string {
	return fmt.Sprintf("the Perses Tempo datasource cannot be built: the Tempo management endpoint is not usable (%s)", e.Detail)
}

// TempoAuthorizationError is returned when service access or the traces
// collection policy does not authorize the Tempo path.
type TempoAuthorizationError struct {
	Detail string
}

func (e *TempoAuthorizationError) Error() string {
	return fmt.Sprintf("the Perses Tempo datasource cannot be built: the Tempo path is not authorized (%s)", e.Detail)
}

// TempoHealthUnverifiedError is the fail-closed verdict when the authorized
// trace query completed cleanly but answered with no trace.
type TempoHealthUnverifiedError struct {
	Detail string
}

func (e *TempoHealthUnverifiedError) Error() string {
	return fmt.Sprintf("the Perses Tempo datasource stays unverified: the authorized TraceQL query produced no trace (%s)", e.Detail)
}

// TempoResourceShapeError is returned when an emitted Perses resource deviates
// from the pinned upstream contract.
type TempoResourceShapeError struct {
	Detail string
}

func (e *TempoResourceShapeError) Error() string {
	return fmt.Sprintf("the emitted Perses Tempo resource is not native (%s): only the pinned Perses resource envelope and plugin kinds are provisioned", e.Detail)
}

// TempoWitnessError is the fail-closed verdict when the pinned upstream model
// witness itself cannot be trusted: a drifted file hash, a mirrored field the
// upstream source does not declare, or a native document the pinned release
// does not ship. Validation never proceeds on a tampered witness.
type TempoWitnessError struct {
	Detail string
}

func (e *TempoWitnessError) Error() string {
	return fmt.Sprintf("the pinned Perses v0.55.1 model witness is unusable (%s): validation must fail closed against the upstream source witness", e.Detail)
}
