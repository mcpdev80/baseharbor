package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type kubernetesBackendProbeExecutor struct {
	provider    Provider
	application string
	environment string
	namespace   string
}

func (e kubernetesBackendProbeExecutor) ProbeBackend(ctx context.Context, probe application.BackendProbe) (string, error) {
	switch probe.Kind {
	case application.BackendProbeSQLSelectOne:
		database := strings.TrimSpace(probe.Database)
		if database == "" {
			return "", fmt.Errorf("postgres verification database is required")
		}
		return e.provider.Exec(
			ctx,
			e.application,
			e.environment,
			e.namespace,
			"postgres",
			"sh", "-ec",
			fmt.Sprintf(`PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U baseharbor -d %q -tAc 'SELECT 1'`, database),
		)
	default:
		return "", fmt.Errorf("unsupported Kubernetes backend probe %q", probe.Kind)
	}
}

func TestKubernetesCorePostgresSemanticProbe(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	provider, err := Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	namespace := strings.TrimSpace(os.Getenv("BASEHARBOR_KUBERNETES_TEST_NAMESPACE"))
	if namespace == "" {
		namespace = provider.Namespace()
	}

	const appName = "bh-core-postgres-proof"
	const environment = "dev"
	const password = "baseharbor-k3s-proof-only"
	database := strings.ReplaceAll(appName+"_"+environment, "-", "_")

	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	m := application.New(appName, environment, true, false, false)

	manifest := fmt.Sprintf(`apiVersion: apps/v1
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
              value: %q
            - name: POSTGRES_USER
              value: baseharbor
            - name: POSTGRES_PASSWORD
              value: %q
          ports:
            - containerPort: 5432
          readinessProbe:
            exec:
              command:
                - sh
                - -ec
                - PGPASSWORD="$POSTGRES_PASSWORD" pg_isready -h 127.0.0.1 -U baseharbor -d "$POSTGRES_DB"
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
  name: %s-postgres
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
`,
		dnsLabel(appName), namespace, dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
		database, password,
		dnsLabel(appName), namespace, dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
	)

	apply := exec.CommandContext(ctx, provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply PostgreSQL proof: %v: %s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if err := provider.Destroy(cleanupCtx, appName, environment, namespace); err != nil {
			t.Errorf("cleanup PostgreSQL proof: %v", err)
		}
	})

	rollout := exec.CommandContext(
		ctx,
		provider.KubectlPath(),
		"rollout", "status",
		"deployment/"+dnsLabel(appName)+"-postgres",
		"-n", namespace,
		"--timeout=120s",
	)
	if output, err := rollout.CombinedOutput(); err != nil {
		t.Fatalf("wait PostgreSQL proof: %v: %s", err, strings.TrimSpace(string(output)))
	}

	executor := kubernetesBackendProbeExecutor{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	if err := application.VerifyPostgresProvider(ctx, executor, m); err != nil {
		t.Fatalf("semantic PostgreSQL verification through runtime-neutral Core seam: %v", err)
	}
}
