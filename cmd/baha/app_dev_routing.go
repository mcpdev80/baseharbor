package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func applicationCanonicalRouteHosts(target string, m application.Manifest) ([]string, error) {
	if !requiresDevelopmentGateway(m) {
		return nil, nil
	}
	routes, err := devgateway.Routes(target)
	if err != nil {
		return nil, err
	}
	appOwner := "app/" + m.Name + "/" + m.Environment
	seen := map[string]struct{}{}
	var hosts []string
	for _, route := range routes {
		include := route.Owner == appOwner || strings.HasPrefix(route.Owner, appOwner+"/")
		if m.Services.ObjectStorageManagementUI && route.Owner == "shared/object-storage" {
			include = true
		}
		if m.Services.SecretsManagementUI && route.Owner == "shared/openbao" {
			include = true
		}
		if m.Services.ObservabilityManagementUI && route.Owner == "shared/prometheus" {
			include = true
		}
		if m.Services.Identity && route.Owner == "shared/keycloak" {
			include = true
		}
		if m.Services.SQLManagementUI && route.Owner == "shared/postgresql" {
			include = true
		}
		if (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI) && route.Owner == "shared/valkey" {
			include = true
		}
		if !include {
			continue
		}
		if _, ok := seen[route.Host]; ok {
			continue
		}
		seen[route.Host] = struct{}{}
		hosts = append(hosts, route.Host)
	}
	sort.Strings(hosts)
	return hosts, nil
}

func developmentWorkloadRoute(appOwner, host, upstreamHost, network string, files application.RuntimeFiles, service string, port int, protocol string) devgateway.Route {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "http"
	}
	route := devgateway.Route{
		Key:      appOwner + "/workload-api",
		Host:     host,
		Upstream: fmt.Sprintf("%s://%s:%d", protocol, upstreamHost, port),
		Network:  network,
	}
	if protocol == "https" {
		route.TrustFile = filepath.Join(files.Bindings, "runtime-identity", "ca.pem")
		route.ServerName = service
	}
	return route
}

func requiresDevelopmentGateway(m application.Manifest) bool {
	if !devaccess.Enabled(m.Environment) {
		return false
	}
	return len(m.Exposures) > 0 ||
		application.HasExplicitWorkload(m) ||
		m.Services.Identity ||
		m.Services.SQLManagementUI ||
		m.Services.CacheManagementUI ||
		m.Services.KeyValueManagementUI ||
		m.Services.MessagingManagementUI ||
		m.Services.DocumentDatabaseManagementUI ||
		m.Services.ObjectStorageManagementUI ||
		m.Services.SecretsManagementUI ||
		m.Services.IdentityManagementUI ||
		m.Services.ObservabilityManagementUI
}

func requiresDevelopmentManagementAccess(m application.Manifest) bool {
	if !devaccess.Enabled(m.Environment) {
		return false
	}
	return m.Services.Identity ||
		m.Services.SQLManagementUI ||
		m.Services.CacheManagementUI ||
		m.Services.KeyValueManagementUI ||
		m.Services.MessagingManagementUI ||
		m.Services.DocumentDatabaseManagementUI ||
		m.Services.ObjectStorageManagementUI ||
		m.Services.SecretsManagementUI ||
		m.Services.IdentityManagementUI ||
		m.Services.ObservabilityManagementUI
}

type developmentRoutePlan struct {
	target    string
	appOwner  string
	appRoutes []devgateway.Route
	groups    []devgateway.OwnerRoutes
}

