package capability

import "testing"

func TestServiceKindForCapability(t *testing.T) {
	tests := map[Kind]ServiceKind{
		SQL:             ServiceSQL,
		KeyValue:        ServiceCache,
		ObjectStorageS3: ServiceObjectStorage,
		Secrets:         ServiceSecrets,
		TelemetryOTLP:   ServiceObservability,
		Metrics:         ServiceObservability,
		Logs:            ServiceObservability,
		Traces:          ServiceObservability,
		ExposureHTTP:    ServiceExposure,
	}
	for capabilityKind, want := range tests {
		got, err := ServiceKindForCapability(capabilityKind)
		if err != nil {
			t.Fatalf("%s: %v", capabilityKind, err)
		}
		if got != want {
			t.Fatalf("%s: service=%s, want %s", capabilityKind, got, want)
		}
	}
}
