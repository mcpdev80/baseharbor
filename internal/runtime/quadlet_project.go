package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type QuadletProject struct {
	Project      string
	Files        map[string]string
	ServiceUnits map[string]string
	Containers   map[string]string
}

type quadletComposeProject struct {
	Services map[string]quadletComposeService  `yaml:"services"`
	Volumes  map[string]quadletComposeResource `yaml:"volumes"`
	Networks map[string]quadletComposeResource `yaml:"networks"`
}

type quadletComposeService struct {
	Image       string                    `yaml:"image"`
	Build       composeBuild              `yaml:"build"`
	Ports       []string                  `yaml:"ports"`
	Environment composeEnv                `yaml:"environment"`
	Restart     string                    `yaml:"restart"`
	Command     quadletStringList         `yaml:"command"`
	Volumes     []string                  `yaml:"volumes"`
	Networks    quadletNetworkAttachments `yaml:"networks"`
	DependsOn   quadletStringSet          `yaml:"depends_on"`
	Profiles    []string                  `yaml:"profiles"`
	User        string                    `yaml:"user"`
	ReadOnly    bool                      `yaml:"read_only"`
	CapDrop     []string                  `yaml:"cap_drop"`
	SecurityOpt []string                  `yaml:"security_opt"`
	Tmpfs       []string                  `yaml:"tmpfs"`
	Healthcheck quadletComposeHealthcheck `yaml:"healthcheck"`
	Logging     quadletComposeLogging     `yaml:"logging"`
}

type quadletComposeResource struct {
	Name     string `yaml:"name"`
	External bool   `yaml:"external"`
	Internal bool   `yaml:"internal"`
}

type quadletComposeHealthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval"`
	Timeout     string   `yaml:"timeout"`
	Retries     int      `yaml:"retries"`
	StartPeriod string   `yaml:"start_period"`
	Disable     bool     `yaml:"disable"`
}

type quadletComposeLogging struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options"`
}

type quadletStringList []string

func (s *quadletStringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		if strings.TrimSpace(node.Value) != "" {
			*s = []string{node.Value}
		}
		return nil
	case yaml.SequenceNode:
		var values []string
		for _, item := range node.Content {
			values = append(values, item.Value)
		}
		*s = values
		return nil
	default:
		return errors.New("unsupported Compose string/list syntax")
	}
}

type quadletStringSet []string

func (s *quadletStringSet) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.SequenceNode:
		var values []string
		for _, item := range node.Content {
			values = append(values, item.Value)
		}
		*s = values
		return nil
	case yaml.MappingNode:
		var values []string
		for i := 0; i+1 < len(node.Content); i += 2 {
			values = append(values, node.Content[i].Value)
		}
		*s = values
		return nil
	default:
		return errors.New("unsupported Compose sequence/mapping syntax")
	}
}

type quadletNetworkAttachments struct {
	Names   []string
	Aliases map[string][]string
}

func (n *quadletNetworkAttachments) UnmarshalYAML(node *yaml.Node) error {
	n.Aliases = map[string][]string{}
	switch node.Kind {
	case 0:
		return nil
	case yaml.SequenceNode:
		for _, item := range node.Content {
			name := strings.TrimSpace(item.Value)
			if name != "" {
				n.Names = append(n.Names, name)
			}
		}
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			name := strings.TrimSpace(node.Content[i].Value)
			if name == "" {
				continue
			}
			n.Names = append(n.Names, name)
			value := node.Content[i+1]
			if value.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(value.Content); j += 2 {
				if value.Content[j].Value != "aliases" {
					continue
				}
				aliasesNode := value.Content[j+1]
				if aliasesNode.Kind != yaml.SequenceNode {
					return errors.New("Compose network aliases must be a sequence")
				}
				for _, alias := range aliasesNode.Content {
					if value := strings.TrimSpace(alias.Value); value != "" {
						n.Aliases[name] = append(n.Aliases[name], value)
					}
				}
			}
		}
		return nil
	default:
		return errors.New("unsupported Compose networks syntax")
	}
}

func RenderComposeProjectQuadlets(composePath, envFile, project string, selectedServices ...string) (QuadletProject, error) {
	return RenderComposeProjectFilesQuadlets([]string{composePath}, envFile, project, selectedServices...)
}

