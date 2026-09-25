package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func emptyQuadletProject(project string) QuadletProject {
	return QuadletProject{
		Project:      project,
		Files:        map[string]string{},
		ServiceUnits: map[string]string{},
		Containers:   map[string]string{},
	}
}

func quadletLoadComposeModel(composePaths []string, envFile string, environment map[string]string) (quadletComposeProject, string, error) {
	env, err := quadletComposeEnvironment(envFile)
	if err != nil {
		return quadletComposeProject{}, "", err
	}
	for key, value := range environment {
		if strings.TrimSpace(key) == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(value, 0) {
			return quadletComposeProject{}, "", errors.New("invalid Compose process environment")
		}
		env[key] = value
	}

	var document yaml.Node
	for index, composePath := range composePaths {
		data, err := os.ReadFile(composePath)
		if err != nil {
			return quadletComposeProject{}, "", err
		}
		var current yaml.Node
		if err := yaml.Unmarshal(data, &current); err != nil {
			return quadletComposeProject{}, "", fmt.Errorf("decode Compose YAML %s: %w", composePath, err)
		}
		expandQuadletComposeNode(&current, env)
		if index == 0 {
			document = current
			continue
		}
		if err := mergeQuadletComposeDocuments(&document, &current); err != nil {
			return quadletComposeProject{}, "", fmt.Errorf("merge Compose YAML %s: %w", composePath, err)
		}
	}

	var model quadletComposeProject
	if err := document.Decode(&model); err != nil {
		return quadletComposeProject{}, "", fmt.Errorf("decode rendered Compose model: %w", err)
	}
	return model, composePaths[0], nil
}

func quadletSelectComposeServices(model quadletComposeProject, selectedServices []string) (map[string]struct{}, error) {
	selected := map[string]struct{}{}
	for _, service := range selectedServices {
		service = strings.TrimSpace(service)
		if service != "" {
			selected[service] = struct{}{}
		}
	}
	for service := range selected {
		if _, ok := model.Services[service]; !ok {
			return nil, fmt.Errorf("Compose service %q not found", service)
		}
	}
	return selected, nil
}

func quadletEnsureDefaultNetwork(model *quadletComposeProject, selected map[string]struct{}) {
	for name, service := range model.Services {
		if !quadletComposeServiceEnabled(name, service, selected) {
			continue
		}
		if len(service.Networks.Names) != 0 {
			continue
		}
		if model.Networks == nil {
			model.Networks = map[string]quadletComposeResource{}
		}
		if _, ok := model.Networks["default"]; !ok {
			model.Networks["default"] = quadletComposeResource{}
		}
		return
	}
}

func quadletRenderProjectResources(result *QuadletProject, model quadletComposeProject, project string) {
	for name, volume := range model.Volumes {
		if volume.External {
			continue
		}
		unit := project + "-" + sanitizeQuadletName(name)
		actual := strings.TrimSpace(volume.Name)
		if actual == "" {
			actual = project + "_" + name
		}
		result.Files[unit+".volume"] = "[Volume]\nVolumeName=" + actual + "\nLabel=com.docker.compose.project=" + project + "\nLabel=io.podman.compose.project=" + project + "\n"
	}

	for name, network := range model.Networks {
		if network.External {
			continue
		}
		unit := project + "-" + sanitizeQuadletName(name)
		actual := strings.TrimSpace(network.Name)
		if actual == "" {
			actual = project + "_" + name
		}
		var b strings.Builder
		b.WriteString("[Network]\nNetworkName=" + actual + "\n")
		b.WriteString("Label=com.docker.compose.project=" + project + "\n")
		b.WriteString("Label=io.podman.compose.project=" + project + "\n")
		if network.Internal {
			b.WriteString("Internal=true\n")
		}
		result.Files[unit+".network"] = b.String()
	}
}

