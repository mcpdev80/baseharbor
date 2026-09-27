package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func requiresDevelopmentGateway(m application.Manifest) bool {
	if !devaccess.Enabled(m.Environment) {
		return false
	}
	return len(m.Exposures) > 0 ||
		m.Services.SQLManagementUI ||
		m.Services.CacheManagementUI ||
		m.Services.ObjectStorageManagementUI ||
		m.Services.SecretsManagementUI ||
		m.Services.IdentityManagementUI ||
		m.Services.ObservabilityManagementUI ||
		m.Services.Identity
}

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
		include := route.Owner == appOwner
		if m.Services.SecretsManagementUI && route.Owner == "shared:openbao" {
			include = true
		}
		if m.Services.ObservabilityManagementUI && route.Owner == "shared:prometheus" {
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

func (e *applicationApplyExecution) reconcileDevelopmentGateway(ctx context.Context) error {
	if !devaccess.Enabled(e.manifest.Environment) {
		return nil
	}
	routes, err := e.developmentGatewayRoutes()
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return nil
	}

	appOwner := "app/" + e.manifest.Name + "/" + e.manifest.Environment
	groups := map[string][]devgateway.Route{appOwner: nil}
	for _, route := range routes {
		owner := appOwner
		switch {
		case strings.HasPrefix(route.Key, "shared:"):
			owner = route.Key
		case strings.HasPrefix(route.Key, "metrics:shared:"):
			owner = "shared:prometheus"
		}
		groups[owner] = append(groups[owner], route)
	}

	owners := make([]string, 0, len(groups))
	for owner := range groups {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	desired := make([]devgateway.OwnerRoutes, 0, len(owners))
	for _, owner := range owners {
		desired = append(desired, devgateway.OwnerRoutes{Owner: owner, Routes: groups[owner]})
	}
	if err := devgateway.ReplaceRoutes(ctx, e.compose, e.issuer, e.resolved.Target.Name, desired...); err != nil {
		return fmt.Errorf("reconcile canonical development gateway: %w", err)
	}
	return nil
}

func (e *applicationApplyExecution) developmentGatewayRoutes() ([]devgateway.Route, error) {
	target := e.resolved.Target.Name
	m := e.manifest
	var routes []devgateway.Route

	appNetwork := application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject)
	if m.Services.SQLManagementUI {
		host, err := devaccess.ApplicationHost(target, m.Name, "pgadmin")
		if err != nil { return nil, err }
		policy, err := serviceaccess.Resolve(m.Environment, "pgadmin", serviceaccess.AuthenticationNative)
		if err != nil { return nil, err }
		material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(e.files.Dir, "providers", "management-ui", "postgres", "pki"))
		if err != nil { return nil, err }
		routes = append(routes, devgateway.Route{
			Key: "app:"+m.Name+":pgadmin", Host: host, Network: appNetwork,
			Upstream: "https://"+devaccess.ApplicationAlias(m.Name, "pgadmin")+":8443",
			TrustFile: material.CA, ServerName: material.ServerName,
		})
	}
	if m.Services.CacheManagementUI {
		host, err := devaccess.ApplicationHost(target, m.Name, "cache")
		if err != nil { return nil, err }
		policy, err := serviceaccess.Resolve(m.Environment, "redis-commander", serviceaccess.AuthenticationNative)
		if err != nil { return nil, err }
		material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(e.files.Dir, "providers", "management-ui", "cache", "pki"))
		if err != nil { return nil, err }
		routes = append(routes, devgateway.Route{
			Key: "app:"+m.Name+":cache", Host: host, Network: appNetwork,
			Upstream: "https://"+devaccess.ApplicationAlias(m.Name, "cache")+":8443",
			TrustFile: material.CA, ServerName: material.ServerName,
		})
	}
	if m.Services.ObjectStorageManagementUI {
		placement, err := application.ResolveProviderPlacement(m, capability.ProviderSeaweedFS)
		if err != nil { return nil, err }
		if placement.Scope != capability.ScopeExternal {
			var host string
			if placement.Scope == capability.ScopeShared {
				host, err = devaccess.SharedHost(target, "storage")
			} else {
				host, err = devaccess.ApplicationHost(target, m.Name, "storage")
			}
			if err != nil { return nil, err }
			files, err := objectstorage.ExistingProviderFilesAt(e.resolved.TargetStateRoot, target)
			if err != nil { return nil, err }
			policy, err := serviceaccess.Resolve("prod", "seaweedfs-admin", serviceaccess.AuthenticationNative)
			if err != nil { return nil, err }
			material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "management-ui", "service-access", "pki"))
			if err != nil { return nil, err }
			key := "app:"+m.Name+":storage"
			if placement.Scope == capability.ScopeShared {
				key = "shared:storage"
			}
			routes = append(routes, devgateway.Route{
				Key: key, Host: host, Network: files.Network,
				Upstream: "https://seaweedfs-admin-access:9443",
				TrustFile: material.CA, ServerName: material.ServerName,
			})
		}
	}
	if m.Services.SecretsManagementUI {
		host, err := devaccess.SharedHost(target, "openbao")
		if err != nil { return nil, err }
		policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
		if err != nil { return nil, err }
		root := filepath.Join(filepath.Dir(e.platformFiles.Compose), "providers", "openbao", "service-access", "pki")
		material, err := serviceaccess.ExistingTLSMaterial(policy, root)
		if err != nil { return nil, err }
		resourceProject := strings.TrimSpace(e.platformFiles.ResourceProject)
		if resourceProject == "" { resourceProject = "baseharbor-" + target }
		routes = append(routes, devgateway.Route{
			Key: "shared:openbao", Host: host, Network: bhruntime.ControlPlaneNetworkName(resourceProject),
			Upstream: "https://openbao-access:8443",
			TrustFile: material.CA, ServerName: material.ServerName,
		})
	}
	if m.Services.IdentityManagementUI || m.Services.Identity {
		placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
		if err != nil { return nil, err }
		if placement.Scope != capability.ScopeExternal {
			files, err := identityprovider.ExistingKeycloakFilesAt(m, e.resolved.TargetStateRoot, target)
			if err != nil { return nil, err }
			if m.Services.Identity {
				host, err := devaccess.ApplicationHost(target, m.Name, "identity")
				if err != nil { return nil, err }
				routes = append(routes, devgateway.Route{
					Key: "app:"+m.Name+":identity", Host: host, Network: files.ConsumerNetwork,
					Upstream: "https://"+devaccess.ProviderAlias(files.Project, "identity")+":"+strconv.Itoa(files.PublicPort),
					TrustFile: files.PublicAccess.Material.CA, ServerName: files.PublicAccess.Material.ServerName,
				})
			}
			if m.Services.IdentityManagementUI {
				host, err := devaccess.ApplicationHost(target, m.Name, "identity-admin")
				if err != nil { return nil, err }
				routes = append(routes, devgateway.Route{
					Key: "app:"+m.Name+":identity-admin", Host: host, Network: files.InternalNetwork,
					Upstream: "https://"+devaccess.ProviderAlias(files.Project, "identity-admin")+":9443",
					TrustFile: files.AdminAccess.Material.CA, ServerName: files.AdminAccess.Material.ServerName,
				})
			}
		}
	}
	if m.Services.ObservabilityManagementUI {
		placement, err := metrics.PlacementForAt(e.resolved.TargetStateRoot, target, m)
		if err != nil { return nil, err }
		if placement.Scope != capability.ScopeExternal {
			files, err := metrics.ExistingProviderFilesAt(e.resolved.TargetStateRoot, target, m)
			if err != nil { return nil, err }
			accessEnvironment := m.Environment
			policy, err := serviceaccess.Resolve(accessEnvironment, "prometheus", serviceaccess.AuthenticationMTLS)
			if err != nil { return nil, err }
			material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
			if err != nil { return nil, err }
			var host string
			if placement.Scope == capability.ScopeShared {
				host, err = devaccess.SharedHost(target, "prometheus")
			} else {
				host, err = devaccess.ApplicationHost(target, m.Name, "prometheus")
			}
			if err != nil { return nil, err }
			serviceName := "prometheus-access"
			if placement.Scope == capability.ScopeApplication { serviceName = "baseharbor-internal-prometheus-access" }
			routes = append(routes, devgateway.Route{
				Key: "metrics:"+string(placement.Scope)+":"+m.Name, Host: host,
				Network: metrics.PublishNetworkName(placement.Project),
				Upstream: "https://"+serviceName+":8443",
				TrustFile: material.CA, ServerName: material.ServerName,
			})
		}
	}
	if len(m.Exposures) > 0 {
		state, files, err := exposure.Load(e.files)
		if err != nil { return nil, err }
		publicCount := 0
		for _, route := range state.Routes {
			if !strings.EqualFold(route.Visibility, "internal") {
				publicCount++
			}
		}
		for _, route := range state.Routes {
			service := route.Name
			if !strings.EqualFold(route.Visibility, "internal") {
				service = devaccess.ExposureService(route.Name, publicCount)
			}
			host, err := devaccess.ApplicationHost(target, m.Name, service)
			if err != nil { return nil, err }
			containerPort := 8080
			upstream := "http://"+devaccess.ProviderAlias(state.Project, route.Name)+":8080"
			trustFile := ""
			serverName := ""
			if route.Protocol == "https" {
				containerPort = 8443
				upstream = "https://"+devaccess.ProviderAlias(state.Project, route.Name)+":"+strconv.Itoa(containerPort)
				trustFile = filepath.Join(files.Dir, "routes", route.Name, "cert.pem")
				serverName = state.Host
			}
			routes = append(routes, devgateway.Route{
				Key: "app:"+m.Name+":exposure:"+route.Name, Host: host, Network: state.Network,
				Upstream: upstream, TrustFile: trustFile, ServerName: serverName,
			})
		}
	}
	return routes, nil
}
