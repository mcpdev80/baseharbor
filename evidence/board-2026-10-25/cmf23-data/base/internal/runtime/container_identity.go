package runtime

// ContainerMatchesProjectResource preserves the existing native naming
// conventions: Compose has ordinal -1; first-party Quadlet uses project-service.
// Both require the actual project and service labels, not a name prefix.
func ContainerMatchesProjectResource(kind ProviderKind, container RuntimeContainer, project string, resource ProjectResource) bool {
	if resource.Kind != "container" || container.Project != project || container.Service == "" || resource.Name != project+"-"+container.Service+"-1" {
		return false
	}
	return container.Name == resource.Name || kind == ProviderPodman && container.Name == project+"-"+container.Service
}
