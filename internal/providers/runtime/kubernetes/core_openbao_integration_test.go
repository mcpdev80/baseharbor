package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type kubernetesOpenBaoCommandExecutor struct {
	provider    Provider
	application string
	environment string
	namespace   string
	service     string
}

func (e kubernetesOpenBaoCommandExecutor) Exec(ctx context.Context, args ...string) (string, error) {
	return e.provider.Exec(ctx, e.application, e.environment, e.namespace, e.service, args...)
}

func (e kubernetesOpenBaoCommandExecutor) ExecInput(ctx context.Context, input []byte, args ...string) (string, error) {
	selector := ownershipSelector(e.application, e.environment) +
		",baseharbor.io/workload-service=" + workloadServiceLabel(e.service)

	podCmd := exec.CommandContext(
		ctx,
		e.provider.KubectlPath(),
		"get", "pods",
		"-n", e.namespace,
		"-l", selector,
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	podOutput, err := podCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve Kubernetes OpenBao pod: %w: %s", err, strings.TrimSpace(string(podOutput)))
	}
	pod := strings.TrimSpace(string(podOutput))
	if pod == "" {
		return "", fmt.Errorf("no Kubernetes pod found for OpenBao")
	}

	execArgs := []string{"exec", "-i", "-n", e.namespace, pod, "--"}
	execArgs = append(execArgs, args...)
	cmd := exec.CommandContext(ctx, e.provider.KubectlPath(), execArgs...)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("exec in Kubernetes OpenBao pod: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func TestKubernetesCoreOpenBaoPostgresStorageAndPKI(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST=1")
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

	const appName = "bh-core-openbao-proof"
	const environment = "dev"
	const password = "baseharbor-openbao-proof-only"
	base := dnsLabel(appName)
	postgresService := base + "-postgres"

	openbaoConfig := fmt.Sprintf(`ui = true
disable_mlock = true

storage "postgresql" {
  connection_url = "postgres://openbao:%s@%s:5432/openbao?sslmode=disable"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true
}

api_addr = "http://127.0.0.1:8200"
`, password, postgresService)

	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-openbao-config
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
data:
  openbao.hcl: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-postgres
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: postgres
  annotations:
    baseharbor.io/workload-service-name: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: postgres
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: postgres
    spec:
      containers:
        - name: postgres
          image: docker.io/library/postgres:18-alpine
          env:
            - name: POSTGRES_DB
              value: openbao
            - name: POSTGRES_USER
              value: openbao
            - name: POSTGRES_PASSWORD
              value: %q
          readinessProbe:
            exec:
              command:
                - sh
                - -ec
                - PGPASSWORD="$POSTGRES_PASSWORD" pg_isready -h 127.0.0.1 -U openbao -d openbao
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql
      volumes:
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %s
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
    baseharbor.io/workload-service: postgres
  ports:
    - port: 5432
      targetPort: 5432
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-openbao
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: openbao
  annotations:
    baseharbor.io/workload-service-name: openbao
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: openbao
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: openbao
    spec:
      containers:
        - name: openbao
          image: docker.io/openbao/openbao:2.7.0
          args: ["server", "-config=/run/baseharbor/openbao/openbao.hcl"]
          env:
            - name: SKIP_CHOWN
              value: "1"
            - name: BAO_ADDR
              value: http://127.0.0.1:8200
          ports:
            - containerPort: 8200
          readinessProbe:
            exec:
              command:
                - sh
                - -ec
                - 'if bao status >/dev/null 2>&1; then exit 0; else code=$?; [ "$code" -eq 2 ]; fi'
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: config
              mountPath: /run/baseharbor/openbao
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: %s-openbao-config
`,
		base, namespace, base, dnsLabel(environment), indentYAMLBlock(openbaoConfig, 4),
		base, namespace, base, dnsLabel(environment),
		base, dnsLabel(environment),
		base, dnsLabel(environment),
		password,
		postgresService, namespace, base, dnsLabel(environment),
		base, dnsLabel(environment),
		base, namespace, base, dnsLabel(environment),
		base, dnsLabel(environment),
		base, dnsLabel(environment),
		base,
	)

	apply := exec.CommandContext(ctx, provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply OpenBao proof: %v: %s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := provider.Destroy(cleanupCtx, appName, environment, namespace); err != nil {
			t.Errorf("cleanup OpenBao proof: %v", err)
		}
	})

	for _, deployment := range []string{base + "-postgres", base + "-openbao"} {
		rollout := exec.CommandContext(
			ctx,
			provider.KubectlPath(),
			"rollout", "status",
			"deployment/"+deployment,
			"-n", namespace,
			"--timeout=180s",
		)
		if output, err := rollout.CombinedOutput(); err != nil {
			diagnostics := kubernetesServiceDiagnostics(ctx, provider, namespace, appName, environment, strings.TrimPrefix(deployment, base+"-"))
			t.Fatalf("wait for %s: %v: %s\n%s", deployment, err, strings.TrimSpace(string(output)), diagnostics)
		}
	}

	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{
		Compose: filepath.Join(stateRoot, "compose.yaml"),
		Env:     filepath.Join(stateRoot, "runtime.env"),
		Project: "runtime-neutral-proof",
	}
	recoveryPath := filepath.Join(filepath.Dir(stateRoot), "openbao-recovery.json")

	command := kubernetesOpenBaoCommandExecutor{
		provider: provider, application: appName, environment: environment, namespace: namespace, service: "openbao",
	}
	executor := openbao.ExecutorFromCommand(command)

	state, err := openbao.Inspect(ctx, executor, files)
	if err != nil {
		t.Fatal(err)
	}
	if state.Initialized {
		t.Fatal("fresh OpenBao proof instance is already initialized")
	}

	if err := openbao.Bootstrap(ctx, executor, files, recoveryPath); err != nil {
		t.Fatalf("bootstrap OpenBao through runtime-neutral Core seam: %v", err)
	}
	if err := openbao.CheckManager(ctx, executor, files); err != nil {
		t.Fatalf("verify OpenBao manager AppRole: %v", err)
	}

	state, err = openbao.Inspect(ctx, executor, files)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || state.Sealed {
		t.Fatalf("OpenBao state after bootstrap = %#v", state)
	}

	ca, err := openbao.ServiceCA(ctx, executor, files)
	if err != nil {
		t.Fatalf("read managed service CA: %v", err)
	}
	if len(ca) == 0 {
		t.Fatal("managed service CA is empty")
	}

	cert, err := openbao.IssueServiceCertificate(ctx, executor, files, openbao.ServiceCertificateRequest{
		CommonName: "identity.baseharbor.local",
		DNSNames:   []string{"identity.baseharbor.local"},
		TTL:        24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("issue service certificate through OpenBao Core PKI semantics: %v", err)
	}
	if len(cert.Certificate) == 0 || len(cert.PrivateKey) == 0 || len(cert.IssuingCA) == 0 || cert.Serial == "" {
		t.Fatalf("incomplete service certificate: %#v", cert)
	}
}

func indentYAMLBlock(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimSuffix(value, "\n"), "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func kubernetesServiceDiagnostics(ctx context.Context, provider Provider, namespace, application, environment, service string) string {
	selector := ownershipSelector(application, environment) +
		",baseharbor.io/workload-service=" + workloadServiceLabel(service)
	var out strings.Builder
	for _, args := range [][]string{
		{"get", "pods", "-n", namespace, "-l", selector, "-o", "wide"},
		{"describe", "pods", "-n", namespace, "-l", selector},
		{"logs", "-n", namespace, "-l", selector, "--tail=200", "--prefix=true"},
	} {
		cmd := exec.CommandContext(ctx, provider.KubectlPath(), args...)
		data, err := cmd.CombinedOutput()
		fmt.Fprintf(&out, "\n$ kubectl %s\n%s", strings.Join(args, " "), strings.TrimSpace(string(data)))
		if err != nil {
			fmt.Fprintf(&out, "\n[diagnostic command error: %v]", err)
		}
		out.WriteString("\n")
	}
	return out.String()
}