func quadletEnabledServiceNames(model quadletComposeProject, selected map[string]struct{}) []string {
	names := make([]string, 0, len(model.Services))
	for name, service := range model.Services {
		if quadletComposeServiceEnabled(name, service, selected) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func quadletRenderProjectService(result *QuadletProject, composePath, project string, model quadletComposeProject, serviceName string, selected map[string]struct{}) error {
	service := model.Services[serviceName]
	unitBase := project + "-" + sanitizeQuadletName(serviceName)
	containerName := unitBase

	image, err := quadletRenderServiceImage(result, composePath, project, serviceName, unitBase, service)
	if err != nil {
		return err
	}
	envName := quadletRenderServiceEnvironment(result, unitBase, service)

	var unit strings.Builder
	quadletRenderServiceUnitHeader(&unit, project, serviceName, image, containerName, envName, service, selected)
	quadletRenderServiceSecurity(&unit, service)
	if err := quadletRenderServiceNetworks(&unit, project, serviceName, model, service); err != nil {
		return err
	}
	if err := quadletRenderServiceVolumes(&unit, composePath, project, serviceName, model, service); err != nil {
		return err
	}
	if err := quadletRenderServiceSecrets(&unit, composePath, serviceName, model, service); err != nil {
		return err
	}
	if err := quadletRenderServiceProcess(&unit, serviceName, service); err != nil {
		return err
	}
	if err := quadletRenderServiceHealth(&unit, serviceName, service); err != nil {
		return err
	}
	quadletRenderServiceLogging(&unit, service)
	if err := quadletRenderServiceRestart(&unit, service); err != nil {
		return err
	}

	result.Files[unitBase+".container"] = unit.String()
	result.ServiceUnits[serviceName] = unitBase + ".service"
	result.Containers[serviceName] = containerName
	return nil
}

func quadletRenderServiceImage(result *QuadletProject, composePath, project, serviceName, unitBase string, service quadletComposeService) (string, error) {
	image := strings.TrimSpace(service.Image)
	if strings.TrimSpace(service.Build.Context) == "" {
		if image == "" {
			return "", fmt.Errorf("Compose service %q has neither image nor build", serviceName)
		}
		return image, nil
	}

	contextDir := service.Build.Context
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(filepath.Dir(composePath), contextDir)
	}
	contextDir, err := filepath.Abs(contextDir)
	if err != nil {
		return "", err
	}
	dockerfile := strings.TrimSpace(service.Build.Dockerfile)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	result.Files[unitBase+".build"] = fmt.Sprintf(
		"[Unit]\nDescription=BaseHarbor Quadlet build for %s/%s\n\n[Build]\nImageTag=localhost/%s:quadlet\nSetWorkingDirectory=%s\nFile=%s\n\n[Service]\nTimeoutStartSec=900\n",
		project, serviceName, unitBase, systemdEscapeValue(contextDir), systemdEscapeValue(dockerfile),
	)
	return unitBase + ".build", nil
}

func quadletRenderServiceEnvironment(result *QuadletProject, unitBase string, service quadletComposeService) string {
	if len(service.Environment) == 0 {
		return ""
	}
	envName := unitBase + ".env"
	keys := make([]string, 0, len(service.Environment))
	for key := range service.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var envOut strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&envOut, "%s=%s\n", key, service.Environment[key])
	}
	result.Files[envName] = envOut.String()
	return envName
}

func quadletRenderServiceUnitHeader(unit *strings.Builder, project, serviceName, image, containerName, envName string, service quadletComposeService, selected map[string]struct{}) {
	unit.WriteString("[Unit]\n")
	fmt.Fprintf(unit, "Description=BaseHarbor Quadlet service %s/%s\n", project, serviceName)
	deps := append([]string(nil), service.DependsOn...)
	sort.Strings(deps)
	for _, dep := range deps {
		if len(selected) > 0 {
			if _, ok := selected[dep]; !ok {
				continue
			}
		}
		depUnit := project + "-" + sanitizeQuadletName(dep) + ".service"
		fmt.Fprintf(unit, "Requires=%s\nAfter=%s\n", depUnit, depUnit)
	}

	unit.WriteString("\n[Container]\n")
	fmt.Fprintf(unit, "Image=%s\nContainerName=%s\n", image, containerName)
	fmt.Fprintf(unit, "Label=com.docker.compose.project=%s\n", project)
	fmt.Fprintf(unit, "Label=com.docker.compose.service=%s\n", serviceName)
	fmt.Fprintf(unit, "Label=io.podman.compose.project=%s\n", project)
	fmt.Fprintf(unit, "Label=io.podman.compose.service=%s\n", serviceName)
	if envName != "" {
		fmt.Fprintf(unit, "EnvironmentFile=./%s\n", envName)
	}
}

