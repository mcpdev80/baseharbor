package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
)

type kubernetesLokiRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	instance    logs.LokiInstance
}

func (r *kubernetesLokiRealization) Apply(ctx context.Context) (logs.LokiInstance, error) {
	if r.instance.HTTPClient != nil {
		return r.instance, nil
	}
	base := dnsLabel(r.application)
	cleanup := exec.CommandContext(
		ctx, r.provider.KubectlPath(),
		"delete",
		"deployment/"+base+"-loki",
		"service/"+base+"-loki",
		"configmap/"+base+"-loki-config",
		"-n", r.namespace,
		"--ignore-not-found=true",
		"--wait=true",
	)
	if output, err := cleanup.CombinedOutput(); err != nil {
		return logs.LokiInstance{}, fmt.Errorf("clean stale Loki proof resources: %w: %s", err, strings.TrimSpace(string(output)))
	}

	config := `auth_enabled: false
server:
  http_listen_address: 0.0.0.0
  http_listen_port: 3100
common:
  path_prefix: /loki
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2020-05-15
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
storage_config:
  filesystem:
    directory: /loki/chunks
`
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-loki-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  loki.yaml: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-loki
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: loki
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: loki
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: loki
    spec:
      containers:
        - name: loki
          image: %s
          args: ["-config.file=/etc/loki/loki.yaml"]
          ports:
            - containerPort: 3100
          readinessProbe:
            httpGet:
              path: /ready
              port: 3100
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: config
              mountPath: /etc/loki
              readOnly: true
            - name: data
              mountPath: /loki
      volumes:
        - name: config
          configMap:
            name: %s-loki-config
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %s-loki
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
    baseharbor.io/workload-service: loki
  ports:
    - port: 3100
      targetPort: 3100
`,
		base, r.namespace, base, dnsLabel(r.environment), indentYAMLBlock(config, 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		logs.LokiImage, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return logs.LokiInstance{}, fmt.Errorf("apply Loki proof: %w: %s", err, strings.TrimSpace(string(output)))
	}
	rollout := exec.CommandContext(ctx, r.provider.KubectlPath(), "rollout", "status", "deployment/"+base+"-loki", "-n", r.namespace, "--timeout=180s")
	if output, err := rollout.CombinedOutput(); err != nil {
		return logs.LokiInstance{}, fmt.Errorf("wait for Loki: %w: %s\n%s", err, strings.TrimSpace(string(output)),
			kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, "loki"))
	}
	ipCmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "get", "service", base+"-loki", "-n", r.namespace, "-o", "jsonpath={.spec.clusterIP}")
	out, err := ipCmd.CombinedOutput()
	if err != nil {
		return logs.LokiInstance{}, fmt.Errorf("resolve Loki ClusterIP: %w: %s", err, strings.TrimSpace(string(out)))
	}
	ip := strings.TrimSpace(string(out))
	if net.ParseIP(ip) == nil {
		return logs.LokiInstance{}, fmt.Errorf("invalid Loki ClusterIP %q", ip)
	}
	r.instance = logs.LokiInstance{
		Endpoint:   "http://" + net.JoinHostPort(ip, "3100"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	return r.instance, nil
}

func (r *kubernetesLokiRealization) Existing(context.Context) (logs.LokiInstance, error) {
	if r.instance.HTTPClient == nil {
		return logs.LokiInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesLokiRealization) RegisterSource(ctx context.Context, source logs.LogSource) error {
	if source.Application != r.application || source.Environment != r.environment || source.Service == "" {
		return fmt.Errorf("invalid Kubernetes log source identity")
	}
	instance, err := r.Existing(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"streams": []any{
			map[string]any{
				"stream": map[string]string{
					"baseharbor_application":  source.Application,
					"baseharbor_environment":  source.Environment,
					"baseharbor_source_class": string(source.Class),
					"baseharbor_service":      source.Service,
				},
				"values": [][]string{{
					strconv.FormatInt(time.Now().UnixNano(), 10),
					"baseharbor kubernetes runtime-stream proof",
				}},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, instance.Endpoint+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := instance.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("push Kubernetes proof log: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push Kubernetes proof log: Loki returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (r *kubernetesLokiRealization) VerifySource(ctx context.Context, source logs.LogSource) error {
	instance, err := r.Existing(ctx)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		`{baseharbor_application=%q,baseharbor_environment=%q,baseharbor_source_class=%q,baseharbor_service=%q}`,
		source.Application, source.Environment, string(source.Class), source.Service,
	)
	deadline := time.Now().Add(45 * time.Second)
	for {
		u := instance.Endpoint + "/loki/api/v1/query_range?query=" + url.QueryEscape(query) + "&limit=10"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := instance.HTTPClient.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 &&
				bytes.Contains(data, []byte("baseharbor kubernetes runtime-stream proof")) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("normalized Kubernetes log source was not queryable in Loki")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (r *kubernetesLokiRealization) Destroy(ctx context.Context) error {
	r.instance = logs.LokiInstance{}
	base := dnsLabel(r.application)
	cmd := exec.CommandContext(
		ctx, r.provider.KubectlPath(),
		"delete",
		"deployment/"+base+"-loki",
		"service/"+base+"-loki",
		"configmap/"+base+"-loki-config",
		"-n", r.namespace,
		"--ignore-not-found=true",
		"--wait=true",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("destroy Loki proof resources: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func TestKubernetesCoreLokiLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_LOKI_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_LOKI_TEST=1")
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

	const appName = "bh-core-loki-proof"
	const environment = "dev"
	const service = "api"
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        appName,
		Environment: environment,
		Logs: application.LogsRequirements{
			Collect: []string{"application"},
		},
	}
	realization := &kubernetesLokiRealization{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	driver := logs.NewDriverWithRealization(realization, app)
	resource := capability.Resource{
		Application: appName,
		Kind:        capability.Logs,
		Name:        service,
		Provider:    capability.ProviderLoki,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "service/" + service,
		Logs: &capability.LogsBinding{
			Direction: "collect",
			Format:    "runtime-stream",
			Service:   service,
		},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("Loki Core preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("Loki Core provision: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("Loki Core bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("Loki Core verify: %v", err)
	}
}