func (e *applicationApplyExecution) reconcileDevelopmentCanonicalRoutes(ctx context.Context) error {
	if !requiresDevelopmentGateway(e.manifest) {
		return nil
	}
	target := e.resolved.Target.Name
	if _, err := devaccess.EnsureDomain(target); err != nil {
		return fmt.Errorf("prepare development domain: %w", err)
	}
	plan := &developmentRoutePlan{
		target:    target,
		appOwner:  "app/" + e.manifest.Name + "/" + e.manifest.Environment,
		appRoutes: make([]devgateway.Route, 0, 8),
	}
	for _, step := range []func(context.Context, *developmentRoutePlan) error{
		e.addDevelopmentBackendRoutes,
		e.addDevelopmentIdentityRoutes,
		e.addDevelopmentWorkloadRoutes,
		e.addDevelopmentExposureRoutes,
		e.addDevelopmentManagementRoutes,
	} {
		if err := step(ctx, plan); err != nil {
			return err
		}
	}
	plan.groups = append(plan.groups, devgateway.OwnerRoutes{Owner: plan.appOwner, Routes: plan.appRoutes})
	if err := devgateway.ReplaceRoutes(ctx, e.compose, e.issuer, target, plan.groups...); err != nil {
		return fmt.Errorf("reconcile canonical development routes: %w", err)
	}
	if err := devgateway.Verify(ctx, target); err != nil {
		return fmt.Errorf("verify canonical development routes: %w", err)
	}
	routes, err := devgateway.Routes(target)
	if err != nil {
		return err
	}
	e.term.Section("Development URLs")
	for _, route := range routes {
		if route.Owner == plan.appOwner || strings.HasPrefix(route.Owner, "shared/") {
			e.term.Result("READY", route.Key, devgateway.URLForTarget(target, route.Host))
		}
	}
	return nil
}

func (e *applicationApplyExecution) addDevelopmentBackendRoutes(_ context.Context, plan *developmentRoutePlan) error {
	if e.manifest.Services.SQLManagementUI {
		placement, err := application.ResolveProviderPlacement(e.manifest, capability.ProviderPostgreSQL)
		if err != nil {
			return err
		}
		if placement.Scope == capability.ScopeShared {
			host, err := devaccess.SharedHost(plan.target, "pgadmin")
			if err != nil {
				return err
			}
			shared := application.SharedBackendFilesAt(e.resolved.TargetStateRoot, plan.target, e.manifest.Environment)
			plan.groups = append(plan.groups, devgateway.OwnerRoutes{
				Owner: "shared/postgresql",
				Routes: []devgateway.Route{{
					Key: "shared/postgresql", Host: host,
					Upstream:   "https://shared-pgadmin:8443",
					Network:    shared.Network,
					TrustFile:  filepath.Join(shared.Dir, "management-ui", "postgres", "pki", "ca.pem"),
					ServerName: "localhost",
				}},
			})
		} else {
			host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, "pgadmin")
			if err != nil {
				return err
			}
			plan.appRoutes = append(plan.appRoutes, devgateway.Route{
				Key: plan.appOwner + "/pgadmin", Host: host,
				Upstream:   "https://" + devaccess.ApplicationAlias(e.manifest.Name, "pgadmin") + ":8443",
				Network:    application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
				TrustFile:  filepath.Join(e.files.Dir, "providers", "management-ui", "postgres", "pki", "ca.pem"),
				ServerName: "localhost",
			})
		}
	}

	if e.manifest.Services.CacheManagementUI || e.manifest.Services.KeyValueManagementUI {
		placement, err := application.ResolveProviderPlacement(e.manifest, capability.ProviderValkey)
		if err != nil {
			return err
		}
		if placement.Scope == capability.ScopeShared {
			host, err := devaccess.SharedHost(plan.target, "cache")
			if err != nil {
				return err
			}
			shared := application.SharedBackendFilesAt(e.resolved.TargetStateRoot, plan.target, e.manifest.Environment)
			plan.groups = append(plan.groups, devgateway.OwnerRoutes{
				Owner: "shared/valkey",
				Routes: []devgateway.Route{{
					Key: "shared/valkey", Host: host,
					Upstream:   "https://shared-cache-ui-access:8443",
					Network:    shared.Network,
					TrustFile:  filepath.Join(shared.Dir, "management-ui", "cache", "pki", "ca.pem"),
					ServerName: "localhost",
				}},
			})
		} else {
			host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, "cache")
			if err != nil {
				return err
			}
			plan.appRoutes = append(plan.appRoutes, devgateway.Route{
				Key: plan.appOwner + "/cache", Host: host,
				Upstream:   "https://" + devaccess.ApplicationAlias(e.manifest.Name, "cache") + ":8443",
				Network:    application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
				TrustFile:  filepath.Join(e.files.Dir, "providers", "management-ui", "cache", "pki", "ca.pem"),
				ServerName: "localhost",
			})
		}
	}

	if e.manifest.Services.MessagingManagementUI {
		for _, instance := range application.RabbitMQInstanceNames(e.manifest) {
			routeName := application.RabbitMQManagementUIRouteName(instance)
			host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, routeName)
			if err != nil {
				return err
			}
			plan.appRoutes = append(plan.appRoutes, devgateway.Route{
				Key: plan.appOwner + "/" + routeName, Host: host,
				Upstream:   "https://" + devaccess.ApplicationAlias(e.manifest.Name, routeName) + ":8443",
				Network:    application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
				TrustFile:  filepath.Join(e.files.Dir, "providers", "management-ui", "rabbitmq", instance, "pki", "ca.pem"),
				ServerName: "localhost",
			})
		}
	}
	if e.manifest.Services.DocumentDatabaseManagementUI {
		for _, instance := range application.DocumentDatabaseInstanceNames(e.manifest) {
			routeName := application.MongoDBManagementUIRouteName(instance)
			host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, routeName)
			if err != nil {
				return err
			}
			plan.appRoutes = append(plan.appRoutes, devgateway.Route{
				Key: plan.appOwner + "/" + routeName, Host: host,
				Upstream:   "https://" + devaccess.ApplicationAlias(e.manifest.Name, routeName) + ":8443",
				Network:    application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
				TrustFile:  filepath.Join(e.files.Dir, "providers", "management-ui", "mongodb", instance, "pki", "ca.pem"),
				ServerName: "localhost",
			})
		}
	}
	return nil
}