func RenderComposeProjectFilesQuadlets(composePaths []string, envFile, project string, selectedServices ...string) (QuadletProject, error) {
	return RenderComposeProjectFilesQuadletsEnv(composePaths, envFile, nil, project, selectedServices...)
}

func RenderComposeProjectFilesQuadletsEnv(composePaths []string, envFile string, environment map[string]string, project string, selectedServices ...string) (QuadletProject, error) {
	project = sanitizeQuadletName(project)
	if project == "" {
		return QuadletProject{}, errors.New("Quadlet project name is empty")
	}
	if len(composePaths) == 0 {
		return QuadletProject{}, errors.New("at least one Compose file is required")
	}

	env, err := quadletComposeEnvironment(envFile)
	if err != nil {
		return QuadletProject{}, err
	}
	for key, value := range environment {
		if strings.TrimSpace(key) == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(value, 0) {
			return QuadletProject{}, errors.New("invalid Compose process environment")
		}
		env[key] = value
	}

	var document yaml.Node
	for index, composePath := range composePaths {
		data, err := os.ReadFile(composePath)
		if err != nil {
			return QuadletProject{}, err
		}
		var current yaml.Node
		if err := yaml.Unmarshal(data, &current); err != nil {
			return QuadletProject{}, fmt.Errorf("decode Compose YAML %s: %w", composePath, err)
		}
		expandQuadletComposeNode(&current, env)
		if index == 0 {
			document = current
			continue
		}
		if err := mergeQuadletComposeDocuments(&document, &current); err != nil {
			return QuadletProject{}, fmt.Errorf("merge Compose YAML %s: %w", composePath, err)
		}
	}

	var model quadletComposeProject
	if err := document.Decode(&model); err != nil {
		return QuadletProject{}, fmt.Errorf("decode rendered Compose model: %w", err)
	}
	composePath := composePaths[0]
	if len(model.Services) == 0 {
		return QuadletProject{Project: project, Files: map[string]string{}, ServiceUnits: map[string]string{}, Containers: map[string]string{}}, nil
	}

	selected := map[string]struct{}{}
	if len(selectedServices) > 0 {
		for _, service := range selectedServices {
			service = strings.TrimSpace(service)
			if service != "" {
				selected[service] = struct{}{}
			}
		}
		for service := range selected {
			if _, ok := model.Services[service]; !ok {
				return QuadletProject{}, fmt.Errorf("Compose service %q not found", service)
			}
		}
	}

	result := QuadletProject{
		Project:      project,
		Files:        map[string]string{},
		ServiceUnits: map[string]string{},
		Containers:   map[string]string{},
	}

	defaultNetworkNeeded := false
	for name, service := range model.Services {
		if len(selected) > 0 {
			if _, ok := selected[name]; !ok {
				continue
			}
		}
		if len(service.Networks.Names) == 0 {
			defaultNetworkNeeded = true
		}
	}
	if defaultNetworkNeeded {
		if _, ok := model.Networks["default"]; !ok {
			model.Networks["default"] = quadletComposeResource{}
		}
	}

	for name, volume := range model.Volumes {
		if volume.External {
			continue
		}
		unit := project + "-" + sanitizeQuadletName(name)
		actual := strings.TrimSpace(volume.Name)
		if actual == "" {
			actual = project + "_" + name
		}
		result.Files[unit+".volume"] = "[Volume]\nVolumeName=" + actual + "\n"
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
		if network.Internal {
			b.WriteString("Internal=true\n")
		}
		result.Files[unit+".network"] = b.String()
	}

	names := make([]string, 0, len(model.Services))
	for name := range model.Services {
		if len(selected) > 0 {
			if _, ok := selected[name]; !ok {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, serviceName := range names {
		service := model.Services[serviceName]
		unitBase := project + "-" + sanitizeQuadletName(serviceName)
		containerName := unitBase
		image := strings.TrimSpace(service.Image)

		if strings.TrimSpace(service.Build.Context) != "" {
			contextDir := service.Build.Context
			if !filepath.IsAbs(contextDir) {
				contextDir = filepath.Join(filepath.Dir(composePath), contextDir)
			}
			contextDir, err = filepath.Abs(contextDir)
			if err != nil {
				return QuadletProject{}, err
			}
			dockerfile := strings.TrimSpace(service.Build.Dockerfile)
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}
			result.Files[unitBase+".build"] = fmt.Sprintf(
				"[Unit]\nDescription=BaseHarbor Quadlet build for %s/%s\n\n[Build]\nImageTag=localhost/%s:quadlet\nSetWorkingDirectory=%s\nFile=%s\n\n[Service]\nTimeoutStartSec=900\n",
				project, serviceName, unitBase, systemdEscapeValue(contextDir), systemdEscapeValue(dockerfile),
			)
			image = unitBase + ".build"
		}
		if image == "" {
			return QuadletProject{}, fmt.Errorf("Compose service %q has neither image nor build", serviceName)
		}

		envName := ""
		if len(service.Environment) > 0 {
			envName = unitBase + ".env"
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
		}

		var unit strings.Builder
		unit.WriteString("[Unit]\n")
		fmt.Fprintf(&unit, "Description=BaseHarbor Quadlet service %s/%s\n", project, serviceName)
		deps := append([]string(nil), service.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			if len(selected) > 0 {
				if _, ok := selected[dep]; !ok {
					continue
				}
			}
			depUnit := project + "-" + sanitizeQuadletName(dep) + ".container"
			fmt.Fprintf(&unit, "Requires=%s\nAfter=%s\n", depUnit, depUnit)
		}

		unit.WriteString("\n[Container]\n")
		fmt.Fprintf(&unit, "Image=%s\nContainerName=%s\n", image, containerName)
		fmt.Fprintf(&unit, "Label=com.docker.compose.project=%s\n", project)
		fmt.Fprintf(&unit, "Label=com.docker.compose.service=%s\n", serviceName)
		fmt.Fprintf(&unit, "Label=io.podman.compose.project=%s\n", project)
		fmt.Fprintf(&unit, "Label=io.podman.compose.service=%s\n", serviceName)
		if envName != "" {
			fmt.Fprintf(&unit, "EnvironmentFile=./%s\n", envName)
		}
		if user := strings.TrimSpace(service.User); user != "" {
			userPart, groupPart, found := strings.Cut(user, ":")
			fmt.Fprintf(&unit, "User=%s\n", userPart)
			if found && strings.TrimSpace(groupPart) != "" {
				fmt.Fprintf(&unit, "Group=%s\n", groupPart)
			}
		}
		if service.ReadOnly {
			unit.WriteString("ReadOnly=true\n")
		}
		for _, capability := range service.CapDrop {
			if strings.TrimSpace(capability) != "" {
				fmt.Fprintf(&unit, "DropCapability=%s\n", strings.ToLower(strings.TrimSpace(capability)))
			}
		}
		for _, option := range service.SecurityOpt {
			if strings.EqualFold(strings.TrimSpace(option), "no-new-privileges:true") || strings.EqualFold(strings.TrimSpace(option), "no-new-privileges") {
				unit.WriteString("NoNewPrivileges=true\n")
			}
		}
		for _, tmpfs := range service.Tmpfs {
			if strings.TrimSpace(tmpfs) != "" {
				fmt.Fprintf(&unit, "Tmpfs=%s\n", strings.TrimSpace(tmpfs))
			}
		}
		for _, port := range service.Ports {
			if strings.TrimSpace(port) != "" {
				fmt.Fprintf(&unit, "PublishPort=%s\n", strings.TrimSpace(port))
			}
		}

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
				fmt.Fprintf(&unit, "Network=%s\n", actual)
			case declared:
				fmt.Fprintf(&unit, "Network=%s-%s.network\n", project, sanitizeQuadletName(networkName))
			default:
				return QuadletProject{}, fmt.Errorf("Compose service %q references undeclared network %q", serviceName, networkName)
			}
			fmt.Fprintf(&unit, "NetworkAlias=%s\n", serviceName)
			for _, alias := range service.Networks.Aliases[networkName] {
				fmt.Fprintf(&unit, "NetworkAlias=%s\n", alias)
			}
		}

		for _, mount := range service.Volumes {
			quadletMount, err := renderQuadletVolumeMount(composePath, project, model.Volumes, mount)
			if err != nil {
				return QuadletProject{}, fmt.Errorf("Compose service %q volume %q: %w", serviceName, mount, err)
			}
			fmt.Fprintf(&unit, "Volume=%s\n", quadletMount)
		}

		if len(service.Command) > 0 {
			fmt.Fprintf(&unit, "Exec=%s\n", quadletSystemdJoin(service.Command))
		}
		if service.Healthcheck.Disable {
			unit.WriteString("HealthCmd=none\n")
		} else if len(service.Healthcheck.Test) > 0 {
			health, err := renderQuadletHealthCommand(service.Healthcheck.Test)
			if err != nil {
				return QuadletProject{}, fmt.Errorf("Compose service %q healthcheck: %w", serviceName, err)
			}
			fmt.Fprintf(&unit, "HealthCmd=%s\n", health)
			if strings.TrimSpace(service.Healthcheck.Interval) != "" {
				fmt.Fprintf(&unit, "HealthInterval=%s\n", service.Healthcheck.Interval)
			}
			if strings.TrimSpace(service.Healthcheck.Timeout) != "" {
				fmt.Fprintf(&unit, "HealthTimeout=%s\n", service.Healthcheck.Timeout)
			}
			if service.Healthcheck.Retries > 0 {
				fmt.Fprintf(&unit, "HealthRetries=%d\n", service.Healthcheck.Retries)
			}
			if strings.TrimSpace(service.Healthcheck.StartPeriod) != "" {
				fmt.Fprintf(&unit, "HealthStartPeriod=%s\n", service.Healthcheck.StartPeriod)
			}
			unit.WriteString("Notify=healthy\n")
		}
		if driver := strings.TrimSpace(service.Logging.Driver); driver != "" {
			fmt.Fprintf(&unit, "LogDriver=%s\n", driver)
			keys := make([]string, 0, len(service.Logging.Options))
			for key := range service.Logging.Options {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(&unit, "LogOpt=%s=%s\n", key, service.Logging.Options[key])
			}
		}

		unit.WriteString("\n[Service]\nTimeoutStartSec=900\n")
		switch strings.ToLower(strings.TrimSpace(service.Restart)) {
		case "always", "unless-stopped":
			unit.WriteString("Restart=always\n")
		case "on-failure":
			unit.WriteString("Restart=on-failure\n")
		case "", "no":
			unit.WriteString("Restart=no\n")
		default:
			return QuadletProject{}, fmt.Errorf("Compose restart policy %q is unsupported", service.Restart)
		}

		result.Files[unitBase+".container"] = unit.String()
		result.ServiceUnits[serviceName] = unitBase + ".service"
		result.Containers[serviceName] = containerName
	}

	return result, nil
}

