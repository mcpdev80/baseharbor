package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type connectivityEndpointInput struct {
	Application string
	Environment string
	Service     string
	Port        int
}

func connectCommand() *cli.Command {
	return &cli.Command{
		Name:    "connect",
		Summary: "Allow one explicit application service to reach another",
		Usage:   "baha connect SOURCE TARGET",
		Long:    "Creates one directional deny-by-default exception such as 'baha connect app-a/api app-b/sql'. BaseHarbor resolves environment, runtime network and target port from current runtime state. Qualify app@environment/service or service:port only when runtime state is ambiguous.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 2 {
				return usageError("baha connect requires SOURCE and TARGET", "Example: baha connect app-a/api app-b/sql")
			}
			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			containers, err := compose.ListComposeContainers(ctx)
			if err != nil {
				return err
			}
			sourceInput, err := parseConnectivityEndpointInput(args[0])
			if err != nil {
				return err
			}
			if sourceInput.Port != 0 {
				return usageError("SOURCE must not include a port", "BaseHarbor needs only the source service identity.")
			}
			targetInput, err := parseConnectivityEndpointInput(args[1])
			if err != nil {
				return err
			}
			source, sourceContainers, err := resolveConnectivityEndpoint(sourceInput, containers)
			if err != nil {
				return fmt.Errorf("resolve source %q: %w", args[0], err)
			}
			target, targetContainers, err := resolveConnectivityEndpoint(targetInput, containers)
			if err != nil {
				return fmt.Errorf("resolve target %q: %w", args[1], err)
			}
			target.Port, err = resolveConnectivityTargetPort(ctx, compose, targetInput, target, targetContainers)
			if err != nil {
				return fmt.Errorf("resolve target %q: %w", args[1], err)
			}
			targetNetwork, err := resolveConnectivityTargetNetwork(ctx, compose, target, containers)
			if err != nil {
				return fmt.Errorf("resolve target network for %q: %w", args[1], err)
			}
			rule := application.ConnectivityRule{Source: source, Target: target}
			if err := rule.Validate(); err != nil {
				return err
			}
			if err := application.AddConnectivityRule(rule); err != nil {
				return err
			}
			rollbackPolicy := true
			defer func() {
				if rollbackPolicy {
					_ = application.RemoveConnectivityRule(rule)
				}
			}()
			if err := convergeConnectivityRule(ctx, compose, rule, sourceContainers, targetNetwork); err != nil {
				_ = suspendConnectivityRule(context.Background(), compose, rule, containers)
				return err
			}
			rollbackPolicy = false
			fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
			fmt.Fprintf(out, "     target alias:      %s\n", application.ConnectivityTargetAlias(rule))
			return nil
		},
	}
}

func disconnectCommand() *cli.Command {
	return &cli.Command{
		Name:    "disconnect",
		Summary: "Remove one explicit cross-application connectivity exception",
		Usage:   "baha disconnect SOURCE TARGET",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 2 {
				return usageError("baha disconnect requires SOURCE and TARGET", "Example: baha disconnect app-a/api app-b/sql")
			}
			source, err := parseConnectivityEndpointInput(args[0])
			if err != nil {
				return err
			}
			target, err := parseConnectivityEndpointInput(args[1])
			if err != nil {
				return err
			}
			rule, err := findConnectivityRule(source, target)
			if err != nil {
				return err
			}
			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			containers, err := compose.ListComposeContainers(ctx)
			if err != nil {
				return err
			}
			if err := suspendConnectivityRule(ctx, compose, rule, containers); err != nil {
				return err
			}
			if err := application.RemoveConnectivityRule(rule); err != nil {
				return err
			}
			if err := connectivityrelay.RemoveFiles(application.ConnectivityRuleID(rule)); err != nil {
				return err
			}
			fmt.Fprintf(out, "[OK] connectivity       removed %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
			return nil
		},
	}
}

func connectionsCommand() *cli.Command {
	return &cli.Command{
		Name:    "connections",
		Summary: "List explicit cross-application connectivity policy",
		Usage:   "baha connections",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha connections does not accept arguments", "Run 'baha connections --help' for usage.")
			}
			rules, err := application.LoadConnectivityRules()
			if err != nil {
				return err
			}
			if len(rules) == 0 {
				fmt.Fprintln(out, "No cross-application connectivity is allowed.")
				return nil
			}
			for _, rule := range rules {
				fmt.Fprintf(out, "%s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
			}
			return nil
		},
	}
}

