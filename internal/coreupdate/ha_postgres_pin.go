package coreupdate

import "strings"

// ClassifyHAPostgresPin binds the running Spilo/Patroni database to its
// release-owned HA image identity. A matching tag without a matching immutable
// digest is never sufficient evidence to replace or restart a data-bearing
// member. PostgreSQL HA image changes remain unsupported until the cluster
// migration and durable recovery contract has explicit runtime evidence.
func ClassifyHAPostgresPin(installed Realization, pin BackingPin) Delta {
	desired := Desired{Kind: SQL, Image: pin.Image, Digest: pin.Digest, Version: pin.Version}
	delta := Delta{
		Installed:      installed,
		Desired:        desired,
		Classification: Unsupported,
		Reason:         "Core HA PostgreSQL Spilo update requires a verified Patroni rolling migration, backup and recovery contract",
	}
	digest := strings.TrimSpace(installed.Digest)
	if strings.HasPrefix(digest, "sha256:") && validDigest(digest) &&
		installed.Kind == SQL && installed.Owner == "baseharbor" &&
		installed.Image == pin.Image && digest == pin.Digest &&
		strings.TrimSpace(pin.Version) != "" && validDigest(pin.Digest) {
		delta.Installed.Version = pin.Version
		delta.Classification = NoChange
		delta.Reason = ""
	}
	return delta
}
