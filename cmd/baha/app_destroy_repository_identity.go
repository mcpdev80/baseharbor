package main

import "github.com/mcpdev80/baseharbor/internal/application"

// applicationDestroyIdentityIDs collects repository identities that may still
// be referenced by Target deployment and provider-owner records after destroy.
func applicationDestroyIdentityIDs(manifest application.Manifest, snapshot application.IdentitySnapshot) map[string]bool {
	ids := make(map[string]bool, len(snapshot.ApplicationIDs)+1)
	if manifest.ApplicationID != "" {
		ids[manifest.ApplicationID] = true
	}
	for _, id := range snapshot.ApplicationIDs {
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}
