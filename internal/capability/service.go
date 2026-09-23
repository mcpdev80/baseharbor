package capability

import "fmt"

// ServiceKind is the provider-neutral application service family. It is
// intentionally broader than a capability/protocol specification and never
// identifies a concrete provider product.
type ServiceKind string

const (
	ServiceSQL           ServiceKind = "sql"
	ServiceCache         ServiceKind = "cache"
	ServiceObjectStorage ServiceKind = "object-storage"
	ServiceSecrets       ServiceKind = "secrets"
	ServiceObservability ServiceKind = "observability"
	ServiceExposure      ServiceKind = "exposure"
)

// ServiceKindForCapability maps shipped v0.4 capability specifications onto
// the standards-first service family without renaming the existing capability
// compatibility surface.
func ServiceKindForCapability(kind Kind) (ServiceKind, error) {
	switch kind {
	case SQL:
		return ServiceSQL, nil
	case KeyValue:
		return ServiceCache, nil
	case ObjectStorageS3:
		return ServiceObjectStorage, nil
	case Secrets:
		return ServiceSecrets, nil
	case TelemetryOTLP, Metrics, Logs, Traces:
		return ServiceObservability, nil
	case ExposureHTTP:
		return ServiceExposure, nil
	default:
		return "", fmt.Errorf("service kind for capability %q is not defined", kind)
	}
}
