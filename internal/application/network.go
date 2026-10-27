package application

import "strings"

// ApplicationBackendNetworkName returns the legacy stable network name.
func ApplicationBackendNetworkName(m Manifest) string {
	return RuntimeProjectName(m) + "_default"
}

func ApplicationBackendNetworkNameForProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	return project + "_default"
}

// ApplicationExposureNetworkName returns the legacy exposure network name.
func ApplicationExposureNetworkName(m Manifest) string {
	return "baseharbor-exposure-" + m.Name + "-" + m.Environment + "_default"
}

func ApplicationExposureNetworkNameForProject(project string) string {
	project = strings.TrimSpace(strings.TrimPrefix(project, "baseharbor-"))
	if project == "" {
		return ""
	}
	return "baseharbor-exposure-" + project + "_default"
}

func DevelopmentWorkloadNetworkNameForProject(project string) string {
	project = strings.TrimSpace(strings.TrimPrefix(project, "baseharbor-"))
	if project == "" {
		return ""
	}
	return "baseharbor-dev-workload-" + project
}

func DevelopmentWorkloadAlias(m Manifest) string {
	name := strings.ToLower(strings.TrimSpace(m.Name))
	if name == "" {
		name = "app"
	}
	return "bh-dev-" + name + "-api"
}
