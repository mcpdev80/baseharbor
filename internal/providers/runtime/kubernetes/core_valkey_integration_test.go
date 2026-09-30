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

func TestKubernetesCoreValkeySemanticProbe(t *testing.T) {
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

	const appName = "bh-core-valkey-proof"
	const environment = "dev"
	const password = "baseharbor-k3s-cache-proof-only"

	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	m := application.New(appName, environment, false, true, false)

	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-valkey
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: valkey
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: valkey
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: valkey
    spec:
      containers:
        - name: valkey
          image: docker.io/valkey/valkey:9.1.2-alpine
          env:
            - name: VALKEY_PASSWORD
              value: %q
          command:
            - sh
            - -ec
            - |
              printf 'requirepass %%s\nappendonly yes\ndir /data\n' "$VALKEY_PASSWORD" > /tmp/valkey.conf
              exec valkey-server /tmp/valkey.conf
          ports:
            - containerPort: 6379
          readinessProbe:
            exec:
              command:
                - sh
                - -ec
                - VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli -h 127.0.0.1 -p 6379 ping | grep -q '^PONG$'
            initialDelaySeconds: 2
            periodSeconds: 2
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %s-valkey
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
    baseharbor.io/workload-service: valkey
  ports:
    - port: 6379
      targetPort: 6379
`,
		dnsLabel(appName), namespace, dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
		password,
		dnsLabel(appName), namespace, dnsLabel(appName), dnsLabel(environment),
		dnsLabel(appName), dnsLabel(environment),
	)

	apply := exec.CommandContext(ctx, provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply Valkey proof: %v: %s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if err := provider.Destroy(cleanupCtx, appName, environment, namespace); err != nil {
			t.Errorf("cleanup Valkey proof: %v", err)
		}
	})

	rollout := exec.CommandContext(ctx, provider.KubectlPath(),
		"rollout", "status",
		"deployment/"+dnsLabel(appName)+"-valkey",
		"-n", namespace,
		"--timeout=120s",
	)
	if output, err := rollout.CombinedOutput(); err != nil {
		t.Fatalf("wait Valkey proof: %v: %s", err, strings.TrimSpace(string(output)))
	}

	executor := kubernetesBackendProbeExecutor{
		provider: provider, application: appName, environment: environment, namespace: namespace,
	}
	if err := application.VerifyValkeyProvider(ctx, executor, m); err != nil {
		t.Fatalf("semantic Valkey verification through runtime-neutral Core seam: %v", err)
	}
}
