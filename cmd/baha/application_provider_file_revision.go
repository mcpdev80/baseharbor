package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Bind acknowledgement to protected file content, not just size/mtime. Digests
// remain internal to the aggregate revision; no secret or per-file digest is
// exposed through material evidence. This reads existing owners only.
func applicationImpactFileRevision(observation application.ProviderImpactObservation) (string, error) {
	revisions := map[string]string{}
	var failures []error
	for _, fact := range observation.Providers {
		for _, material := range fact.Materials {
			if !material.Verified || material.Ownership != capability.OwnershipBaseHarbor || material.Location == nil || material.Location.Kind != "path" {
				continue
			}
			path := material.Location.Reference
			if _, seen := revisions[path]; seen {
				continue
			}
			if len(revisions) >= 10000 {
				return "", errors.New("provider content revision exceeds bounded inspection")
			}
			revisions[path] = "unverified"
			data, err := runtimeMutationOwnerBytes(path, false)
			if err != nil {
				failures = append(failures, runtimeMutationOwnerError())
				continue
			}
			digest := sha256.Sum256(data)
			zeroBytes(data)
			revisions[path] = hex.EncodeToString(digest[:])
		}
	}
	raw, err := json.Marshal(revisions)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), errors.Join(failures...)
}