func reconcileConnectivityForManifest(ctx context.Context, out io.Writer, compose bhruntime.Compose, m application.Manifest) error {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	containers, err := compose.ListComposeContainers(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !connectivityEndpointMatchesManifest(rule.Source, m) && !connectivityEndpointMatchesManifest(rule.Target, m) {
			continue
		}
		sourceContainers := containersForResolvedEndpoint(rule.Source, containers)
		targetContainers := containersForResolvedEndpoint(rule.Target, containers)
		if len(sourceContainers) == 0 || len(targetContainers) == 0 {
			continue
		}
		targetNetwork, err := resolveConnectivityTargetNetwork(ctx, compose, rule.Target, containers)
		if err != nil {
			return err
		}
		if err := convergeConnectivityRule(ctx, compose, rule, sourceContainers, targetNetwork); err != nil {
			return err
		}
		fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
	}
	return nil
}

func suspendConnectivityForManifest(ctx context.Context, compose bhruntime.Compose, m application.Manifest) error {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	containers, err := compose.ListComposeContainers(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !connectivityEndpointMatchesManifest(rule.Source, m) && !connectivityEndpointMatchesManifest(rule.Target, m) {
			continue
		}
		if err := suspendConnectivityRule(ctx, compose, rule, containers); err != nil {
			return err
		}
	}
	return nil
}

func convergeConnectivityRule(ctx context.Context, compose bhruntime.Compose, rule application.ConnectivityRule, sourceContainers []string, targetNetwork string) error {
	network := application.ConnectivityNetworkName(rule)
	if err := compose.EnsureManagedNetwork(ctx, network); err != nil {
		return fmt.Errorf("create connectivity network: %w", err)
	}
	connected := make([]string, 0, len(sourceContainers))
	for _, container := range sourceContainers {
		if err := compose.ConnectManagedNetwork(ctx, network, container, ""); err != nil {
			for _, attached := range connected {
				_ = compose.DisconnectManagedNetwork(context.Background(), network, attached)
			}
			return fmt.Errorf("attach connectivity source %s: %w", container, err)
		}
		connected = append(connected, container)
	}

	files, err := connectivityrelay.EnsureFiles(connectivityrelay.RuntimeSpec{
		ID:            application.ConnectivityRuleID(rule),
		SourceNetwork: network,
		SourceAlias:   application.ConnectivityTargetAlias(rule),
		TargetNetwork: targetNetwork,
		TargetHost:    rule.Target.Service,
		TargetPort:    rule.Target.Port,
	})
	if err != nil {
		return err
	}
	if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start directed connectivity relay: %w", err)
	}
	if err := waitConnectivityRelayReady(ctx, compose, files.Project); err != nil {
		return err
	}
	return nil
}

