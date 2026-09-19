package application

// ApplicationBackendNetworkName returns the stable Docker/Podman network name
// used by BaseHarbor-managed backend services for one application environment.
//
// Application-owned workloads may attach to this network as an additional
// external network without replacing their own Compose networks. The current
// Compose provider maps the logical backend network to the deterministic
// project default network.
func ApplicationBackendNetworkName(m Manifest) string {
	return RuntimeProjectName(m) + "_default"
}

// ApplicationExposureNetworkName returns the stable Compose network used to
// connect explicitly exposed workload endpoints to the selected exposure
// provider. It is separate from the managed backend network because endpoint
// exposure must also work for workload-only applications.
func ApplicationExposureNetworkName(m Manifest) string {
	return "baseharbor-exposure-" + m.Name + "-" + m.Environment + "_default"
}