func quadletRenderServiceSecurity(unit *strings.Builder, service quadletComposeService) {
	if user := strings.TrimSpace(service.User); user != "" {
		userPart, groupPart, found := strings.Cut(user, ":")
		fmt.Fprintf(unit, "User=%s\n", userPart)
		if found && strings.TrimSpace(groupPart) != "" {
			fmt.Fprintf(unit, "Group=%s\n", groupPart)
		}
	}
	if service.ReadOnly {
		unit.WriteString("ReadOnly=true\n")
	}
	for _, capability := range service.CapDrop {
		if strings.TrimSpace(capability) != "" {
			fmt.Fprintf(unit, "DropCapability=%s\n", strings.ToLower(strings.TrimSpace(capability)))
		}
	}
	for _, capability := range service.CapAdd {
		if strings.TrimSpace(capability) != "" {
			fmt.Fprintf(unit, "AddCapability=%s\n", strings.TrimSpace(capability))
		}
	}
	for _, option := range service.SecurityOpt {
		if strings.EqualFold(strings.TrimSpace(option), "no-new-privileges:true") || strings.EqualFold(strings.TrimSpace(option), "no-new-privileges") {
			unit.WriteString("NoNewPrivileges=true\n")
		}
	}
	for _, tmpfs := range service.Tmpfs {
		if strings.TrimSpace(tmpfs) != "" {
			fmt.Fprintf(unit, "Tmpfs=%s\n", strings.TrimSpace(tmpfs))
		}
	}
	for _, port := range service.Ports {
		if strings.TrimSpace(port) != "" {
			fmt.Fprintf(unit, "PublishPort=%s\n", strings.TrimSpace(port))
		}
	}
}

func quadletRenderServiceNetworks(unit *strings.Builder, project, serviceName string, model quadletComposeProject, service quadletComposeService) error {
	serviceNetworks := append([]string(nil), service.Networks.Names...)
	if len(serviceNetworks) == 0 {
		serviceNetworks = []string{"default"}
	}
	sort.Strings(serviceNetworks)
	for _, networkName := range serviceNetworks {
		network, declared := model.Networks[networkName]
		switch {
		case declared && network.External:
			actual := strings.TrimSpace(network.Name)
			if actual == "" {
				actual = networkName
			}
			fmt.Fprintf(unit, "Network=%s\n", actual)
		case declared:
			fmt.Fprintf(unit, "Network=%s-%s.network\n", project, sanitizeQuadletName(networkName))
		default:
			return fmt.Errorf("Compose service %q references undeclared network %q", serviceName, networkName)
		}
		fmt.Fprintf(unit, "NetworkAlias=%s\n", serviceName)
		for _, alias := range service.Networks.Aliases[networkName] {
			fmt.Fprintf(unit, "NetworkAlias=%s\n", alias)
		}
	}
	return nil
}

func quadletRenderServiceVolumes(unit *strings.Builder, composePath, project, serviceName string, model quadletComposeProject, service quadletComposeService) error {
	for _, mount := range service.Volumes {
		quadletMount, err := renderQuadletVolumeMount(composePath, project, model.Volumes, mount)
		if err != nil {
			return fmt.Errorf("Compose service %q volume %q: %w", serviceName, mount, err)
		}
		fmt.Fprintf(unit, "Volume=%s\n", quadletMount)
	}
	return nil
}

