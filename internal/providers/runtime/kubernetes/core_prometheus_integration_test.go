package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/metrics"
)

type kubernetesPrometheusRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	instance    metrics.PrometheusInstance
}

func (r *kubernetesPrometheusRealization) Apply(ctx context.Context) (metrics.PrometheusInstance, error) {
	if r.instance.HTTPClient != nil {
		return r.instance, nil
	}
	base := dnsLabel(r.application)
	cleanup := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"delete", "deployment/"+base+"-prometheus", "service/"+base+"-prometheus", "configmap/"+base+"-prometheus-config",
		"-n", r.namespace,
		"--ignore-not-found=true",
		"--wait=true",
	)
	if output, err := cleanup.CombinedOutput(); err != nil {
		return metrics.PrometheusInstance{}, fmt.Errorf("clean stale Prometheus proof resources: %w: %s", err, strings.TrimSpace(string(output)))
	}

	config := prometheusProofConfig(nil)
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-prometheus-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  prometheus.yml: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-prometheus
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: prometheus
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: prometheus
    spec:
      containers:
        - name: prometheus
          image: %s
          args:
            - --config.file=/etc/prometheus/prometheus.yml
          ports:
            - containerPort: 9090
          readinessProbe:
            httpGet:
              path: /-/ready
              port: 9090
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: config
              mountPath: /etc/prometheus
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: %s-prometheus-config
---
apiVersion: v1
kind: Service
metadata:
  name: %s-prometheus
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
    baseharbor.io/workload-service: prometheus
  ports:
    - port: 9090
      targetPort: 9090
`,
		base, r.namespace, base, dnsLabel(r.environment), indentYAMLBlock(config, 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		metrics.ProviderImage, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)
	apply := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		return metrics.PrometheusInstance{}, fmt.Errorf("apply Prometheus proof: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := r.waitRollout(ctx); err != nil {
		return metrics.PrometheusInstance{}, err
	}
	clusterIP, err := r.serviceClusterIP(ctx, base+"-prometheus")
	if err != nil {
		return metrics.PrometheusInstance{}, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	r.instance = metrics.PrometheusInstance{
		Endpoint:   "http://" + net.JoinHostPort(clusterIP, "9090"),
		HTTPClient: client,
	}
	return r.instance, nil
}

func (r *kubernetesPrometheusRealization) Existing(context.Context) (metrics.PrometheusInstance, error) {
	if r.instance.HTTPClient == nil {
		return metrics.PrometheusInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesPrometheusRealization) RegisterTarget(ctx context.Context, target metrics.PrometheusTarget) error {
	base := dnsLabel(r.application)
	serviceName := base + "-" + dnsLabel(target.Service) + "." + r.namespace + ".svc"
	config := prometheusProofConfig(&target)
	configMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-prometheus-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  prometheus.yml: |
%s
`, base, r.namespace, base, dnsLabel(r.environment), indentYAMLBlock(strings.ReplaceAll(config, "__TARGET__", net.JoinHostPort(serviceName, fmt.Sprint(target.Port))), 4))
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(configMap)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("update Prometheus target config: %w: %s", err, strings.TrimSpace(string(output)))
	}
	restart := exec.CommandContext(ctx, r.provider.KubectlPath(), "rollout", "restart", "deployment/"+base+"-prometheus", "-n", r.namespace)
	if output, err := restart.CombinedOutput(); err != nil {
		return fmt.Errorf("restart Prometheus after target update: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return r.waitRollout(ctx)
}

func (r *kubernetesPrometheusRealization) Destroy(ctx context.Context) error {
	r.instance = metrics.PrometheusInstance{}
	return r.provider.Destroy(ctx, r.application, r.environment, r.namespace)
}

func (r *kubernetesPrometheusRealization) waitRollout(ctx context.Context) error {
	base := dnsLabel(r.application)
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "rollout", "status", "deployment/"+base+"-prometheus", "-n", r.namespace, "--timeout=180s")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wait for Prometheus: %w: %s\n%s", err, strings.TrimSpace(string(output)),
			kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, "prometheus"))
	}
	return nil
}

func (r *kubernetesPrometheusRealization) serviceClusterIP(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "get", "service", name, "-n", r.namespace, "-o", "jsonpath={.spec.clusterIP}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve %s ClusterIP: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	ip := strings.TrimSpace(string(output))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid %s ClusterIP %q", name, ip)
	}
	return ip, nil
}

