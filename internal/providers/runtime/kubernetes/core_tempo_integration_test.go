package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

type kubernetesTempoRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	instance    traces.TempoInstance
}

func (r *kubernetesTempoRealization) Apply(ctx context.Context) (traces.TempoInstance, error) {
	if r.instance.HTTPClient != nil {
		return r.instance, nil
	}
	base := dnsLabel(r.application)
	cleanup := exec.CommandContext(ctx, r.provider.KubectlPath(),
		"delete",
		"deployment/"+base+"-tempo",
		"service/"+base+"-tempo",
		"configmap/"+base+"-tempo-config",
		"-n", r.namespace,
		"--ignore-not-found=true",
		"--wait=true",
	)
	if output, err := cleanup.CombinedOutput(); err != nil {
		return traces.TempoInstance{}, fmt.Errorf("clean stale Tempo proof resources: %w: %s", err, strings.TrimSpace(string(output)))
	}

	config := `server:
  http_listen_port: 3200
distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: 0.0.0.0:4318
storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/traces
usage_report:
  reporting_enabled: false
`
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-tempo-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  tempo.yaml: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-tempo
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: tempo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: tempo
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: tempo
    spec:
      containers:
        - name: tempo
          image: %s
          args: ["-config.file=/etc/tempo/tempo.yaml"]
          ports:
            - containerPort: 3200
            - containerPort: 4318
          readinessProbe:
            httpGet:
              path: /ready
              port: 3200
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: config
              mountPath: /etc/tempo
              readOnly: true
            - name: data
              mountPath: /var/tempo
      volumes:
        - name: config
          configMap:
            name: %s-tempo-config
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %s-tempo
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
    baseharbor.io/workload-service: tempo
  ports:
    - name: query
      port: 3200
      targetPort: 3200
    - name: otlp-http
      port: 4318
      targetPort: 4318
`,
		base, r.namespace, base, dnsLabel(r.environment), indentYAMLBlock(config, 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		traces.ProviderImage, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return traces.TempoInstance{}, fmt.Errorf("apply Tempo proof: %w: %s", err, strings.TrimSpace(string(output)))
	}
	rollout := exec.CommandContext(ctx, r.provider.KubectlPath(), "rollout", "status", "deployment/"+base+"-tempo", "-n", r.namespace, "--timeout=180s")
	if output, err := rollout.CombinedOutput(); err != nil {
		return traces.TempoInstance{}, fmt.Errorf("wait for Tempo: %w: %s\n%s", err, strings.TrimSpace(string(output)),
			kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, "tempo"))
	}
	ipCmd := exec.CommandContext(ctx, r.provider.KubectlPath(), "get", "service", base+"-tempo", "-n", r.namespace, "-o", "jsonpath={.spec.clusterIP}")
	out, err := ipCmd.CombinedOutput()
	if err != nil {
		return traces.TempoInstance{}, fmt.Errorf("resolve Tempo ClusterIP: %w: %s", err, strings.TrimSpace(string(out)))
	}
	ip := strings.TrimSpace(string(out))
	if net.ParseIP(ip) == nil {
		return traces.TempoInstance{}, fmt.Errorf("invalid Tempo ClusterIP %q", ip)
	}
	r.instance = traces.TempoInstance{
		Endpoint:   "http://" + net.JoinHostPort(ip, "3200"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	return r.instance, nil
}

func (r *kubernetesTempoRealization) Existing(context.Context) (traces.TempoInstance, error) {
	if r.instance.HTTPClient == nil {
		return traces.TempoInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesTempoRealization) VerifyTrace(ctx context.Context, traceID string) error {
	instance, err := r.Existing(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(instance.Endpoint, "/")+"/api/traces/"+traceID, nil)
		resp, err := instance.HTTPClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Tempo did not return verification trace %s", traceID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (r *kubernetesTempoRealization) Destroy(ctx context.Context) error {
	r.instance = traces.TempoInstance{}
	base := dnsLabel(r.application)
	cmd := exec.CommandContext(ctx, r.provider.KubectlPath(),
		"delete",
		"deployment/"+base+"-tempo",
		"service/"+base+"-tempo",
		"configmap/"+base+"-tempo-config",
		"-n", r.namespace,
		"--ignore-not-found=true",
		"--wait=true",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("destroy Tempo proof resources: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func TestKubernetesCoreTempoLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_TEMPO_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_TEMPO_TEST=1")
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

	const appName = "bh-core-tempo-proof"
	const environment = "dev"
	app := application.WithOTLPTelemetry(application.New(appName, environment, false, false, false), "traces")
	realization := &kubernetesTempoRealization{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	driver := traces.NewDriverWithRealization(realization, app)
	resource := capability.Resource{
		Application: appName,
		Kind:        capability.Traces,
		Name:        "default",
		Provider:    capability.ProviderTempo,
	}

	if err := driver.Preflight(ctx, resource, capability.Binding{}); err != nil {
		t.Fatalf("Tempo Core preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})
	if err := driver.Provision(ctx, resource, capability.Binding{}); err != nil {
		t.Fatalf("Tempo Core provision: %v", err)
	}

	base := dnsLabel(appName)
	ipCmd := exec.CommandContext(ctx, provider.KubectlPath(), "get", "service", base+"-tempo", "-n", namespace, "-o", "jsonpath={.spec.clusterIP}")
	ipOut, err := ipCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve Tempo OTLP endpoint: %v: %s", err, strings.TrimSpace(string(ipOut)))
	}
	otlpEndpoint := "http://" + net.JoinHostPort(strings.TrimSpace(string(ipOut)), "4318") + "/v1/traces"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, otlpEndpoint, bytes.NewReader(telemetry.VerificationTracePayload(app)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("push verification trace to Tempo OTLP receiver: %v", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("push verification trace to Tempo OTLP receiver: HTTP %d", resp.StatusCode)
	}

	if err := driver.Verify(ctx, resource, capability.Binding{}); err != nil {
		t.Fatalf("Tempo Core verify: %v", err)
	}
}
