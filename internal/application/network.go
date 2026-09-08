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