func (e *applicationApplyExecution) addDevelopmentIdentityRoutes(_ context.Context, plan *developmentRoutePlan) error {
	if !e.manifest.Services.Identity {
		return nil
	}
	placement, err := application.ResolveProviderPlacement(e.manifest, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := identityprovider.ExistingKeycloakFilesAt(e.manifest, e.resolved.TargetStateRoot, plan.target)
	if err != nil {
		return err
	}
	owner := plan.appOwner
	loginKey := plan.appOwner + "/identity"
	adminKey := plan.appOwner + "/identity-admin"
	host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, "identity")
	if placement.Scope == capability.ScopeShared {
		owner = "shared/keycloak"
		loginKey = owner + "/login"
		adminKey = owner + "/admin"
		host, err = devaccess.SharedHost(plan.target, "identity")
	}
	if err != nil {
		return err
	}
	identityRoutes := []devgateway.Route{{
		Key: loginKey, Host: host,
		Upstream:   fmt.Sprintf("https://%s:%d", devaccess.ProviderAlias(files.Project, "identity"), 8443),
		Network:    files.ConsumerNetwork,
		TrustFile:  files.PublicAccess.Material.CA,
		ServerName: files.PublicAccess.Material.ServerName,
	}}
	if e.manifest.Services.IdentityManagementUI {
		var adminHost string
		if placement.Scope == capability.ScopeShared {
			adminHost, err = devaccess.SharedHost(plan.target, "identity-admin")
		} else {
			adminHost, err = devaccess.ApplicationHost(plan.target, e.manifest.Name, "identity-admin")
		}
		if err != nil {
			return err
		}
		identityRoutes = append(identityRoutes, devgateway.Route{
			Key: adminKey, Host: adminHost,
			Upstream:   "https://" + devaccess.ProviderAlias(files.Project, "identity-admin") + ":8443",
			Network:    files.InternalNetwork,
			TrustFile:  files.AdminAccess.Material.CA,
			ServerName: files.AdminAccess.Material.ServerName,
		})
	}
	if placement.Scope == capability.ScopeShared {
		plan.groups = append(plan.groups, devgateway.OwnerRoutes{Owner: owner, Routes: identityRoutes})
	} else {
		plan.appRoutes = append(plan.appRoutes, identityRoutes...)
	}
	return nil
}

