package capability

import (
	"fmt"
	"strings"
)

type SpecificationVersion string

const SpecificationV1 SpecificationVersion = "v1"

type SpecificationID string

type CapabilitySpecification struct {
	ID      SpecificationID      `json:"id"`
	Kind    Kind                 `json:"kind"`
	Version SpecificationVersion `json:"version"`
}

var (
	SQLV1 = CapabilitySpecification{ID: "database.sql/v1", Kind: SQL, Version: SpecificationV1}
	KeyValueV1 = CapabilitySpecification{ID: "cache.key-value/v1", Kind: KeyValue, Version: SpecificationV1}
	SecretsV1 = CapabilitySpecification{ID: "secrets/v1", Kind: Secrets, Version: SpecificationV1}
	ExposureHTTPV1 = CapabilitySpecification{ID: "exposure.http/v1", Kind: ExposureHTTP, Version: SpecificationV1}
	ObjectStorageS3V1 = CapabilitySpecification{ID: "object-storage.s3/v1", Kind: ObjectStorageS3, Version: SpecificationV1}
)

func SpecificationForKind(kind Kind) (CapabilitySpecification, error) {
	switch kind {
	case SQL:
		return SQLV1, nil
	case KeyValue:
		return KeyValueV1, nil
	case Secrets:
		return SecretsV1, nil
	case ExposureHTTP:
		return ExposureHTTPV1, nil
	case ObjectStorageS3:
		return ObjectStorageS3V1, nil
	default:
		return CapabilitySpecification{}, fmt.Errorf("capability specification for %q is not defined", kind)
	}
}

func ParseSpecificationID(id SpecificationID) (CapabilitySpecification, error) {
	value := strings.TrimSpace(string(id))
	if value == "" {
		return CapabilitySpecification{}, fmt.Errorf("capability specification id is required")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return CapabilitySpecification{}, fmt.Errorf("invalid capability specification id %q", value)
	}
	version := SpecificationVersion(parts[1])
	if version != SpecificationV1 {
		return CapabilitySpecification{}, fmt.Errorf("unsupported capability specification version %q", version)
	}
	spec, err := SpecificationForKind(Kind(parts[0]))
	if err != nil {
		return CapabilitySpecification{}, err
	}
	if spec.ID != id {
		return CapabilitySpecification{}, fmt.Errorf("capability specification id %q is not canonical", value)
	}
	return spec, nil
}
