package applicationbackup

import (
	"errors"

	"github.com/mcpdev80/baseharbor/internal/application"
)

const applicationManifestEntry = "metadata/baseharbor.yaml"

func ApplicationManifestPayloadEntry(m application.Manifest) (PayloadEntry, error) {
	if err := m.Validate(); err != nil {
		return PayloadEntry{}, err
	}
	return PayloadEntry{Name: applicationManifestEntry, Data: []byte(m.YAML())}, nil
}

func ApplicationManifestFromPayload(payload Payload) (application.Manifest, error) {
	var raw []byte
	for _, entry := range payload.Entries {
		if entry.Name != applicationManifestEntry {
			continue
		}
		if raw != nil {
			return application.Manifest{}, errors.New("backup contains duplicate application manifest entry")
		}
		raw = entry.Data
	}
	if raw == nil {
		return application.Manifest{}, errors.New("backup does not contain application manifest metadata")
	}
	m, err := application.ParseYAML(string(raw))
	if err != nil {
		return application.Manifest{}, errors.New("backup application manifest is invalid")
	}
	if m.Name != payload.Manifest.Application || m.Environment != payload.Manifest.Environment {
		return application.Manifest{}, errors.New("backup application manifest identity does not match archive identity")
	}
	return m, nil
}