func (e *applicationApplyExecution) addDevelopmentWorkloadRoutes(_ context.Context, plan *developmentRoutePlan) error {
	if len(e.manifest.Exposures) != 0 || !application.HasExplicitWorkload(e.manifest) {
		return nil
	}
	selected, composePath, found, err := application.SelectedWorkloadServices(e.resolved.repositoryRoot(), e.manifest)
	if err != nil || !found || len(selected) != 1 {
		return err
	}
	relative := composePath
	if rel, relErr := filepath.Rel(e.resolved.repositoryRoot(), composePath); relErr == nil {
		relative = rel
	}
	analysis, err := repositoryinspect.AnalyzeComposeFile(e.resolved.repositoryRoot(), relative)
	if err != nil {
		return err
	}
	var ports []int
	for _, item := range analysis.Ports {
		if item.Service == selected[0] {
			if port, ok := composeTargetPort(item.Value); ok {
				ports = append(ports, port)
			}
		}
	}
	if len(ports) != 1 {
		return nil
	}
	host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, "api")
	if err != nil {
		return err
	}
	protocol := strings.ToLower(strings.TrimSpace(analysis.WorkloadProtocols[selected[0]]))
	if protocol == "https" {
		ca := filepath.Join(e.files.Bindings, "runtime-identity", "ca.pem")
		if info, statErr := os.Stat(ca); statErr != nil || !info.Mode().IsRegular() {
			if statErr != nil {
				return fmt.Errorf("HTTPS workload service %q requires BaseHarbor workload TLS trust %s: %w", selected[0], ca, statErr)
			}
			return fmt.Errorf("HTTPS workload service %q requires BaseHarbor workload TLS trust %s to be a regular file", selected[0], ca)
		}
	}
	plan.appRoutes = append(plan.appRoutes, developmentWorkloadRoute(
		plan.appOwner,
		host,
		application.DevelopmentWorkloadAlias(e.manifest),
		application.DevelopmentWorkloadNetworkNameForProject(e.files.ResourceProject),
		e.files,
		selected[0],
		ports[0],
		protocol,
	))
	return nil
}

func (e *applicationApplyExecution) addDevelopmentExposureRoutes(_ context.Context, plan *developmentRoutePlan) error {
	if len(e.manifest.Exposures) > 0 {
		state, providerFiles, err := exposure.Load(e.files)
		if err != nil {
			return fmt.Errorf("load managed exposure for canonical routes: %w", err)
		}
		public := make([]exposure.Route, 0, len(state.Routes))
		for _, route := range state.Routes {
			if !strings.EqualFold(strings.TrimSpace(route.Visibility), "internal") {
				public = append(public, route)
			}
		}
		for _, route := range public {
			label := strings.TrimSpace(route.Name)
			if len(public) == 1 {
				label = "api"
			} else if label == "" {
				label = route.Service
			}
			host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, label)
			if err != nil {
				return err
			}
			containerPort := 8080
			upstreamScheme := "http"
			trustFile := ""
			serverName := ""
			providerProtocol := strings.ToLower(strings.TrimSpace(route.ProviderProtocol))
			if providerProtocol == "" {
				providerProtocol = strings.ToLower(strings.TrimSpace(route.Protocol))
			}
			if providerProtocol == "https" {
				containerPort = 8443
				upstreamScheme = "https"
				trustFile = filepath.Join(providerFiles.Dir, "routes", route.Name, "cert.pem")
				serverName = state.Host
			}
			plan.appRoutes = append(plan.appRoutes, devgateway.Route{
				Key:        plan.appOwner + "/exposure/" + route.Name,
				Host:       host,
				Upstream:   fmt.Sprintf("%s://%s:%d", upstreamScheme, devaccess.ProviderAlias(state.Project, route.Name), containerPort),
				Network:    state.Network,
				TrustFile:  trustFile,
				ServerName: serverName,
			})
		}
	}
	if !application.RequiresRuntimeBroker(e.manifest) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(e.files)
	if err != nil {
		return err
	}
	if strings.TrimSpace(brokerFiles.DocsURL) == "" {
		return nil
	}
	host, err := devaccess.ApplicationHost(plan.target, e.manifest.Name, "api")
	if err != nil {
		return err
	}
	plan.appRoutes = append(plan.appRoutes, devgateway.Route{
		Key:        plan.appOwner + "/runtime-docs",
		Host:       host,
		PathPrefix: "/swagger",
		Upstream:   "https://baseharbor-runtime:8081",
		Network:    application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
		TrustFile:  filepath.Join(e.files.Bindings, "runtime-identity", "ca.pem"),
		ServerName: "baseharbor-runtime",
	})
	return nil
}

