package capability

import "fmt"

// ServiceKind is the provider-neutral application service family. It is
// intentionally broader than a capability/protocol specification and never
// identifies a concrete provider product.
type ServiceKind string

const (
	ServiceSQL              ServiceKind = "sql"
	ServiceCache            ServiceKind = "cache"
	ServiceKeyValue         ServiceKind = "key-value"
	ServiceDocumentDatabase ServiceKind = "document-database"
	ServiceObjectStorage    ServiceKind = "object-storage"
	ServiceSecrets          ServiceKind = "secrets"
	ServiceObservability    ServiceKind = "observability"
	ServiceIdentity         ServiceKind = "identity"
	ServiceMessaging        ServiceKind = "messaging"
	ServiceVector           ServiceKind = "vector"
	ServiceExposure         ServiceKind = "exposure"
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
	case DurableKeyValue:
		return ServiceKeyValue, nil
	case DocumentDatabase:
		return ServiceDocumentDatabase, nil
	case ObjectStorageS3:
		return ServiceObjectStorage, nil
	case Secrets:
		return ServiceSecrets, nil
	case TelemetryOTLP, Metrics, Logs, Traces:
		return ServiceObservability, nil
	case ExposureHTTP:
		return ServiceExposure, nil
	case Identity:
		return ServiceIdentity, nil
	case MessagingQueue, MessagingPubSub, MessagingStream:
		return ServiceMessaging, nil
	default:
		return "", fmt.Errorf("service kind for capability %q is not defined", kind)
	}
}
