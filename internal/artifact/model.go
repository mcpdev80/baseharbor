package artifact

// Artifact is an OCI image artifact resolved for one workload service.
// Reference is the runtime-consumable image reference. Digest is populated
// when the resolver/builder can prove immutable artifact identity.
type Artifact struct {
	Service   string
	Reference string
	Digest    string
}

// BuildRequest is provider-neutral source-to-OCI-artifact input. It describes
// source that needs packaging, not how a particular builder must execute.
type BuildRequest struct {
	Service       string
	Context       string
	Dockerfile    string
	ReferenceHint string
}

// Resolution separates already-consumable artifacts from source that still
// requires an artifact builder.
type Resolution struct {
	Artifacts []Artifact
	Builds    []BuildRequest
}

// Complete reports whether every workload service already has a consumable
// runtime artifact.
func (r Resolution) Complete() bool {
	return len(r.Builds) == 0
}

// Images returns service -> OCI reference for resolved artifacts.
func (r Resolution) Images() map[string]string {
	result := make(map[string]string, len(r.Artifacts))
	for _, item := range r.Artifacts {
		if item.Reference != "" {
			result[item.Service] = item.Reference
		}
	}
	return result
}
