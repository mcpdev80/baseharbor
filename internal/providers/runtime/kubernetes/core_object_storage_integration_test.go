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
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
)

type kubernetesSeaweedFSRealization struct {
	provider    Provider
	application string
	environment string
	namespace   string
	instance    objectstorage.SeaweedFSInstance
}

func (r *kubernetesSeaweedFSRealization) Apply(ctx context.Context) (objectstorage.SeaweedFSInstance, error) {
	if r.instance.HTTPClient != nil {
		return r.instance, nil
	}
	if err := r.provider.Destroy(ctx, r.application, r.environment, r.namespace); err != nil {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf("clean stale SeaweedFS proof resources: %w", err)
	}

	base := dnsLabel(r.application)
	serviceHost := base + "-seaweedfs"
	certPEM, keyPEM, err := selfSignedSeaweedCertificate(serviceHost)
	if err != nil {
		return objectstorage.SeaweedFSInstance{}, err
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s-seaweedfs-tls
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
type: Opaque
stringData:
  server.pem: |
%s
  server-key.pem: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-seaweedfs
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: baseharbor
    baseharbor.io/application: %s
    baseharbor.io/environment: %s
    baseharbor.io/workload-service: seaweedfs
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/managed-by: baseharbor
      baseharbor.io/application: %s
      baseharbor.io/environment: %s
      baseharbor.io/workload-service: seaweedfs
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: baseharbor
        baseharbor.io/application: %s
        baseharbor.io/environment: %s
        baseharbor.io/workload-service: seaweedfs
    spec:
      containers:
        - name: seaweedfs
          image: %s
          args:
            - server
            - -s3
            - -iam=true
            - -s3.iam.readOnly=false
            - -s3.port.https=8443
            - -s3.cert.file=/run/baseharbor/tls/server.pem
            - -s3.key.file=/run/baseharbor/tls/server-key.pem
          ports:
            - containerPort: 8443
          readinessProbe:
            tcpSocket:
              port: 8443
            initialDelaySeconds: 4
            periodSeconds: 2
          volumeMounts:
            - name: tls
              mountPath: /run/baseharbor/tls
              readOnly: true
            - name: data
              mountPath: /data
      volumes:
        - name: tls
          secret:
            secretName: %s-seaweedfs-tls
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %s-seaweedfs
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
    baseharbor.io/workload-service: seaweedfs
  ports:
    - port: 8443
      targetPort: 8443
`,
		base, r.namespace, base, dnsLabel(r.environment),
		indentYAMLBlock(string(certPEM), 4), indentYAMLBlock(string(keyPEM), 4),
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
		objectstorage.ProviderImage, base,
		base, r.namespace, base, dnsLabel(r.environment),
		base, dnsLabel(r.environment),
	)

	apply := exec.CommandContext(ctx, r.provider.KubectlPath(), "apply", "-f", "-")
	apply.Stdin = bytes.NewBufferString(manifest)
	if output, err := apply.CombinedOutput(); err != nil {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf("apply SeaweedFS proof: %w: %s", err, strings.TrimSpace(string(output)))
	}

	rollout := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"rollout", "status",
		"deployment/"+base+"-seaweedfs",
		"-n", r.namespace,
		"--timeout=180s",
	)
	if output, err := rollout.CombinedOutput(); err != nil {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf(
			"wait for SeaweedFS: %w: %s\n%s",
			err,
			strings.TrimSpace(string(output)),
			kubernetesServiceDiagnostics(ctx, r.provider, r.namespace, r.application, r.environment, "seaweedfs"),
		)
	}

	clusterIPCmd := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"get", "service", base+"-seaweedfs",
		"-n", r.namespace,
		"-o", "jsonpath={.spec.clusterIP}",
	)
	clusterIPOutput, err := clusterIPCmd.CombinedOutput()
	if err != nil {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf("resolve SeaweedFS ClusterIP: %w: %s", err, strings.TrimSpace(string(clusterIPOutput)))
	}
	clusterIP := strings.TrimSpace(string(clusterIPOutput))
	if net.ParseIP(clusterIP) == nil {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf("invalid SeaweedFS ClusterIP %q", clusterIP)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return objectstorage.SeaweedFSInstance{}, fmt.Errorf("load SeaweedFS proof CA")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	tlsConfig.RootCAs = pool
	tlsConfig.ServerName = serviceHost
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(clusterIP, "8443"))
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	instance := objectstorage.SeaweedFSInstance{
		Endpoint:         "https://" + serviceHost + ":8443",
		WorkloadEndpoint: "https://" + serviceHost + ":8443",
		TrustBundle:      certPEM,
		HTTPClient:       client,
	}
	r.instance = instance
	return instance, nil
}

func (r *kubernetesSeaweedFSRealization) Existing(context.Context) (objectstorage.SeaweedFSInstance, error) {
	if r.instance.HTTPClient == nil {
		return objectstorage.SeaweedFSInstance{}, os.ErrNotExist
	}
	return r.instance, nil
}

func (r *kubernetesSeaweedFSRealization) Admin(ctx context.Context, command string) (string, error) {
	base := dnsLabel(r.application)
	cmd := exec.CommandContext(
		ctx,
		r.provider.KubectlPath(),
		"exec", "-i",
		"-n", r.namespace,
		"deployment/"+base+"-seaweedfs",
		"--", "weed", "shell",
	)
	cmd.Stdin = strings.NewReader(strings.TrimSpace(command) + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SeaweedFS admin command: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (r *kubernetesSeaweedFSRealization) Destroy(ctx context.Context) error {
	r.instance = objectstorage.SeaweedFSInstance{}
	return r.provider.Destroy(ctx, r.application, r.environment, r.namespace)
}

func TestKubernetesCoreObjectStorageS3Lifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_OBJECT_STORAGE_TEST") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_OBJECT_STORAGE_TEST=1")
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

	const appName = "bh-core-object-storage-proof"
	const environment = "dev"
	const bucket = "assets"
	app := application.WithObjectStorageBuckets(application.Manifest{
		Version:     application.CurrentVersion,
		Name:        appName,
		Environment: environment,
	}, bucket)

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
	if err := os.WriteFile(files.Env, []byte(
		"S3_ASSETS_ACCESS_KEY_ID=BHOBJECTSTORAGEACCESS\n"+
			"S3_ASSETS_SECRET_ACCESS_KEY=BHOBJECTSTORAGESECRET0123456789\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ApplicationEnv, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	realization := &kubernetesSeaweedFSRealization{
		provider:    provider,
		application: appName,
		environment: environment,
		namespace:   namespace,
	}
	driver := objectstorage.NewDriverWithRealization(realization, app, files)

	resource := capability.Resource{
		Application: appName,
		Kind:        capability.ObjectStorageS3,
		Name:        bucket,
		Provider:    capability.ProviderSeaweedFS,
	}
	security := application.ObjectStorageSecureBinding(app, bucket)
	binding := capability.Binding{
		Resource: resource,
		Workload: "application",
		ObjectStorageS3: &capability.ObjectStorageS3Binding{
			Bucket: bucket,
		},
		Security: &security,
	}

	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatalf("SeaweedFS Core preflight: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = realization.Destroy(cleanupCtx)
	})

	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatalf("SeaweedFS Core provision: %v", err)
	}
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatalf("SeaweedFS Core bind: %v", err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatalf("SeaweedFS Core verify: %v", err)
	}

	for _, name := range []string{"endpoint", "bucket", "region", "access_key_id", "secret_access_key", "certificates"} {
		data, err := os.ReadFile(filepath.Join(files.Bindings, "object-storage-s3", bucket, name))
		if err != nil {
			t.Fatalf("object-storage binding %s: %v", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("object-storage binding %s is empty", name)
		}
	}

	if err := driver.DestroyBucket(ctx, bucket); err != nil {
		t.Fatalf("SeaweedFS Core destroy bucket: %v", err)
	}
}

func selfSignedSeaweedCertificate(host string) ([]byte, []byte, error) {
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
