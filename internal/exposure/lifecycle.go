package exposure

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// PlannedRoute is the runtime-neutral exposure intent handed to a realization.
type PlannedRoute struct {
	Name       string
	Service    string
	TargetPort int
	Protocol   string
	Visibility string
}

// PublishedState is the runtime-neutral exposure result consumed by Core and
// user/agent surfaces. Runtime-native project/network/resource identity stays
// below the realization boundary.
type PublishedState struct {
	Host   string
	Routes []Route
}

// Realization owns provider/runtime-specific delivery. Docker/Podman may use
// Caddy, Kubernetes may use Gateway API, OpenShift may use Route, and cloud
// targets may use a managed ingress implementation without changing Core.
type Realization interface {
	Apply(context.Context, []PlannedRoute) (PublishedState, error)
	Reconcile(context.Context, []PlannedRoute) (PublishedState, error)
	Verify(context.Context, Route) error
	Rollback(context.Context) error
}

// Lifecycle is the frozen semantic exposure driver.
type Lifecycle struct {
	hostname    string
	tlsMode     string
	realization Realization
	planned     map[string]PlannedRoute
	state       PublishedState
	provisioned bool
}

func NewLifecycle(hostname, tlsMode string, realization Realization) *Lifecycle {
	return &Lifecycle{
		hostname:    strings.TrimSpace(hostname),
		tlsMode:     strings.TrimSpace(tlsMode),
		realization: realization,
		planned:     map[string]PlannedRoute{},
	}
}

func (d *Lifecycle) Descriptor() capability.Provider { return capability.Caddy }

func (d *Lifecycle) IntegrationDescriptor() capability.IntegrationDescriptor {
	return capability.CaddyIntegration
}

func (d *Lifecycle) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.ExposureHTTP {
		return fmt.Errorf("managed exposure cannot satisfy capability %q", resource.Kind)
	}
	if d.realization == nil {
		return errors.New("managed exposure realization is required")
	}
	if d.hostname == "" {
		return errors.New("managed HTTP exposure requires repository deployment initialization; run 'baha app init'")
	}
	if binding.HTTPExposure == nil {
		return fmt.Errorf("HTTP exposure %q binding metadata is required", resource.Name)
	}
	route := *binding.HTTPExposure
	if strings.TrimSpace(route.Service) == "" || route.TargetPort < 1 || route.TargetPort > 65535 {
		return fmt.Errorf("HTTP exposure %q has invalid logical endpoint metadata", resource.Name)
	}
	if route.Protocol != "http" && route.Protocol != "https" {
		return fmt.Errorf("HTTP exposure %q protocol %q is unsupported", resource.Name, route.Protocol)
	}
	if route.Visibility != "public" && route.Visibility != "internal" {
		return fmt.Errorf("HTTP exposure %q visibility %q is unsupported", resource.Name, route.Visibility)
	}
	if binding.Workload != "service/"+route.Service {
		return fmt.Errorf("HTTP exposure %q binding targets %q, expected service/%s", resource.Name, binding.Workload, route.Service)
	}
	if route.Protocol == "https" && d.tlsMode != "existing" {
		return fmt.Errorf("managed HTTPS exposure %q currently requires existing/BYOC TLS; deployment TLS mode is %q", resource.Name, d.tlsMode)
	}
	if err := capability.RequireIntegrationContract(d.IntegrationDescriptor()); err != nil {
		return err
	}
	planned := PlannedRoute{
		Name:       resource.Name,
		Service:    route.Service,
		TargetPort: route.TargetPort,
		Protocol:   route.Protocol,
		Visibility: route.Visibility,
	}
	if previous, exists := d.planned[resource.Name]; exists && previous != planned {
		return fmt.Errorf("HTTP exposure %q has conflicting preflight bindings", resource.Name)
	}
	d.planned[resource.Name] = planned
	return nil
}

func (d *Lifecycle) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if d.provisioned {
		return nil
	}
	state, err := d.realization.Apply(ctx, d.plan())
	if err != nil {
		return err
	}
	d.state = state
	d.provisioned = true
	return nil
}

