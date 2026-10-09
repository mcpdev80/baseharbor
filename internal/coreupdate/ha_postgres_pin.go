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
	// Allow only a pinned, same-PostgreSQL-major Spilo patch transition.
	// The caller must still run StageAndRollPatroniCluster with physical
	// recovery and verified DCS evidence before touching any member.
	if installed.Kind == SQL && installed.Owner == "baseharbor" &&
		installed.Scope == "shared" && installed.Instance == "postgres-member-1" &&
		validDigest(digest) && validDigest(pin.Digest) &&
		strings.HasPrefix(installed.Image, "ghcr.io/zalando/spilo-18:") {
		tag := strings.TrimPrefix(installed.Image, "ghcr.io/zalando/spilo-18:")
		previous := BackingPin{Role: pin.Role, Version: "18-spilo-" + tag, Image: installed.Image, Digest: digest}
		if previous.Version != pin.Version && safeSpiloTransition(previous, pin) && installed.Image != pin.Image {
			delta.Installed.Version = previous.Version
			delta.Classification = BackupRequired
			delta.Reason = "verified HA Spilo rolling update requires physical backup, DCS checkpoint and member journal"
		}
	}
	return delta
}