func waitConnectivityRelayReady(ctx context.Context, compose bhruntime.Compose, project string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var lastStatus string
	for waitCtx.Err() == nil {
		containers, err := compose.ListComposeContainers(waitCtx)
		if err != nil {
			return err
		}
		var relay string
		for _, container := range containers {
			if container.Project == project && container.Service == "relay" {
				if relay != "" && relay != container.Name {
					return fmt.Errorf("connectivity relay project %s has multiple relay containers", project)
				}
				relay = container.Name
			}
		}
		if relay != "" {
			status, err := compose.ContainerHealthStatus(waitCtx, relay)
			if err != nil {
				return err
			}
			lastStatus = status
			if status == "healthy" {
				return nil
			}
		}
		select {
		case <-waitCtx.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("directed connectivity relay did not become ready: last status %q: %w", lastStatus, waitCtx.Err())
}

func suspendConnectivityRule(ctx context.Context, compose bhruntime.Compose, rule application.ConnectivityRule, containers []bhruntime.ComposeContainer) error {
	id := application.ConnectivityRuleID(rule)
	files, err := connectivityrelay.ExistingFiles(id)
	if err == nil {
		if err := compose.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop directed connectivity relay: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	network := application.ConnectivityNetworkName(rule)
	for _, container := range containersForResolvedEndpoint(rule.Source, containers) {
		if err := compose.DisconnectManagedNetwork(ctx, network, container); err != nil {
			return fmt.Errorf("detach connectivity source %s: %w", container, err)
		}
	}
	if err := compose.RemoveManagedNetwork(ctx, network); err != nil {
		return fmt.Errorf("remove connectivity network: %w", err)
	}
	return nil
}

func connectivityEndpointMatchesManifest(endpoint application.ConnectivityEndpoint, m application.Manifest) bool {
	return endpoint.Application == m.Name && endpoint.Environment == m.Environment
}

func parseConnectivityEndpointInput(raw string) (connectivityEndpointInput, error) {
	raw = strings.TrimSpace(raw)
	left, servicePart, ok := strings.Cut(raw, "/")
	if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(servicePart) == "" || strings.Contains(servicePart, "/") {
		return connectivityEndpointInput{}, fmt.Errorf("connectivity endpoint %q must use application/service", raw)
	}
	app := strings.TrimSpace(left)
	environment := ""
	if value, env, found := strings.Cut(app, "@"); found {
		app = strings.TrimSpace(value)
		environment = strings.TrimSpace(env)
		if app == "" || environment == "" {
			return connectivityEndpointInput{}, fmt.Errorf("connectivity endpoint %q has invalid application@environment", raw)
		}
	}
	service := strings.TrimSpace(servicePart)
	port := 0
	if name, rawPort, found := strings.Cut(service, ":"); found {
		service = strings.TrimSpace(name)
		value, err := strconv.Atoi(strings.TrimSpace(rawPort))
		if err != nil || value < 1 || value > 65535 {
			return connectivityEndpointInput{}, fmt.Errorf("connectivity endpoint %q has invalid port", raw)
		}
		port = value
	}
	if service == "" {
		return connectivityEndpointInput{}, fmt.Errorf("connectivity endpoint %q has no service", raw)
	}
	return connectivityEndpointInput{Application: app, Environment: environment, Service: canonicalConnectivityService(service), Port: port}, nil
}

func canonicalConnectivityService(service string) string {
	service = strings.TrimSpace(service)
	switch strings.ToLower(service) {
	case "sql":
		return "postgres"
	case "cache", "redis":
		return "valkey"
	default:
		return service
	}
}

func resolveConnectivityEndpoint(input connectivityEndpointInput, containers []bhruntime.ComposeContainer) (application.ConnectivityEndpoint, []string, error) {
	type candidate struct {
		endpoint   application.ConnectivityEndpoint
		containers []string
	}
	byKey := map[string]*candidate{}
	for _, container := range containers {
		environment, ok := connectivityProjectEnvironment(container.Project, input.Application)
		if !ok || (input.Environment != "" && input.Environment != environment) {
			continue
		}
		if !connectivityServiceMatches(input.Service, container.Service) {
			continue
		}
		endpoint := application.ConnectivityEndpoint{Application: input.Application, Environment: environment, Service: container.Service}
		key := endpoint.Environment + "\x00" + endpoint.Service
		item := byKey[key]
		if item == nil {
			item = &candidate{endpoint: endpoint}
			byKey[key] = item
		}
		item.containers = append(item.containers, container.Name)
	}
	if len(byKey) == 0 {
		return application.ConnectivityEndpoint{}, nil, errors.New("no matching BaseHarbor-managed Compose service found")
	}
	if len(byKey) > 1 {
		var choices []string
		for _, item := range byKey {
			choices = append(choices, formatConnectivityEndpoint(item.endpoint))
		}
		sort.Strings(choices)
		return application.ConnectivityEndpoint{}, nil, fmt.Errorf("endpoint is ambiguous (%s); qualify the environment or exact service", strings.Join(choices, ", "))
	}
	for _, item := range byKey {
		sort.Strings(item.containers)
		return item.endpoint, item.containers, nil
	}
	panic("unreachable")
}

func resolveConnectivityTargetPort(ctx context.Context, compose bhruntime.Compose, input connectivityEndpointInput, endpoint application.ConnectivityEndpoint, containers []string) (int, error) {
	if input.Port != 0 {
		return input.Port, nil
	}
	switch {
	case endpoint.Service == "postgres" || strings.HasPrefix(endpoint.Service, "postgres-"):
		return 5432, nil
	case endpoint.Service == "valkey" || strings.HasPrefix(endpoint.Service, "valkey-"):
		return 6379, nil
	}
	ports := map[int]struct{}{}
	for _, container := range containers {
		values, err := compose.ContainerExposedTCPPorts(ctx, container)
		if err != nil {
			return 0, err
		}
		for _, port := range values {
			ports[port] = struct{}{}
		}
	}
	if len(ports) != 1 {
		return 0, errors.New("target port is not unambiguous; use application/service:PORT")
	}
	for port := range ports {
		return port, nil
	}
	panic("unreachable")
}

func resolveConnectivityTargetNetwork(ctx context.Context, compose bhruntime.Compose, endpoint application.ConnectivityEndpoint, containers []bhruntime.ComposeContainer) (string, error) {
	var matched []bhruntime.ComposeContainer
	for _, container := range containers {
		environment, ok := connectivityProjectEnvironment(container.Project, endpoint.Application)
		if ok && environment == endpoint.Environment && container.Service == endpoint.Service {
			matched = append(matched, container)
		}
	}
	if len(matched) == 0 {
		return "", errors.New("target service is not running")
	}
	common := map[string]int{}
	owned := map[string]struct{}{}
	for _, container := range matched {
		networks, err := compose.ContainerNetworks(ctx, container.Name)
		if err != nil {
			return "", err
		}
		for _, network := range networks {
			common[network]++
			if network == container.Project+"_default" {
				return network, nil
			}
			owner, err := compose.NetworkProjectOwner(ctx, network)
			if err == nil && owner == container.Project {
				owned[network] = struct{}{}
			}
		}
	}
	if len(owned) > 0 {
		names := make([]string, 0, len(owned))
		for name := range owned {
			names = append(names, name)
		}
		sort.Strings(names)
		return names[0], nil
	}
	var shared []string
	for name, count := range common {
		if count == len(matched) {
			shared = append(shared, name)
		}
	}
	sort.Strings(shared)
	if len(shared) == 1 {
		return shared[0], nil
	}
	return "", errors.New("target network is not unambiguous")
}

func connectivityServiceMatches(requested, actual string) bool {
	if requested == actual {
		return true
	}
	if requested == "postgres" && strings.HasPrefix(actual, "postgres-") {
		return true
	}
	if requested == "valkey" && strings.HasPrefix(actual, "valkey-") {
		return true
	}
	return false
}

func connectivityProjectEnvironment(project, applicationName string) (string, bool) {
	for _, prefix := range []string{
		"baseharbor-workload-" + applicationName + "-",
		"baseharbor-" + applicationName + "-",
	} {
		if strings.HasPrefix(project, prefix) {
			environment := strings.TrimPrefix(project, prefix)
			if environment != "" {
				return environment, true
			}
		}
	}
	return "", false
}

func findConnectivityRule(source, target connectivityEndpointInput) (application.ConnectivityRule, error) {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return application.ConnectivityRule{}, err
	}
	var matches []application.ConnectivityRule
	for _, rule := range rules {
		if connectivityInputMatchesEndpoint(source, rule.Source) && connectivityInputMatchesEndpoint(target, rule.Target) {
			matches = append(matches, rule)
		}
	}
	if len(matches) == 0 {
		return application.ConnectivityRule{}, errors.New("connectivity rule not found")
	}
	if len(matches) > 1 {
		return application.ConnectivityRule{}, errors.New("connectivity rule is ambiguous; qualify application environment or target port")
	}
	return matches[0], nil
}

func connectivityInputMatchesEndpoint(input connectivityEndpointInput, endpoint application.ConnectivityEndpoint) bool {
	if input.Application != endpoint.Application {
		return false
	}
	if input.Environment != "" && input.Environment != endpoint.Environment {
		return false
	}
	if input.Port != 0 && input.Port != endpoint.Port {
		return false
	}
	return connectivityServiceMatches(input.Service, endpoint.Service)
}

func containersForResolvedEndpoint(endpoint application.ConnectivityEndpoint, containers []bhruntime.ComposeContainer) []string {
	var result []string
	for _, container := range containers {
		environment, ok := connectivityProjectEnvironment(container.Project, endpoint.Application)
		if !ok || environment != endpoint.Environment || container.Service != endpoint.Service {
			continue
		}
		result = append(result, container.Name)
	}
	sort.Strings(result)
	return result
}

func formatConnectivityEndpoint(endpoint application.ConnectivityEndpoint) string {
	service := endpoint.Service
	switch service {
	case "postgres":
		service = "sql"
	case "valkey":
		service = "cache"
	}
	value := endpoint.Application + "@" + endpoint.Environment + "/" + service
	if endpoint.Port != 0 && !((service == "sql" && endpoint.Port == 5432) || (service == "cache" && endpoint.Port == 6379)) {
		value += ":" + strconv.Itoa(endpoint.Port)
	}
	return value
}
