package applicationbackup

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	RecoveryManifestVersion = 1
	recoveryManifestEntry    = "metadata/recovery.json"
)

type RecoveryManifest struct {
	Version      int                   `json:"version"`
	Contributors []RecoveryContributor `json:"contributors"`
}

func RecoveryManifestPayloadEntry(selection RecoverySelection) (PayloadEntry, error) {
	manifest := RecoveryManifest{
		Version:      RecoveryManifestVersion,
		Contributors: append([]RecoveryContributor(nil), selection.Contributors...),
	}
	if err := manifest.Validate(); err != nil {
		return PayloadEntry{}, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return PayloadEntry{}, fmt.Errorf("encode recovery manifest: %w", err)
	}
	return PayloadEntry{Name: recoveryManifestEntry, Data: data}, nil
}

func RecoveryManifestFromPayload(payload Payload) (RecoveryManifest, bool, error) {
	var raw []byte
	for _, entry := range payload.Entries {
		if entry.Name != recoveryManifestEntry {
			continue
		}
		if raw != nil {
			return RecoveryManifest{}, false, errors.New("backup contains duplicate recovery manifest entry")
		}
		raw = entry.Data
	}
	if raw == nil {
		return RecoveryManifest{}, false, nil
	}
	var manifest RecoveryManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return RecoveryManifest{}, true, errors.New("backup recovery manifest is malformed")
	}
	if err := manifest.Validate(); err != nil {
		return RecoveryManifest{}, true, err
	}
	return manifest, true, nil
}

func (m RecoveryManifest) Validate() error {
	if m.Version != RecoveryManifestVersion {
		return fmt.Errorf("unsupported recovery manifest version %d", m.Version)
	}
	seen := make(map[string]struct{}, len(m.Contributors))
	for _, contributor := range m.Contributors {
		if err := contributor.Validate(); err != nil {
			return err
		}
		if contributor.Selected && contributor.Support != RecoverySupported {
			return fmt.Errorf("recovery contributor %s %q is selected but %s", contributor.StateClass, contributor.LogicalResource, contributor.Support)
		}
		key := string(contributor.StateClass) + "\x00" + contributor.LogicalResource
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate recovery contributor %s %q", contributor.StateClass, contributor.LogicalResource)
		}
		seen[key] = struct{}{}
	}
	return nil
}