func quadletRenderServiceSecrets(unit *strings.Builder, composePath, serviceName string, model quadletComposeProject, service quadletComposeService) error {
	for _, secretRef := range service.Secrets {
		secretName := strings.TrimSpace(secretRef.Source)
		secretTarget := strings.TrimSpace(secretRef.Target)
		if secretName == "" {
			return fmt.Errorf("Compose service %q references an empty secret source", serviceName)
		}
		if secretTarget == "" {
			secretTarget = secretName
		}
		secret, ok := model.Secrets[secretName]
		if !ok {
			return fmt.Errorf("Compose service %q references undeclared secret %q", serviceName, secretName)
		}
		if secret.External {
			return fmt.Errorf("Compose service %q uses external secret %q which is unsupported by the Quadlet runtime", serviceName, secretName)
		}
		source := strings.TrimSpace(secret.File)
		if source == "" {
			return fmt.Errorf("Compose secret %q has no file source", secretName)
		}
		if !filepath.IsAbs(source) {
			source = filepath.Join(filepath.Dir(composePath), source)
		}
		source, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		fmt.Fprintf(unit, "Volume=%s:/run/secrets/%s:ro\n", source, secretTarget)
	}
	return nil
}

func quadletRenderServiceProcess(unit *strings.Builder, serviceName string, service quadletComposeService) error {
	if len(service.Entrypoint) > 0 {
		entrypoint, err := json.Marshal([]string(service.Entrypoint))
		if err != nil {
			return fmt.Errorf("encode Compose service %q entrypoint: %w", serviceName, err)
		}
		fmt.Fprintf(unit, "Entrypoint=%s\n", entrypoint)
	}
	if len(service.Command) > 0 {
		fmt.Fprintf(unit, "Exec=%s\n", quadletSystemdJoin(service.Command))
	}
	return nil
}

func quadletRenderServiceHealth(unit *strings.Builder, serviceName string, service quadletComposeService) error {
	if service.Healthcheck.Disable {
		unit.WriteString("HealthCmd=none\n")
		return nil
	}
	if len(service.Healthcheck.Test) == 0 {
		return nil
	}
	health, err := renderQuadletHealthCommand(service.Healthcheck.Test)
	if err != nil {
		return fmt.Errorf("Compose service %q healthcheck: %w", serviceName, err)
	}
	fmt.Fprintf(unit, "HealthCmd=%s\n", health)
	if strings.TrimSpace(service.Healthcheck.Interval) != "" {
		fmt.Fprintf(unit, "HealthInterval=%s\n", service.Healthcheck.Interval)
	}
	if strings.TrimSpace(service.Healthcheck.Timeout) != "" {
		fmt.Fprintf(unit, "HealthTimeout=%s\n", service.Healthcheck.Timeout)
	}
	if service.Healthcheck.Retries > 0 {
		fmt.Fprintf(unit, "HealthRetries=%d\n", service.Healthcheck.Retries)
	}
	if strings.TrimSpace(service.Healthcheck.StartPeriod) != "" {
		fmt.Fprintf(unit, "HealthStartPeriod=%s\n", service.Healthcheck.StartPeriod)
	}
	return nil
}

func quadletRenderServiceLogging(unit *strings.Builder, service quadletComposeService) {
	driver := strings.TrimSpace(service.Logging.Driver)
	if driver == "" {
		return
	}
	fmt.Fprintf(unit, "LogDriver=%s\n", driver)
	keys := make([]string, 0, len(service.Logging.Options))
	for key := range service.Logging.Options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(unit, "LogOpt=%s=%s\n", key, service.Logging.Options[key])
	}
}

func quadletRenderServiceRestart(unit *strings.Builder, service quadletComposeService) error {
	unit.WriteString("\n[Service]\nTimeoutStartSec=900\n")
	switch strings.ToLower(strings.TrimSpace(service.Restart)) {
	case "always", "unless-stopped":
		unit.WriteString("Restart=always\n")
	case "on-failure":
		unit.WriteString("Restart=on-failure\n")
	case "", "no":
		unit.WriteString("Restart=no\n")
	default:
		return fmt.Errorf("Compose restart policy %q is unsupported", service.Restart)
	}
	return nil
}
