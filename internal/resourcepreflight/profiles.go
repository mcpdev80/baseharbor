package resourcepreflight

// Profile is implementation metadata calibrated from BaseHarbor reference-stack
// measurements. It is deliberately not part of portable Application Intent.
type Profile struct {
	ID             string
	SteadyBytes    uint64
	StartupBytes   uint64
	Confidence     Confidence
	Source         string
}

var ReferenceProfiles = map[string]Profile{
	"keycloak": {
		ID: "keycloak", SteadyBytes: 691 * MiB, StartupBytes: 1457 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"otel-collector": {
		ID: "otel-collector", SteadyBytes: 398 * MiB, StartupBytes: 398 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"grafana-alloy": {
		ID: "grafana-alloy", SteadyBytes: 437 * MiB, StartupBytes: 437 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"loki": {
		ID: "loki", SteadyBytes: 213 * MiB, StartupBytes: 213 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"tempo": {
		ID: "tempo", SteadyBytes: 160 * MiB, StartupBytes: 160 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"prometheus": {
		ID: "prometheus", SteadyBytes: 126 * MiB, StartupBytes: 126 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"pgadmin": {
		ID: "pgadmin", SteadyBytes: 276 * MiB, StartupBytes: 276 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 full-stack convergence trace",
	},
	"seaweedfs-group": {
		ID: "seaweedfs-group", SteadyBytes: 180 * MiB, StartupBytes: 240 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 grouped SeaweedFS measurement",
	},
	"runtime-broker": {
		ID: "runtime-broker", SteadyBytes: 8 * MiB, StartupBytes: 8 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 convergence trace",
	},
	"runtime-executor": {
		ID: "runtime-executor", SteadyBytes: 13 * MiB, StartupBytes: 13 * MiB,
		Confidence: ConfidenceKnownBaseline, Source: "#549 v0.4.17 convergence trace",
	},
	"postgresql": {
		ID: "postgresql", SteadyBytes: 64 * MiB, StartupBytes: 96 * MiB,
		Confidence: ConfidenceEstimated, Source: "#549 grouped PostgreSQL calibration; conservative per-instance estimate pending isolated remeasurement",
	},
	"valkey": {
		ID: "valkey", SteadyBytes: 64 * MiB, StartupBytes: 96 * MiB,
		Confidence: ConfidenceEstimated, Source: "#549 grouped backend calibration; isolated provider measurement pending",
	},
	"openbao": {
		ID: "openbao", SteadyBytes: 64 * MiB, StartupBytes: 96 * MiB,
		Confidence: ConfidenceEstimated, Source: "#549 grouped control-plane calibration; isolated provider measurement pending",
	},
	"developer-gateway": {
		ID: "developer-gateway", SteadyBytes: 64 * MiB, StartupBytes: 96 * MiB,
		Confidence: ConfidenceEstimated, Source: "#549 grouped gateway calibration; isolated provider measurement pending",
	},
}

func RequirementForProfile(id, scope string, alreadyRunning bool) (Requirement, bool) {
	p, ok := ReferenceProfiles[id]
	if !ok {
		return Requirement{}, false
	}
	minimum := uint64(0)
	if p.Confidence == ConfidenceKnownBaseline {
		minimum = p.SteadyBytes
	}
	return Requirement{
		ID: "provider/" + p.ID, Kind: "provider", Scope: scope,
		MinimumBytes: minimum, EstimatedBytes: p.StartupBytes,
		Confidence: p.Confidence, Source: p.Source, AlreadyRunning: alreadyRunning,
	}, true
}
