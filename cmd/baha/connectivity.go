package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type connectivityEndpointInput struct {
	Application string
	Environment string
	Service     string
}

func connectCommand() *cli.Command {
	return &cli.Command{
		Name:    "connect",
		Summary: "Allow one explicit application service to reach another",
		Usage:   "baha connect SOURCE TARGET",
		Long:    "Creates one directional deny-by-default exception such as 'baha connect app-a/api app-b/sql'. BaseHarbor resolves environment, runtime service and network details from current runtime state; use app@environment/service only when multiple environments make the short form ambiguous.",
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
			network := application.ConnectivityNetworkName(rule)
			if err := compose.EnsureManagedNetwork(ctx, network); err != nil {
				return fmt.Errorf("create connectivity network: %w", err)
			}
			connected := make([]string, 0, len(sourceContainers)+len(targetContainers))
			rollbackRuntime := func() {
				for _, container := range connected {
					_ = compose.DisconnectManagedNetwork(context.Background(), network, container)
				}
				_ = compose.RemoveManagedNetwork(context.Background(), network)
			}
			for _, container := range sourceContainers {
				if err := compose.ConnectManagedNetwork(ctx, network, container, ""); err != nil {
					rollbackRuntime()
					return fmt.Errorf("attach source container %s: %w", container, err)
				}
				connected = append(connected, container)
			}
			alias := application.ConnectivityTargetAlias(rule)
			for _, container := range targetContainers {
				if err := compose.ConnectManagedNetwork(ctx, network, container, alias); err != nil {
					rollbackRuntime()
					return fmt.Errorf("attach target container %s: %w", container, err)
				}
				connected = append(connected, container)
			}
			rollbackPolicy = false
			fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(source), formatConnectivityEndpoint(target))
			fmt.Fprintf(out, "     target alias:      %s\n", alias)
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
			network := application.ConnectivityNetworkName(rule)
			for _, container := range containersForResolvedEndpoint(rule.Source, containers) {
				if err := compose.DisconnectManagedNetwork(ctx, network, container); err != nil {
					return fmt.Errorf("detach source container %s: %w", container, err)
				}
			}
			for _, container := range containersForResolvedEndpoint(rule.Target, containers) {
				if err := compose.DisconnectManagedNetwork(ctx, network, container); err != nil {
					return fmt.Errorf("detach target container %s: %w", container, err)
				}
			}
			if err := application.RemoveConnectivityRule(rule); err != nil {
				return err
			}
			if err := compose.RemoveManagedNetwork(ctx, network); err != nil {
				return fmt.Errorf("remove connectivity network: %w", err)
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
		network := application.ConnectivityNetworkName(rule)
		if err := compose.EnsureManagedNetwork(ctx, network); err != nil {
			return err
		}
		for _, container := range sourceContainers {
			if err := compose.ConnectManagedNetwork(ctx, network, container, ""); err != nil {
				return fmt.Errorf("attach connectivity source %s: %w", container, err)
			}
		}
		alias := application.ConnectivityTargetAlias(rule)
		for _, container := range targetContainers {
			if err := compose.ConnectManagedNetwork(ctx, network, container, alias); err != nil {
				return fmt.Errorf("attach connectivity target %s: %w", container, err)
			}
		}
		fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
	}
	return nil
}

func ensureConnectivityNetworksForManifest(ctx context.Context, compose bhruntime.Compose, m application.Manifest) error {
	rules, err := application.LoadConnectivityRules()
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if !connectivityEndpointMatchesManifest(rule.Source, m) && !connectivityEndpointMatchesManifest(rule.Target, m) {
			continue
		}
		name := application.ConnectivityNetworkName(rule)
		if _, ok := seen[name]; ok {
			continue
		}
		if err := compose.EnsureManagedNetwork(ctx, name); err != nil {
			return err
		}
		seen[name] = struct{}{}
	}
	return nil
}

func connectivityEndpointMatchesManifest(endpoint application.ConnectivityEndpoint, m application.Manifest) bool {
	return endpoint.Application == m.Name && endpoint.Environment == m.Environment
}

func parseConnectivityEndpointInput(raw string) (connectivityEndpointInput, error) {
	raw = strings.TrimSpace(raw)
	left, service, ok := strings.Cut(raw, "/")
	if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(service) == "" || strings.Contains(service, "/") {
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
	return connectivityEndpointInput{Application: app, Environment: environment, Service: canonicalConnectivityService(service)}, nil
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
		return application.ConnectivityRule{}, errors.New("connectivity rule is ambiguous; qualify application environments")
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
	return endpoint.Application + "@" + endpoint.Environment + "/" + endpoint.Service
}
