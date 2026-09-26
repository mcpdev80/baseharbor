package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	workloadPortOverridesFile      = "workload-ports.env"
	workloadFixedPortOverrideFile  = "workload-fixed-ports.override.yaml"
)

var (
	composePublishedPortVariableRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):-([0-9]{1,5})\}\s*:`)
	conflictingHostPortRE          = regexp.MustCompile(`(?i)(?:127\.0\.0\.1:|0\.0\.0\.0:|\[::\]:|\[::1\]:|::1:|host port\s+)([0-9]{1,5})`)
)

type workloadPublishedPortVariable struct {
	Name        string
	DefaultPort int
}

func ensureRepositoryWorkloadPortsForUp(ctx context.Context, in io.Reader, out io.Writer, resolved resolvedApplication, repoRoot string) error {
	if !resolved.FromRepository {
		return nil
	}
	if _, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); err == nil {
		return nil
	} else if !errors.Is(err, application.ErrRuntimeNotApplied) {
		return err
	}

	composePath, found, err := application.ResolveWorkloadCompose(repoRoot, resolved.Manifest)
	if err != nil || !found {
		return err
	}
	variables, err := workloadPublishedPortVariables(application.WorkloadFiles{Compose: composePath})
	if err != nil {
		return fmt.Errorf("inspect configurable workload host ports: %w", err)
	}
	if len(variables) == 0 {
		return nil
	}
	persisted, err := readSimpleEnvFile(repositoryInitEnvPathFromStateRoot(resolved.stateRoot()))
	if errors.Is(err, os.ErrNotExist) {
		persisted = map[string]string{}
	} else if err != nil {
		return err
	}

	for _, variable := range variables {
		if value, explicit := os.LookupEnv(variable.Name); explicit && strings.TrimSpace(value) != "" {
			port, parseErr := parsePort(value)
			if parseErr != nil {
				return fmt.Errorf("invalid explicit workload port %s=%s: %w", variable.Name, value, parseErr)
			}
			if !portAvailable(port) {
				return usageError(
					fmt.Sprintf("explicit workload host port %s=%d is already in use", variable.Name, port),
					"Choose a free explicit port or unset the variable so BaseHarbor can select and persist a fallback.",
				)
			}
			fmt.Fprintf(out, "[OK] workload-port      %s=%d explicit and available\n", variable.Name, port)
			continue
		}

		port := variable.DefaultPort
		if value := strings.TrimSpace(persisted[variable.Name]); value != "" {
			parsed, parseErr := parsePort(value)
			if parseErr != nil {
				return fmt.Errorf("invalid persisted workload port %s=%s: %w", variable.Name, value, parseErr)
			}
			port = parsed
		}

		if portAvailable(port) {
			if strings.TrimSpace(persisted[variable.Name]) == "" {
				if err := updateRepositoryInitValuesAtStateRoot(resolved.stateRoot(), map[string]string{variable.Name: strconv.Itoa(port)}); err != nil {
					return fmt.Errorf("persist workload host port %s=%d: %w", variable.Name, port, err)
				}
				persisted[variable.Name] = strconv.Itoa(port)
			}
			fmt.Fprintf(out, "[OK] workload-port      %s=%d available for this deployment\n", variable.Name, port)
			continue
		}

		fallback := proposedWorkloadPort(port)
		if fallback == 0 {
			return fmt.Errorf("no free fallback port found for %s", variable.Name)
		}
		accepted, err := acceptWorkloadPortFallback(ctx, in, out, variable.Name, port, fallback)
		if err != nil {
			return err
		}
		if !accepted {
			return fmt.Errorf("workload port fallback declined for %s; choose a free host port and retry", variable.Name)
		}
		if err := updateRepositoryInitValuesAtStateRoot(resolved.stateRoot(), map[string]string{variable.Name: strconv.Itoa(fallback)}); err != nil {
			return fmt.Errorf("persist workload host port %s=%d: %w", variable.Name, fallback, err)
		}
		persisted[variable.Name] = strconv.Itoa(fallback)
		fmt.Fprintf(out, "[OK] workload-port      %s=%d saved for this deployment\n", variable.Name, fallback)
	}
	return nil
}

func mergeRepositoryDeploymentWorkloadPorts(environment map[string]string, resolved resolvedApplication) error {
	if !resolved.FromRepository {
		return nil
	}
	repoRoot := resolved.repositoryRoot()
	composePath, found, err := application.ResolveWorkloadCompose(repoRoot, resolved.Manifest)
	if err != nil || !found {
		return err
	}
	variables, err := workloadPublishedPortVariables(application.WorkloadFiles{Compose: composePath})
	if err != nil {
		return err
	}
	values, err := readSimpleEnvFile(repositoryInitEnvPathFromStateRoot(resolved.stateRoot()))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, variable := range variables {
		if _, explicit := os.LookupEnv(variable.Name); explicit {
			continue
		}
		if value := strings.TrimSpace(values[variable.Name]); value != "" {
			environment[variable.Name] = value
		}
	}
	return nil
}

func mergePersistedWorkloadPortOverrides(environment map[string]string, files application.RuntimeFiles) error {
	path := filepath.Join(files.Dir, workloadPortOverridesFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read workload port overrides: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !validWorkloadEnvironmentName(name) {
			return fmt.Errorf("invalid workload port override entry %q", line)
		}
		if _, explicit := os.LookupEnv(name); explicit {
			continue
		}
		if _, err := parsePort(value); err != nil {
			return fmt.Errorf("invalid persisted workload port override %s=%s: %w", name, value, err)
		}
		environment[name] = value
	}
	return nil
}

func persistWorkloadPortOverride(files application.RuntimeFiles, environment map[string]string, name string, port int) error {
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return err
	}
	overrides := map[string]string{}
	path := filepath.Join(files.Dir, workloadPortOverridesFile)
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if ok {
				overrides[key] = value
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	overrides[name] = strconv.Itoa(port)
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("# Generated by BaseHarbor. Repository environment variables set explicitly by the operator take precedence.\n")
	for _, key := range keys {
		fmt.Fprintf(&builder, "%s=%s\n", key, overrides[key])
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	environment[name] = strconv.Itoa(port)
	return nil
}

func workloadPublishedPortVariables(workload application.WorkloadFiles) ([]workloadPublishedPortVariable, error) {
	data, err := os.ReadFile(workload.Compose)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	result := []workloadPublishedPortVariable{}
	for _, match := range composePublishedPortVariableRE.FindAllStringSubmatch(string(data), -1) {
		port, err := strconv.Atoi(match[2])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		result = append(result, workloadPublishedPortVariable{Name: match[1], DefaultPort: port})
	}
	return result, nil
}

func preflightRepositoryWorkloadPublishedPorts(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	workload application.WorkloadFiles,
	files application.RuntimeFiles,
	environment map[string]string,
) error {
	variables, err := workloadPublishedPortVariables(workload)
	if err != nil {
		return fmt.Errorf("inspect configurable workload host ports: %w", err)
	}
	for _, variable := range variables {
		port := currentWorkloadPort(variable, environment)
		if portAvailable(port) {
			continue
		}
		if _, explicit := os.LookupEnv(variable.Name); explicit {
			return &machine.Error{Code: machine.ErrorPortConflict, CauseCode: "host_port_in_use", Message: fmt.Sprintf("Port %d is already in use.", port), Resource: variable.Name, Remediation: "requires developer input", Next: fmt.Sprintf("Choose a free value for %s and retry; BaseHarbor will not replace an explicit operator value.", variable.Name)}
		}
		fallback := proposedWorkloadPort(port)
		if fallback == 0 {
			return &machine.Error{Code: machine.ErrorPortConflict, CauseCode: "host_port_in_use", Message: fmt.Sprintf("Port %d is already in use and no safe fallback was found.", port), Resource: variable.Name, Remediation: "manual action required", Next: "Free the port or choose a free configurable host port and retry."}
		}
		accepted, err := acceptWorkloadPortFallback(ctx, in, out, variable.Name, port, fallback)
		if err != nil {
			return err
		}
		if !accepted {
			return &machine.Error{Code: machine.ErrorPortConflict, CauseCode: "host_port_in_use", Message: fmt.Sprintf("Port %d is already in use.", port), Resource: variable.Name, Remediation: "requires developer input", Next: fmt.Sprintf("Choose a free value for %s and retry.", variable.Name)}
		}
		if err := persistWorkloadPortOverride(files, environment, variable.Name, fallback); err != nil {
			return fmt.Errorf("persist workload host-port preflight selection: %w", err)
		}
		fmt.Fprintf(out, "[OK] workload-port      %s=%d saved for this deployment\n", variable.Name, fallback)
	}

	rel, err := filepath.Rel(workload.RepositoryRoot, workload.Compose)
	if err != nil {
		return err
	}
	analysis, err := repositoryinspect.AnalyzeComposeFile(workload.RepositoryRoot, filepath.ToSlash(rel))
	if err != nil {
		return fmt.Errorf("inspect fixed workload host ports: %w", err)
	}
	selected := map[string]struct{}{}
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	servicePorts := map[string][]string{}
	for _, evidence := range analysis.Ports {
		if _, ok := selected[evidence.Service]; !ok {
			continue
		}
		servicePorts[evidence.Service] = append(servicePorts[evidence.Service], evidence.Value)
	}

	rewritten := map[string][]string{}
	for service, ports := range servicePorts {
		values := append([]string(nil), ports...)
		changed := false
		for i, value := range values {
			port, fixed := fixedComposeHostPort(value)
			if !fixed || portAvailable(port) {
				continue
			}
			fallback := proposedWorkloadPort(port)
			if fallback == 0 {
				return &machine.Error{Code: machine.ErrorPortConflict, CauseCode: "host_port_in_use", Message: fmt.Sprintf("Port %d is already in use and no safe fallback was found.", port), Resource: service, Remediation: "manual action required", Next: "Free the port or choose a free host port and retry."}
			}
			accepted, err := acceptFixedWorkloadPortFallback(ctx, in, out, service, port, fallback)
			if err != nil {
				return err
			}
			if !accepted {
				return &machine.Error{Code: machine.ErrorPortConflict, CauseCode: "host_port_in_use", Message: fmt.Sprintf("Port %d is already in use.", port), Resource: service, Remediation: "requires developer input", Next: fmt.Sprintf("Free port %d or retry and accept BaseHarbor's deployment-local fallback.", port)}
			}
			replacement, ok := rewriteFixedComposeHostPort(value, fallback)
			if !ok {
				return fmt.Errorf("rewrite fixed workload host port for %s: unsupported binding %q", service, value)
			}
			values[i] = replacement
			changed = true
			fmt.Fprintf(out, "[OK] workload-port      %s %d -> %d saved for this deployment\n", service, port, fallback)
		}
		if changed {
			rewritten[service] = values
		}
	}
	if err := persistFixedWorkloadPortOverrides(files, rewritten); err != nil {
		return err
	}
	return nil
}

func fixedWorkloadPortOverridePath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, workloadFixedPortOverrideFile)
}

func existingFixedWorkloadPortOverride(files application.RuntimeFiles) (string, bool, error) {
	path := fixedWorkloadPortOverridePath(files)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("workload fixed-port override %s is not a regular file", path)
	}
	return path, true, nil
}

func rewriteFixedComposeHostPort(value string, published int) (string, bool) {
	value = strings.TrimSpace(strings.Trim(value, ""'"))
	if _, fixed := fixedComposeHostPort(value); !fixed {
		return "", false
	}
	protocol := ""
	if before, after, ok := strings.Cut(value, "/"); ok {
		value = before
		protocol = "/" + after
	}
	last := strings.LastIndex(value, ":")
	if last < 0 {
		return "", false
	}
	prefix, target := value[:last], value[last+1:]
	hostSep := strings.LastIndex(prefix, ":")
	if hostSep < 0 {
		prefix = strconv.Itoa(published)
	} else {
		prefix = prefix[:hostSep+1] + strconv.Itoa(published)
	}
	return prefix + ":" + target + protocol, true
}

func persistFixedWorkloadPortOverrides(files application.RuntimeFiles, ports map[string][]string) error {
	if len(ports) == 0 {
		return nil
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return err
	}
	services := make([]string, 0, len(ports))
	for service := range ports {
		services = append(services, service)
	}
	sort.Strings(services)
	var b strings.Builder
	b.WriteString("# Generated by BaseHarbor. Replaces repository host-port bindings for this deployment only.\n")
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n", service)
		b.WriteString("    ports: !override\n")
		for _, value := range ports[service] {
			fmt.Fprintf(&b, "      - %s\n", strconv.Quote(value))
		}
	}
	path := fixedWorkloadPortOverridePath(files)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write fixed workload port override: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func acceptFixedWorkloadPortFallback(ctx context.Context, in io.Reader, out io.Writer, service string, from, to int) (bool, error) {
	fmt.Fprintf(out, "Workload host port %d for %s is already in use.\n", from, service)
	fmt.Fprintf(out, "Found free host port %d.\n", to)
	if commandAssumesYes() || (!noInput(ctx) && !readerIsTerminal(in)) {
		fmt.Fprintf(out, "[RETRYING] using %d for %s automatically.\n", to, service)
		return true, nil
	}
	if noInput(ctx) {
		return false, usageError("fixed workload port conflict requires an explicit decision in --no-input mode", "Choose a free host port or run with --yes to accept BaseHarbor's deployment-local fallback.")
	}
	fmt.Fprintf(out, "Use %d instead? [Y/n]: ", to)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes" || answer == "j" || answer == "ja", nil
}

func fixedComposeHostPort(value string) (int, bool) {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	if value == "" || strings.Contains(value, "${") {
		return 0, false
	}
	if before, _, ok := strings.Cut(value, "/"); ok {
		value = before
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, false
	}
	host := strings.TrimSpace(parts[len(parts)-2])
	port, err := strconv.Atoi(host)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func conflictingWorkloadHostPort(err error) int {
	if err == nil {
		return 0
	}
	match := conflictingHostPortRE.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	port, _ := strconv.Atoi(match[1])
	return port
}

func currentWorkloadPort(variable workloadPublishedPortVariable, environment map[string]string) int {
	if value, ok := environment[variable.Name]; ok {
		if port, err := strconv.Atoi(value); err == nil {
			return port
		}
	}
	if value, ok := os.LookupEnv(variable.Name); ok {
		if port, err := strconv.Atoi(value); err == nil {
			return port
		}
	}
	return variable.DefaultPort
}

func proposedWorkloadPort(conflict int) int {
	start := conflict + 1
	switch conflict {
	case 80:
		start = 8080
	case 443:
		start = 8443
	}
	return firstAvailablePort(start)
}

func commandAssumesYes() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--yes" || arg == "-y" {
			return true
		}
	}
	return false
}

func acceptWorkloadPortFallback(ctx context.Context, in io.Reader, out io.Writer, variable string, from, to int) (bool, error) {
	fmt.Fprintf(out, "Workload host port %d is already in use.\n", from)
	fmt.Fprintf(out, "Found free host port %d for %s.\n", to, variable)
	if commandAssumesYes() || (!noInput(ctx) && !readerIsTerminal(in)) {
		fmt.Fprintf(out, "[RETRYING] using %s=%d automatically.\n", variable, to)
		return true, nil
	}
	if noInput(ctx) {
		return false, usageError("workload port conflict requires an explicit override in --no-input mode", "Set the published port environment variable explicitly to a free port and retry.")
	}
	fmt.Fprintf(out, "Use %d instead? [Y/n]: ", to)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" || answer == "j" || answer == "ja" {
		return true, nil
	}
	return false, nil
}

func startRepositoryWorkloadWithPortFallback(ctx context.Context, in io.Reader, out io.Writer, compose bhruntime.Compose, workload application.WorkloadFiles, files application.RuntimeFiles, environment map[string]string, startServices, composeFiles []string) error {
	variables, err := workloadPublishedPortVariables(workload)
	if err != nil {
		return fmt.Errorf("inspect application workload published ports: %w", err)
	}
	const maxAttempts = 4
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := compose.UpProjectFilesSelectedNoBuildProgress(ctx, workload.Project, workload.RepositoryRoot, environment, startServices, func(detail string) {
			cli.ReportActivityDetail(out, detail)
		}, composeFiles...)
		if err == nil {
			return nil
		}
		if !bhruntime.IsPortBindingConflict(err) || attempt == maxAttempts {
			return err
		}
		conflict := conflictingWorkloadHostPort(err)
		if conflict == 0 {
			return err
		}
		var candidate *workloadPublishedPortVariable
		for i := range variables {
			if currentWorkloadPort(variables[i], environment) == conflict {
				candidate = &variables[i]
				break
			}
		}
		if candidate == nil {
			return &machine.Error{
				Code:        machine.ErrorPortConflict,
				CauseCode:   "host_port_in_use",
				Message:     fmt.Sprintf("Port %d is already in use.", conflict),
				Remediation: "requires developer input",
				Next:        fmt.Sprintf("Free port %d or make the Compose host binding configurable with a supported ${VAR:-PORT} form.", conflict),
				Cause:       err,
			}
		}
		if _, explicit := os.LookupEnv(candidate.Name); explicit {
			return fmt.Errorf("%w; %s=%d was set explicitly by the operator, choose another free value and retry", err, candidate.Name, conflict)
		}
		fallback := proposedWorkloadPort(conflict)
		if fallback == 0 {
			return fmt.Errorf("%w; no free fallback port found for %s", err, candidate.Name)
		}
		accepted, promptErr := acceptWorkloadPortFallback(ctx, in, out, candidate.Name, conflict, fallback)
		if promptErr != nil {
			return promptErr
		}
		if !accepted {
			return fmt.Errorf("workload port fallback declined for %s; choose a free host port and retry", candidate.Name)
		}
		if downErr := compose.DownProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...); downErr != nil {
			return errors.Join(err, fmt.Errorf("clean up partially started workload before host-port retry: %w", downErr))
		}
		if persistErr := persistWorkloadPortOverride(files, environment, candidate.Name, fallback); persistErr != nil {
			return errors.Join(err, fmt.Errorf("persist workload host-port fallback: %w", persistErr))
		}
	}
	return errors.New("application workload start exhausted host-port retries")
}