func WriteQuadletProject(dir string, project QuadletProject) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	names := make([]string, 0, len(project.Files))
	for name := range project.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		mode := os.FileMode(0o600)
		if strings.HasSuffix(name, ".network") || strings.HasSuffix(name, ".volume") || strings.HasSuffix(name, ".container") || strings.HasSuffix(name, ".build") {
			mode = 0o644
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(project.Files[name]), mode); err != nil {
			return err
		}
	}
	return nil
}

func quadletComposeEnvironment(envFile string) (map[string]string, error) {
	result := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[key] = value
		}
	}
	if strings.TrimSpace(envFile) == "" {
		return result, nil
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid environment line in %s", envFile)
		}
		result[strings.TrimSpace(key)] = value
	}
	return result, nil
}

func expandQuadletComposeNode(node *yaml.Node, env map[string]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		node.Value = expandQuadletComposeString(node.Value, env)
	}
	for _, child := range node.Content {
		expandQuadletComposeNode(child, env)
	}
}

func expandQuadletComposeString(value string, env map[string]string) string {
	const dollarSentinel = "__BASEHARBOR_DOLLAR__"
	value = strings.ReplaceAll(value, "$$", dollarSentinel)
	for {
		start := strings.Index(value, "${")
		if start < 0 {
			break
		}
		endRel := strings.Index(value[start+2:], "}")
		if endRel < 0 {
			break
		}
		end := start + 2 + endRel
		expr := value[start+2 : end]
		key := expr
		fallback := ""
		hasFallback := false
		if before, after, ok := strings.Cut(expr, ":-"); ok {
			key, fallback, hasFallback = before, after, true
		}
		replacement := env[key]
		if replacement == "" && hasFallback {
			replacement = fallback
		}
		value = value[:start] + replacement + value[end+1:]
	}
	return strings.ReplaceAll(value, dollarSentinel, "$")
}

