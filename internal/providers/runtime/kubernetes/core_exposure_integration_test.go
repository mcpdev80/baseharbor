package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/exposure"
)

type kubernetesExposureRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	state       exposure.PublishedState
}

func (r *kubernetesExposureRealization) Apply(ctx context.Context, planned []exposure.PlannedRoute) (exposure.PublishedState, error) {
	if len(planned) != 1 {
		return exposure.PublishedState{}, fmt.Errorf("proof expects exactly one route, got %d", len(planned))
	}
	if err := r.provider.Destroy(ctx, r.application, r.environment, r.namespace); err != nil {
		return exposure.PublishedState{}, err
	}
	route := planned[0]
	base := dnsLabel(r.application)
	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-web
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: %s
    spec:
      containers:
        - name: web
          image: docker.io/library/nginx:1.29.1-alpine
          ports:
            - containerPort: %d
          readinessProbe:
            httpGet:
              path: /
              port: %d
            initialDelaySeconds: 2
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: %s-exposure
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
spec:
  selector:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: %s
  ports:
    - port: 80
      targetPort: %d
`,
		base, r.namespace, base, dnsLabel(r.environment), route.Service,
		base, dnsLabel(r.environment), route.Service,
		base, dnsLabel(r.environment), route.Service,
		route.TargetPort, route.TargetPort,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment), route.Service, route.TargetPort,
	)
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return exposure.PublishedState{}, fmt.Errorf("apply exposure proof: %w: %s", err, strings.TrimSpace(string(output)))
	}
	rollout := exec.CommandContext(ctx, r.provider.KubectlPath(), "rollout", "status", "deployment/"+base+"-web", "-n", r.namespace, "--timeout=120s")
	if output, err := rollout.CombinedOutput(); err != nil {
		return exposure.PublishedState{}, fmt.Errorf("wait exposure backend: %w: %s", err, strings.TrimSpace(string(output)))
	}
	ipCmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "get", "service", base+"-exposure", "-n", r.namespace, "-o", "jsonpath={.spec.clusterIP}")
	ipOut, err := ipCmd.CombinedOutput()
	if err != nil {
		return exposure.PublishedState{}, fmt.Errorf("resolve exposure ClusterIP: %w: %s", err, strings.TrimSpace(string(ipOut)))
	}
	ip := strings.TrimSpace(string(ipOut))
	if net.ParseIP(ip) == nil {
		return exposure.PublishedState{}, fmt.Errorf("invalid exposure ClusterIP %q", ip)
	}
	state := exposure.PublishedState{
		Host: ip,
		Routes: []exposure.Route{{
			Name:          route.Name,
			Service:       route.Service,
			TargetPort:    route.TargetPort,
			Protocol:      route.Protocol,
			Visibility:    route.Visibility,
			PublishedPort: 80,
		}},
	}
	r.state = state
	return state, nil
}

func (r *kubernetesExposureRealization) Reconcile(ctx context.Context, planned []exposure.PlannedRoute) (exposure.PublishedState, error) {
	if len(r.state.Routes) == 0 {
		return r.Apply(ctx, planned)
	}
	return r.state, nil
}

func (r *kubernetesExposureRealization) Verify(ctx context.Context, route exposure.Route) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(r.state.Host, fmt.Sprint(route.PublishedPort))+"/", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("verify Kubernetes exposure: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("verify Kubernetes exposure returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (r *kubernetesExposureRealization) Rollback(ctx context.Context) error {
	r.state = exposure.PublishedState{}
	return r.provider.Destroy(ctx, r.application, r.environment, r.namespace)
}

func TestKubernetesCoreExposureLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_EXPOSURE_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_EXPOSURE_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	provider, err := Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	namespace := strings.TrimSpace(os.Getenv("BASEHARBOR_KUBERNETES_TEST_NAMESPACE"))
	if namespace == "" {
		namespace = provider.Namespace()
	}

	const appName = "bh-core-exposure-proof"
	const environment = "dev"
	realization := &kubernetesExposureRealization{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	driver := exposure.NewLifecycle("exposure.baseharbor.local", "disabled", realization)
	resource := capability.Resource{
		Application: appName,
		Kind: capability.ExposureHTTP,
		Name: "public",
		Provider: capability.ProviderCaddy,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/web",
		HTTPExposure: &capability.HTTPExposureBinding{
			Service: "web", TargetPort: 80, Protocol: "http", Visibility: "internal",
		},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("exposure preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		_ = realization.Rollback(cleanupCtx)
	})
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("exposure provision: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("exposure bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("exposure verify: %v", err)
	}
	if err := driver.ReconcileWorkloadTransport(ctx); err != nil {
		t.Fatalf("exposure reconcile: %v", err)
	}
	if state := driver.State(); state.Host == "" || len(state.Routes) != 1 {
		t.Fatalf("unexpected published exposure state: %#v", state)
	}
}
