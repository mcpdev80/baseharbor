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
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

type kubernetesKeycloakRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	stateDir    string
	instance    identityprovider.KeycloakInstance
	portForward *exec.Cmd
}

func (r *kubernetesKeycloakRealization) Apply(ctx context.Context) (identityprovider.KeycloakInstance, error) {
	if r.instance.PublicHTTPClient != nil {
		return r.instance, nil
	}

	const host = "identity.baseharbor.local"
	const adminUser = "developer"
	const adminPassword = "baseharbor-keycloak-proof-only"
	const dbPassword = "baseharbor-keycloak-db-proof-only"
	base := dnsLabel(r.application)

	certPEM, keyPEM, err := selfSignedServerCertificate(host)
	if err != nil {
		return identityprovider.KeycloakInstance{}, err
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s-keycloak-tls
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
type: Opaque
stringData:
  tls.crt: |
%s
  tls.key: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-keycloak-db
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: keycloak-db
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: keycloak-db
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: keycloak-db
    spec:
      containers:
        - name: postgres
          image: docker.io/library/postgres:18-alpine
          env:
            - name: POSTGRES_DB
              value: keycloak
            - name: POSTGRES_USER
              value: keycloak
            - name: POSTGRES_PASSWORD
              value: %q
          readinessProbe:
            exec:
              command: ["sh", "-ec", "PGPASSWORD=\"$POSTGRES_PASSWORD\" pg_isready -h 127.0.0.1 -U keycloak -d keycloak"]
            initialDelaySeconds: 2
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: %s-keycloak-db
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
    baseharbor.io/workload-service: keycloak-db
  ports:
    - port: 5432
      targetPort: 5432
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-keycloak
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: keycloak
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: keycloak
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: keycloak
    spec:
      containers:
        - name: keycloak
          image: quay.io/keycloak/keycloak:26.7.4
          args:
            - start
            - --http-enabled=false
            - --https-port=8443
            - --https-certificate-file=/run/baseharbor/tls/tls.crt
            - --https-certificate-key-file=/run/baseharbor/tls/tls.key
            - --health-enabled=true
            - --metrics-enabled=true
            - --hostname-strict=false
          env:
            - name: KC_BOOTSTRAP_ADMIN_USERNAME
              value: %q
            - name: KC_BOOTSTRAP_ADMIN_PASSWORD
              value: %q
            - name: KC_DB
              value: postgres
            - name: KC_DB_URL
              value: jdbc:postgresql://%s-keycloak-db:5432/keycloak
            - name: KC_DB_USERNAME
              value: keycloak
            - name: KC_DB_PASSWORD
              value: %q
            - name: KC_HOSTNAME
              value: https://%s
          ports:
            - containerPort: 8443
          readinessProbe:
            tcpSocket:
              port: 8443
            initialDelaySeconds: 5
            periodSeconds: 3
          volumeMounts:
            - name: tls
              mountPath: /run/baseharbor/tls
              readOnly: true
      volumes:
        - name: tls
          secret:
            secretName: %s-keycloak-tls
---
apiVersion: v1
kind: Service
metadata:
  name: %s-keycloak
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
    baseharbor.io/workload-service: keycloak
  ports:
    - port: 8443
      targetPort: 8443
`,
		base, r.namespace, base, dnsLabel(r.environment),
		indentYAMLBlock(string(certPEM), 4), indentYAMLBlock(string(keyPEM), 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment), dbPassword,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		adminUser, adminPassword, base, dbPassword, host, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)

	apply := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		return identityprovider.KeycloakInstance{}, fmt.Errorf("apply Keycloak proof: %w: %s", err, strings.TrimSpace(string(output)))
	}

	for _, deployment := range []string{base + "-keycloak-db", base + "-keycloak"} {
		rollout := exec.CommandContext(
			ctx,
			r.provider.KubectlPath(),
			"rollout", "status",
			"deployment/"+deployment,
			"-n", r.namespace,
			"--timeout=240s",
		)
		if output, err := rollout.CombinedOutput(); err != nil {
			return identityprovider.KeycloakInstance{}, fmt.Errorf(
				"wait for %s: %w: %s\n%s",
				deployment,
				err,
				strings.TrimSpace(string(output)),
				kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, strings.TrimPrefix(deployment, base+"-")),
			)
		}
	}

	port, err := allocateLoopbackPort()
	if err != nil {
		return identityprovider.KeycloakInstance{}, err
	}
	cmd := exec.CommandContext(
		context.Background(),
		r.provider.KubectlPath(),
		"port-forward",
		"-n", r.namespace,
		"service/"+base+"-keycloak",
		fmt.Sprintf("%d:8443", port),
	)
	if err := cmd.Start(); err != nil {
		return identityprovider.KeycloakInstance{}, fmt.Errorf("start Keycloak port-forward: %w", err)
	}
	r.portForward = cmd

	if err := waitTCP("127.0.0.1", port, 20*time.Second); err != nil {
		return identityprovider.KeycloakInstance{}, err
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return identityprovider.KeycloakInstance{}, fmt.Errorf("load Keycloak proof CA")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
				ServerName: host,
			},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
			},
		},
		Timeout: 10 * time.Second,
	}

	instance := identityprovider.KeycloakInstance{
		StateDir:         r.stateDir,
		EndpointBaseURL:  "https://" + host,
		PublicBaseURL:    "https://" + host,
		WorkloadBaseURL:  "https://" + host,
		TrustBundle:      certPEM,
		PublicHTTPClient: client,
		AdminHTTPClient:  client,
		AdminUsername:    adminUser,
		AdminPassword:    adminPassword,
	}
	r.instance = instance
	return instance, nil
}

func (r *kubernetesKeycloakRealization) Existing(context.Context) (identityprovider.KeycloakInstance, error) {
	if r.instance.PublicHTTPClient == nil {
		return identityprovider.KeycloakInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesKeycloakRealization) Destroy(ctx context.Context) error {
	if r.portForward != nil && r.portForward.Process != nil {
		_ = r.portForward.Process.Kill()
		_, _ = r.portForward.Process.Wait()
		r.portForward = nil
	}
	return r.provider.Destroy(ctx, r.application, r.environment, r.namespace)
}

func TestKubernetesCoreKeycloakIdentityLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_CORE_SERVICES_TEST=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	provider, err := Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	namespace := strings.TrimSpace(os.Getenv("BASEHARBOR_KUBERNETES_TEST_NAMESPACE"))
	if namespace == "" {
		namespace = provider.Namespace()
	}

	const appName = "bh-core-identity-proof"
	const environment = "dev"
	app := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        appName,
		Environment: environment,
		Services:    application.Services{Identity: true},
		Identity:    application.IdentityRequirements{Scopes: []string{"openid", "profile", "email"}},
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
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	realization := &kubernetesKeycloakRealization{
		provider:    provider,
		application: appName,
		environment: environment,
		namespace:   namespace,
		stateDir:    filepath.Join(root, "keycloak-state"),
	}
	if err := os.MkdirAll(realization.stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	driver := identityprovider.NewKeycloakDriverWithRealization(realization, app, files)
	resource := capability.Resource{
		Application: appName,
		Kind:        capability.Identity,
		Name:        "identity",
		Provider:    capability.ProviderKeycloak,
	}
	binding := capability.Binding{
		Resource: resource,
		Workload: "application",
		Identity: &capability.IdentityBinding{
			Scopes: []string{"openid", "profile", "email"},
		},
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("Keycloak Core preflight: %v", err)
	}
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("Keycloak Core provision: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})

	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("Keycloak Core bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("Keycloak Core verify: %v", err)
	}

	for _, name := range []string{"provider", "oidc.issuer", "oidc.client-id", "ca.crt"} {
		data, err := os.ReadFile(filepath.Join(files.Bindings, application.IdentityBindingName, name))
		if err != nil {
			t.Fatalf("identity binding %s: %v", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("identity binding %s is empty", name)
		}
	}

	if err := driver.DestroyApplication(ctx); err != nil {
		t.Fatalf("Keycloak Core destroy: %v", err)
	}
}

func selfSignedServerCertificate(host string) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func allocateLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitTCP(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, fmt.Sprint(port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("wait for %s timed out", address)
}