func (e *applicationApplyExecution) addDevelopmentManagementRoutes(_ context.Context, plan *developmentRoutePlan) error {
	if e.manifest.Services.ObjectStorageManagementUI {
		placement, err := application.ResolveProviderPlacement(e.manifest, capability.ProviderSeaweedFS)
		if err != nil {
			return err
		}
		if placement.Scope != capability.ScopeExternal {
			files, err := objectstorage.ExistingProviderFilesAt(e.resolved.TargetStateRoot, plan.target)
			if err != nil {
				return err
			}
			var host string
			owner := plan.appOwner
			key := plan.appOwner + "/storage"
			if placement.Scope == capability.ScopeShared {
				host, err = devaccess.SharedHost(plan.target, "storage")
				owner = "shared/object-storage"
				key = owner
			} else {
				host, err = devaccess.ApplicationHost(plan.target, e.manifest.Name, "storage")
			}
			if err != nil {
				return err
			}
			plan.groups = append(plan.groups, devgateway.OwnerRoutes{
				Owner: owner,
				Routes: []devgateway.Route{{
					Key: key, Host: host,
					Upstream:   "https://seaweedfs-admin-access:9443",
					Network:    files.Network,
					TrustFile:  filepath.Join(files.Dir, "management-ui", "service-access", "pki", "ca.pem"),
					ServerName: "localhost",
				}},
			})
		}
	}
	if e.manifest.Services.SecretsManagementUI {
		host, err := devaccess.SharedHost(plan.target, "openbao")
		if err != nil {
			return err
		}
		resourceProject := strings.TrimSpace(e.platformFiles.ResourceProject)
		if resourceProject == "" {
			resourceProject = strings.TrimSpace(e.platformFiles.Project)
		}
		plan.groups = append(plan.groups, devgateway.OwnerRoutes{
			Owner: "shared/openbao",
			Routes: []devgateway.Route{{
				Key: "shared/openbao", Host: host,
				Upstream:   "https://openbao:8200",
				Network:    bhruntime.ControlPlaneNetworkName(resourceProject),
				TrustFile:  filepath.Join(filepath.Dir(e.platformFiles.Compose), "providers", "openbao", "service-access", "pki", "ca.pem"),
				ServerName: "openbao",
			}},
		})
	}
	if !e.manifest.Services.ObservabilityManagementUI {
		return nil
	}
	placement, err := metrics.PlacementForAt(e.resolved.TargetStateRoot, plan.target, e.manifest)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := metrics.ExistingProviderFilesAt(e.resolved.TargetStateRoot, plan.target, e.manifest)
	if err != nil {
		return err
	}
	service := "prometheus-access"
	host, err := devaccess.SharedHost(plan.target, "prometheus")
	owner := "shared/prometheus"
	key := "shared/prometheus"
	if placement.Scope == capability.ScopeApplication {
		service = "baseharbor-internal-prometheus-access"
		host, err = devaccess.ApplicationHost(plan.target, e.manifest.Name, "prometheus")
		owner = plan.appOwner + "/prometheus"
		key = owner
	}
	if err != nil {
		return err
	}
	plan.groups = append(plan.groups, devgateway.OwnerRoutes{
		Owner: owner,
		Routes: []devgateway.Route{{
			Key: key, Host: host,
			Upstream:   "https://" + service + ":9090",
			Network:    metrics.PublishNetworkName(placement.Project),
			TrustFile:  filepath.Join(files.Dir, "service-access", "pki", "ca.pem"),
			ServerName: "prometheus",
		}},
	})
	return nil
}
