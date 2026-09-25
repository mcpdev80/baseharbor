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