func prometheusProofConfig(target *metrics.PrometheusTarget) string {
	var b strings.Builder
	b.WriteString("global:\n  scrape_interval: 1s\n")
	if target == nil {
		b.WriteString("scrape_configs: []\n")
		return b.String()
	}
	fmt.Fprintf(&b, "scrape_configs:\n  - job_name: baseharbor-applications\n")
	fmt.Fprintf(&b, "    scheme: %s\n", target.Scheme)
	fmt.Fprintf(&b, "    metrics_path: %s\n", target.Path)
	b.WriteString("    static_configs:\n      - targets: [\"__TARGET__\"]\n        labels:\n")
	fmt.Fprintf(&b, "          baseharbor_application: %q\n", target.Application)
	fmt.Fprintf(&b, "          baseharbor_environment: %q\n", target.Environment)
	fmt.Fprintf(&b, "          baseharbor_service: %q\n", target.Service)
	fmt.Fprintf(&b, "          baseharbor_source: %q\n", target.Source)
	fmt.Fprintf(&b, "          baseharbor_source_class: %q\n", string(application.MetricsSourceApplication))
	fmt.Fprintf(&b, "          baseharbor_metrics_path: %q\n", target.Path)
	fmt.Fprintf(&b, "          baseharbor_metrics_scheme: %q\n", target.Scheme)
	return b.String()
}

func TestKubernetesCorePrometheusLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_PROMETHEUS_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_PROMETHEUS_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	provider, err := Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	namespace := strings.TrimSpace(os.Getenv("BASEHARBOR_KUBERNETES_TEST_NAMESPACE"))
	if namespace == "" {
		namespace = provider.Namespace()
	}

	const appName = "bh-core-prometheus-proof"
	const environment = "dev"
	const sourceName = "http"
	const serviceName = "api"
	const sourcePort = 8080

	app := application.WithMetricsSource(application.Manifest{
		Version:     application.CurrentVersion,
		Name:        appName,
		Environment: environment,
	}, sourceName, serviceName, sourcePort, "/metrics")

	base := dnsLabel(appName)
	sourceManifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-metrics-source
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  metrics: |
    # TYPE baseharbor_proof gauge
    baseharbor_proof 1
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-api
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: api
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: api
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: api
    spec:
      containers:
        - name: metrics
          image: docker.io/library/python:3.13-alpine
          command: ["python3", "-m", "http.server", "%d", "--directory", "/metrics"]
          ports:
            - containerPort: %d
          readinessProbe:
            tcpSocket:
              port: %d
            initialDelaySeconds: 1
            periodSeconds: 2
          volumeMounts:
            - name: metrics
              mountPath: /metrics/metrics
              subPath: metrics
      volumes:
        - name: metrics
          configMap:
            name: %s-metrics-source
---
apiVersion: v1
kind: Service
metadata:
  name: %s-api
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
    baseharbor.io/workload-service: api
  ports:
    - port: %d
      targetPort: %d
`,
		base, namespace, base, dnsLabel(environment),
		base, namespace, base, dnsLabel(environment),
		base, dnsLabel(environment),
		base, dnsLabel(environment),
		sourcePort, sourcePort, sourcePort, base,
		base, namespace, base, dnsLabel(environment),
		base, dnsLabel(environment), sourcePort, sourcePort,
	)
	apply := exec.CommandContext(ctx, provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = strings.NewReader(sourceManifest)
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply metrics source: %v: %s", err, strings.TrimSpace(string(output)))
	}
	rollout := exec.CommandContext(ctx, provider.KubectlPath(), "rollout", "status", "deployment/"+base+"-api", "-n", namespace, "--timeout=120s")
	if output, err := rollout.CombinedOutput(); err != nil {
		t.Fatalf("wait metrics source: %v: %s", err, strings.TrimSpace(string(output)))
	}

	realization := &kubernetesPrometheusRealization{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	driver := metrics.NewDriverWithRealization(realization, app)
	resource := capability.Resource{
		Application: appName,
		Kind:        capability.Metrics,
		Name:        sourceName,
		Provider:    capability.ProviderPrometheus,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/" + serviceName,
		Metrics: &capability.MetricsBinding{
			Direction: "provide",
			Format:    "openmetrics",
			Service:   serviceName,
			Scheme:    "http",
			Port:      sourcePort,
			Path:      "/metrics",
		},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("Prometheus Core preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("Prometheus Core provision: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("Prometheus Core bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("Prometheus Core verify: %v", err)
	}
}