func (d *Lifecycle) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	route, ok := d.stateRoute(resource.Name)
	if !ok {
		return fmt.Errorf("managed exposure %q was not materialized", resource.Name)
	}
	if binding.HTTPExposure == nil {
		return fmt.Errorf("HTTP exposure %q binding metadata is required", resource.Name)
	}
	if binding.Workload != "service/"+route.Service {
		return fmt.Errorf("managed exposure %q has invalid workload binding %q", resource.Name, binding.Workload)
	}
	return nil
}

func (d *Lifecycle) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	route, ok := d.stateRoute(resource.Name)
	if !ok {
		return fmt.Errorf("managed exposure %q state is missing", resource.Name)
	}
	return d.realization.Verify(ctx, route)
}

func (d *Lifecycle) ReconcileWorkloadTransport(ctx context.Context) error {
	if !d.provisioned {
		return errors.New("managed exposure must be provisioned before workload transport reconciliation")
	}
	state, err := d.realization.Reconcile(ctx, d.plan())
	if err != nil {
		return err
	}
	d.state = state
	return nil
}

func (d *Lifecycle) Rollback(ctx context.Context) error {
	if d.realization == nil {
		return nil
	}
	err := d.realization.Rollback(ctx)
	d.provisioned = false
	return err
}

func (d *Lifecycle) State() PublishedState { return d.state }

func (d *Lifecycle) plan() []PlannedRoute {
	names := make([]string, 0, len(d.planned))
	for name := range d.planned {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]PlannedRoute, 0, len(names))
	for _, name := range names {
		out = append(out, d.planned[name])
	}
	return out
}

func (d *Lifecycle) stateRoute(name string) (Route, bool) {
	for _, route := range d.state.Routes {
		if route.Name == name {
			return route, true
		}
	}
	return Route{}, false
}

// CaddyRealization adapts the current Docker/Podman Caddy implementation to
// the runtime-neutral Lifecycle without exposing Compose vocabulary upward.
type CaddyRealization struct {
	driver *Driver
}

func NewCaddyRealization(driver *Driver) *CaddyRealization {
	return &CaddyRealization{driver: driver}
}

func (r *CaddyRealization) Apply(ctx context.Context, planned []PlannedRoute) (PublishedState, error) {
	if r == nil || r.driver == nil {
		return PublishedState{}, errors.New("Caddy realization is required")
	}
	for _, route := range planned {
		resource, binding := plannedRouteCapability(route)
		if err := r.driver.Preflight(ctx, resource, binding); err != nil {
			return PublishedState{}, err
		}
	}
	if len(planned) > 0 {
		resource, binding := plannedRouteCapability(planned[0])
		if err := r.driver.Provision(ctx, resource, binding); err != nil {
			return PublishedState{}, err
		}
	}
	return publishedState(r.driver.State()), nil
}

func (r *CaddyRealization) Reconcile(ctx context.Context, planned []PlannedRoute) (PublishedState, error) {
	if err := r.driver.ReconcileWorkloadTransport(ctx); err != nil {
		return PublishedState{}, err
	}
	return publishedState(r.driver.State()), nil
}

func (r *CaddyRealization) Verify(ctx context.Context, route Route) error {
	resource, binding := plannedRouteCapability(PlannedRoute{
		Name:       route.Name,
		Service:    route.Service,
		TargetPort: route.TargetPort,
		Protocol:   route.Protocol,
		Visibility: route.Visibility,
	})
	return r.driver.Verify(ctx, resource, binding)
}

func (r *CaddyRealization) Rollback(ctx context.Context) error {
	return r.driver.Rollback(ctx)
}

func plannedRouteCapability(route PlannedRoute) (capability.Resource, capability.Binding) {
	resource := capability.Resource{
		Kind:     capability.ExposureHTTP,
		Name:     route.Name,
		Provider: capability.ProviderCaddy,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/" + route.Service,
		HTTPExposure: &capability.HTTPExposureBinding{
			Service:    route.Service,
			TargetPort: route.TargetPort,
			Protocol:   route.Protocol,
			Visibility: route.Visibility,
		},
	}
	return resource, binding
}

func publishedState(state State) PublishedState {
	return PublishedState{
		Host:   state.Host,
		Routes: append([]Route(nil), state.Routes...),
	}
}