func renderQuadletVolumeMount(composePath, project string, volumes map[string]quadletComposeResource, mount string) (string, error) {
	parts := strings.Split(mount, ":")
	if len(parts) < 1 || len(parts) > 3 {
		return "", errors.New("unsupported volume syntax")
	}
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), nil
	}
	source := strings.TrimSpace(parts[0])
	target := strings.TrimSpace(parts[1])
	if source == "" || target == "" {
		return "", errors.New("empty volume source or target")
	}
	options := ""
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		options = ":" + strings.TrimSpace(parts[2])
	}
	if volume, ok := volumes[source]; ok {
		if volume.External {
			actual := strings.TrimSpace(volume.Name)
			if actual == "" {
				actual = source
			}
			return actual + ":" + target + options, nil
		}
		return project + "-" + sanitizeQuadletName(source) + ".volume:" + target + options, nil
	}
	if strings.HasPrefix(source, ".") {
		absolute, err := filepath.Abs(filepath.Join(filepath.Dir(composePath), source))
		if err != nil {
			return "", err
		}
		source = absolute
	}
	return source + ":" + target + options, nil
}

func renderQuadletHealthCommand(test []string) (string, error) {
	if len(test) == 0 {
		return "", nil
	}
	mode := strings.ToUpper(strings.TrimSpace(test[0]))
	switch mode {
	case "NONE":
		return "none", nil
	case "CMD":
		return quadletSystemdJoin(test[1:]), nil
	case "CMD-SHELL":
		if len(test) != 2 {
			return "", errors.New("CMD-SHELL requires exactly one command")
		}
		return quadletSystemdJoin([]string{"/bin/sh", "-c", test[1]}), nil
	default:
		return quadletSystemdJoin(test), nil
	}
}

func quadletSystemdJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ReplaceAll(value, "%", "%%")
		if value == "" || strings.ContainsAny(value, " \t\n\"'\\") {
			quoted = append(quoted, strconv.Quote(value))
		} else {
			quoted = append(quoted, value)
		}
	}
	return strings.Join(quoted, " ")
}

func mergeQuadletComposeDocuments(base, override *yaml.Node) error {
	baseMap, err := quadletDocumentMap(base)
	if err != nil {
		return err
	}
	overrideMap, err := quadletDocumentMap(override)
	if err != nil {
		return err
	}
	mergeQuadletYAMLMap(baseMap, overrideMap)
	return nil
}

func quadletDocumentMap(document *yaml.Node) (*yaml.Node, error) {
	if document == nil || document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, errors.New("Compose document must contain one root mapping")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("Compose document root must be a mapping")
	}
	return root, nil
}

func mergeQuadletYAMLMap(base, override *yaml.Node) {
	for i := 0; i+1 < len(override.Content); i += 2 {
		key := override.Content[i]
		value := override.Content[i+1]
		found := -1
		for j := 0; j+1 < len(base.Content); j += 2 {
			if base.Content[j].Value == key.Value {
				found = j
				break
			}
		}
		if found < 0 {
			base.Content = append(base.Content, cloneQuadletYAMLNode(key), cloneQuadletYAMLNode(value))
			continue
		}
		baseValue := base.Content[found+1]
		if baseValue.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode {
			mergeQuadletYAMLMap(baseValue, value)
			continue
		}
		base.Content[found+1] = cloneQuadletYAMLNode(value)
	}
}

func cloneQuadletYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	copy := *node
	copy.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		copy.Content[i] = cloneQuadletYAMLNode(child)
	}
	return &copy
}

func RenderComposeProjectFilesJSON(composePaths []string, environment map[string]string) (string, error) {
	if len(composePaths) == 0 {
		return "", errors.New("at least one Compose file is required")
	}
	env, err := quadletComposeEnvironment("")
	if err != nil {
		return "", err
	}
	for key, value := range environment {
		if strings.TrimSpace(key) == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(value, 0) {
			return "", errors.New("invalid Compose process environment")
		}
		env[key] = value
	}
	var document yaml.Node
	for index, composePath := range composePaths {
		data, err := os.ReadFile(composePath)
		if err != nil {
			return "", err
		}
		var current yaml.Node
		if err := yaml.Unmarshal(data, &current); err != nil {
			return "", fmt.Errorf("decode Compose YAML %s: %w", composePath, err)
		}
		expandQuadletComposeNode(&current, env)
		if index == 0 {
			document = current
			continue
		}
		if err := mergeQuadletComposeDocuments(&document, &current); err != nil {
			return "", fmt.Errorf("merge Compose YAML %s: %w", composePath, err)
		}
	}
	var model any
	if err := document.Decode(&model); err != nil {
		return "", err
	}
	data, err := json.Marshal(model)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
