package kubernetes

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type kubernetesOTLPRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	instance    telemetry.OTLPInstance
}

func (r *kubernetesOTLPRealization) Apply(ctx context.Context) (telemetry.OTLPInstance, error) {
	if r.instance.HTTPClient != nil {
		return r.instance, nil
	}
	if err := r.provider.Destroy(ctx, r.application, r.environment, r.namespace); err != nil {
		return telemetry.OTLPInstance{}, fmt.Errorf("clean stale OTLP proof resources: %w", err)
	}

	base := dnsLabel(r.application)
	serviceHost := base + "-otel"
	caPEM, serverCertPEM, serverKeyPEM, clientCertPEM, clientKeyPEM, err := otlpProofCertificates(serviceHost)
	if err != nil {
		return telemetry.OTLPInstance{}, err
	}

	collectorConfig := `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
        tls:
          cert_file: /run/baseharbor/tls/server.pem
          key_file: /run/baseharbor/tls/server-key.pem
          client_ca_file: /run/baseharbor/tls/ca.pem
exporters:
  debug:
    verbosity: basic
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
`

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s-otel-tls
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
type: Opaque
stringData:
  ca.pem: |
%s
  server.pem: |
%s
  server-key.pem: |
%s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-otel-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  collector.yaml: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-otel
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: otel-collector
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: otel-collector
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: otel-collector
    spec:
      containers:
        - name: otel-collector
          image: %s
          args:
            - --config=/etc/otelcol-contrib/collector.yaml
          ports:
            - containerPort: 4318
          readinessProbe:
            tcpSocket:
              port: 4318
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: config
              mountPath: /etc/otelcol-contrib
              readOnly: true
            - name: tls
              mountPath: /run/baseharbor/tls
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: %s-otel-config
        - name: tls
          secret:
            secretName: %s-otel-tls
---
apiVersion: v1
kind: Service
metadata:
  name: %s-otel
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
    baseharbor.io/workload-service: otel-collector
  ports:
    - port: 4318
      targetPort: 4318
`,
		base, r.namespace, base, dnsLabel(r.environment),
		indentYAMLBlock(string(caPEM), 4),
		indentYAMLBlock(string(serverCertPEM), 4),
		indentYAMLBlock(string(serverKeyPEM), 4),
		base, r.namespace, base, dnsLabel(r.environment),
		indentYAMLBlock(collectorConfig, 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		telemetry.ProviderImage, base, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)

	apply := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		return telemetry.OTLPInstance{}, fmt.Errorf("apply OTLP proof: %w: %s", err, strings.TrimSpace(string(output)))
	}

	rollout := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"rollout", "status",
		"deployment/"+base+"-otel",
		"-n", r.namespace,
		"--timeout=180s",
	)
	if output, err := rollout.CombinedOutput(); err != nil {
		return telemetry.OTLPInstance{}, fmt.Errorf(
			"wait for OTLP collector: %w: %s\n%s",
			err,
			strings.TrimSpace(string(output)),
			kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, "otel-collector"),
		)
	}

	clusterIPCmd := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"get", "service", base+"-otel",
		"-n", r.namespace,
		"-o", "jsonpath={.spec.clusterIP}",
	)
	clusterIPOutput, err := clusterIPCmd.CombinedOutput()
	if err != nil {
		return telemetry.OTLPInstance{}, fmt.Errorf("resolve OTLP ClusterIP: %w: %s", err, strings.TrimSpace(string(clusterIPOutput)))
	}
	clusterIP := strings.TrimSpace(string(clusterIPOutput))
	if net.ParseIP(clusterIP) == nil {
		return telemetry.OTLPInstance{}, fmt.Errorf("invalid OTLP ClusterIP %q", clusterIP)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return telemetry.OTLPInstance{}, fmt.Errorf("load OTLP proof CA")
	}
	clientCertificate, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return telemetry.OTLPInstance{}, fmt.Errorf("load OTLP client certificate: %w", err)
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      pool,
		ServerName:   serviceHost,
		Certificates: []tls.Certificate{clientCertificate},
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(clusterIP, "4318"))
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	instance := telemetry.OTLPInstance{
		HostEndpoint:      "https://" + serviceHost + ":4318",
		WorkloadEndpoint:  "https://" + serviceHost + ":4318",
		TrustBundle:       caPEM,
		ClientCertificate: clientCertPEM,
		ClientKey:         clientKeyPEM,
		HTTPClient:        client,
		Network:           "kubernetes",
		ObservationID:     "opentelemetry-collector:" + base,
	}
	r.instance = instance
	return instance, nil
}

func (r *kubernetesOTLPRealization) Existing(context.Context) (telemetry.OTLPInstance, error) {
	if r.instance.HTTPClient == nil {
		return telemetry.OTLPInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesOTLPRealization) Destroy(ctx context.Context) error {
	r.instance = telemetry.OTLPInstance{}
	return r.provider.Destroy(ctx, r.application, r.environment, r.namespace)
}

func TestKubernetesCoreOTLPLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_OTLP_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_OTLP_TEST=1")
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

	const appName = "bh-core-otlp-proof"
	const environment = "dev"
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        appName,
		Environment: environment,
		Telemetry: application.TelemetryRequirements{
			OTLP: &application.OTLPRequirement{Signals: []string{"traces"}},
		},
	}

	root := t.TempDir()
	files := application.RuntimeFiles{
		Dir:            filepath.Join(root, "runtime"),
		Env:            filepath.Join(root, "runtime", "runtime.env"),
		ApplicationEnv: filepath.Join(root, "runtime", "application.env"),
		Bindings:       filepath.Join(root, "runtime", "bindings"),
		Namespace:      namespace,
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ApplicationEnv, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	realization := &kubernetesOTLPRealization{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	driver := telemetry.NewDriverWithRealization(realization, app, files)

	resource := capability.Resource{
		Application: appName,
		Kind:        capability.TelemetryOTLP,
		Name:        "default",
		Provider:    capability.ProviderOTelCollector,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "application/" + appName,
		TelemetryOTLP: &capability.OTLPTelemetryBinding{
			Direction: "export",
			Protocol:  "http/protobuf",
			Signals:   []string{"traces"},
		},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("OTLP Core preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("OTLP Core provision: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("OTLP Core bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("OTLP Core verify: %v", err)
	}

	for _, name := range []string{"ca.pem", "client-cert.pem", "client-key.pem"} {
		data, err := os.ReadFile(filepath.Join(files.Bindings, "telemetry", name))
		if err != nil {
			t.Fatalf("OTLP binding %s: %v", name, err)
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			t.Fatalf("OTLP binding %s is empty", name)
		}
	}
	envData, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envData), "OTEL_EXPORTER_OTLP_ENDPOINT=https://"+dnsLabel(appName)+"-otel:4318") {
		t.Fatalf("OTLP workload endpoint not materialized: %s", string(envData))
	}
}

func otlpProofCertificates(host string) (caPEM, serverCertPEM, serverKeyPEM, clientCertPEM, clientKeyPEM []byte, err error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	now := time.Now()
	caTemplate := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "BaseHarbor OTLP proof CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	sign := func(commonName string, dnsNames []string, usage x509.ExtKeyUsage) ([]byte, []byte, error) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		leafSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return nil, nil, err
		}
		template := x509.Certificate{
			SerialNumber: leafSerial,
			Subject:      pkix.Name{CommonName: commonName},
			NotBefore:    now.Add(-time.Minute),
			NotAfter:     now.Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			DNSNames:     dnsNames,
		}
		der, err := x509.CreateCertificate(rand.Reader, &template, &caTemplate, &key.PublicKey, caKey)
		if err != nil {
			return nil, nil, err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
	}

	serverCertPEM, serverKeyPEM, err = sign(host, []string{host}, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	clientCertPEM, clientKeyPEM, err = sign("baseharbor-otlp-proof-client", nil, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return caPEM, serverCertPEM, serverKeyPEM, clientCertPEM, clientKeyPEM, nil
}
