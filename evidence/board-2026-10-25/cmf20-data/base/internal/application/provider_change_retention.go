package application

// Durable material and meaningful transient state are different. An observed
// volume alone does not prove that sessions, queues or in-flight work survive.
type MaterialRetention struct {
	SurvivesStop           bool   `json:"survives_stop"`
	SurvivesRuntimeRemoval bool   `json:"survives_runtime_removal"`
	Verified               bool   `json:"verified"`
	Evidence               string `json:"evidence,omitempty"`
}

func meaningfulTransient(kind string) bool {
	switch kind {
	case "cache", "session", "job", "queue", "message", "in-flight", "telemetry-buffer", "lease", "token":
		return true
	}
	return false
}

func transientMaterialDisposition(m ProviderMaterial, op string, d MaterialDisposition) (MaterialDisposition, string) {
	// Local unbinding changes delivery, not the foreign/shared running store.
	if op == "unbind" || op == "disable" || op == "rebind" {
		return d, "material remains at its observed store; selected consumer access changes"
	}
	if m.Retention == nil || !m.Retention.Verified || m.Retention.Evidence == "" {
		return MaterialDisposition{MaterialID: m.ID, Disposition: "unknown"}, "survival of meaningful transient state is unverified; a volume alone does not prove recoverability"
	}
	survives := m.Retention.SurvivesStop
	if op == "destroy" || op == "remove-runtime" || op == "replace" {
		survives = m.Retention.SurvivesRuntimeRemoval
	}
	if survives {
		return d, "observed retention mechanism preserves material at its existing location; delivery still requires a running consumer/provider"
	}
	return MaterialDisposition{MaterialID: m.ID, Disposition: "nonrecoverable", Supported: true, Verified: true, Evidence: m.Retention.Evidence}, "observed lifecycle loses this transient material; no destination or recoverable continuation"
}
