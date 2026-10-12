package workload

// Model is the provider-neutral workload representation produced from a
// repository-owned workload source. It intentionally contains workload
// semantics only; Docker, Podman, Kubernetes and OpenShift realization details
// do not belong here.
type Model struct {
	Services []Service
}

// Service describes one application workload service. Args preserve the image CMD override semantics; provider-specific entrypoint fields are deliberately absent.
type Service struct {
	Name        string
	Image       string
	Build       *Build
	Args        []string
	Environment map[string]string
	Ports       []Port
}

// Build records source-to-artifact input. Runtime providers consume an
// already-resolved image artifact and must not turn build configuration into
// provider-specific application contract fields.
type Build struct {
	Context    string
	Dockerfile string
}

// Port is the portable container endpoint. Host publishing is deployment
// realization and therefore deliberately absent.
type Port struct {
	Container int
	Protocol  string
}
