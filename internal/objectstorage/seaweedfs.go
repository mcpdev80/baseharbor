package objectstorage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	ProviderProject = "baseharbor-object-storage"
	ProviderService = "seaweedfs-node-1"
	ProviderNetwork = "baseharbor-object-storage"
	ProviderImage   = "docker.io/chrislusf/seaweedfs:4.47"

	sharedProviderReconcileTimeout = 120 * time.Second
	providerReadinessTimeout       = 90 * time.Second
	existingProviderProbeTimeout   = 3 * time.Second
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
	ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error)
	DestroyProject(context.Context, string, string, string) error
}

type runtimeDiagnostics interface {
	DiagnosticsProject(context.Context, string, string, string) string
}

type Driver struct {
	runtime        Runtime
	realization    SeaweedFSRealization
	app            application.Manifest
	files          application.RuntimeFiles
	issuer         serviceaccess.Issuer
	client         *http.Client
	createdBuckets map[string]struct{}
	dataDir        string
	namespace      string
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
	Project string
	Network string
}

func EnsureSharedProvider(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer) (ProviderFiles, AdminCredentials, string, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	return EnsureSharedProviderAt(ctx, runtime, issuer, dataDir, "")
}

type legacyServiceCleaner interface {
	RemoveProjectServices(context.Context, string, ...string) error
}

func EnsureSharedProviderAt(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace string) (ProviderFiles, AdminCredentials, string, error) {
	if runtime == nil {
		return ProviderFiles{}, AdminCredentials{}, "", errors.New("SeaweedFS runtime is required")
	}
	reconcileCtx, cancel := context.WithTimeout(ctx, sharedProviderReconcileTimeout)
	defer cancel()

	files, err := EnsureProviderFilesAt(reconcileCtx, issuer, dataDir, namespace)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	if cleaner, ok := runtime.(legacyServiceCleaner); ok {
		if err := cleaner.RemoveProjectServices(reconcileCtx, files.Project, "seaweedfs-access"); err != nil {
			return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("remove legacy SeaweedFS access gateway: %w", err)
		}
	}
	if err := runtime.ConfigProject(reconcileCtx, files.Project, files.Compose, files.Env); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("validate SeaweedFS provider configuration: %w", err)
	}
	if err := runtime.UpProject(reconcileCtx, files.Project, files.Compose, files.Env); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("start SeaweedFS provider: %w", err)
	}
	credentials, credentialPath, err := EnsureAdminCredentials(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	command := fmt.Sprintf(
		"s3.configure -access_key=%s -secret_key=%s -user=baseharbor-runtime-admin -actions=Admin,Read,Write,List,Tagging -apply",
		credentials.AccessKeyID,
		credentials.SecretAccessKey,
	)
	if err := reconcileRuntimeAdminIdentity(reconcileCtx, runtime, files, command); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}

	endpoint, err := providerEndpoint(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	readinessCtx, readinessCancel := context.WithTimeout(reconcileCtx, providerReadinessTimeout)
	defer readinessCancel()
	if err := waitS3(readinessCtx, client, endpoint); err != nil {
		if diagnostics, ok := runtime.(runtimeDiagnostics); ok {
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			detail := diagnostics.DiagnosticsProject(diagnosticCtx, files.Project, files.Compose, files.Env)
			diagnosticCancel()
			if strings.TrimSpace(detail) != "" {
				return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("wait for SeaweedFS S3 readiness: %w\n%s", err, detail)
			}
		}
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("wait for SeaweedFS S3 readiness: %w", err)
	}
	return files, credentials, credentialPath, nil
}

func reconcileRuntimeAdminIdentity(ctx context.Context, runtime Runtime, files ProviderFiles, command string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		input := []byte(command + "\n")
		if _, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, ProviderService, "weed", "shell"); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return fmt.Errorf("configure SeaweedFS runtime admin identity: %w", last)
		case <-ticker.C:
		}
	}
}

func ExistingReadySharedProvider(ctx context.Context) (ProviderFiles, AdminCredentials, string, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	return ExistingReadySharedProviderAt(ctx, dataDir, "")
}

func ExistingReadySharedProviderAt(ctx context.Context, dataDir, namespace string) (ProviderFiles, AdminCredentials, string, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	probeCtx, cancel := context.WithTimeout(ctx, existingProviderProbeTimeout)
	defer cancel()
	if err := waitS3(probeCtx, client, endpoint); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("existing SeaweedFS provider is not ready: %w", err)
	}
	credentialPath := filepath.Join(files.Dir, providerAdminCredentialsFile)
	credentials, err := LoadAdminCredentials(credentialPath)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	return files, credentials, credentialPath, nil
}

func NewDriver(runtime Runtime, app application.Manifest, files application.RuntimeFiles, issuer serviceaccess.Issuer) *Driver {
	return &Driver{
		runtime:        runtime,
		realization:    newRuntimeSeaweedFSRealization(runtime, issuer, "", "", app),
		app:            app,
		files:          files,
		issuer:         issuer,
		createdBuckets: map[string]struct{}{},
	}
}

func NewDriverAt(runtime Runtime, app application.Manifest, files application.RuntimeFiles, issuer serviceaccess.Issuer, dataDir, namespace string) *Driver {
	return &Driver{
		runtime:        runtime,
		realization:    newRuntimeSeaweedFSRealization(runtime, issuer, dataDir, namespace, app),
		app:            app,
		files:          files,
		issuer:         issuer,
		createdBuckets: map[string]struct{}{},
		dataDir:        filepath.Clean(dataDir),
		namespace:      strings.TrimSpace(namespace),
	}
}

func NewDriverWithRealization(realization SeaweedFSRealization, app application.Manifest, files application.RuntimeFiles) *Driver {
	return &Driver{
		realization:    realization,
		app:            app,
		files:          files,
		createdBuckets: map[string]struct{}{},
	}
}

func (d *Driver) existingProviderFiles() (ProviderFiles, error) {
	if d.dataDir != "" && d.dataDir != "." {
		return ExistingProviderFilesAt(d.dataDir, d.namespace)
	}
	return ExistingProviderFiles()
}

func (d *Driver) Descriptor() capability.Provider { return capability.SeaweedFS }

func (d *Driver) EnsureSharedProvider(ctx context.Context) (ProviderFiles, AdminCredentials, string, error) {
	dataDir := d.dataDir
	if dataDir == "" || dataDir == "." {
		var err error
		dataDir, err = bhruntime.DataDir("")
		if err != nil {
			return ProviderFiles{}, AdminCredentials{}, "", err
		}
	}
	if d.app.Services.ObjectStorageManagementUI {
		if err := RegisterManagementUIConsumerAt(dataDir, d.namespace, d.app); err != nil {
			return ProviderFiles{}, AdminCredentials{}, "", err
		}
	}
	return EnsureSharedProviderAt(ctx, d.runtime, d.issuer, dataDir, d.namespace)
}

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.ObjectStorageS3 || resource.Provider != capability.ProviderSeaweedFS {
		return fmt.Errorf("SeaweedFS provider cannot satisfy %s via %s", resource.Kind, resource.Provider)
	}
	if binding.ObjectStorageS3 == nil || strings.TrimSpace(binding.ObjectStorageS3.Bucket) == "" {
		return errors.New("S3 bucket binding is required")
	}
	if binding.Security == nil {
		return errors.New("S3 secure binding is required")
	}
	if err := binding.Security.Validate(); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	if d.realization == nil {
		return errors.New("SeaweedFS realization is required")
	}
	if _, err := d.realization.Apply(ctx); err != nil {
		return err
	}

	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)

	exists, err := d.bucketExists(ctx, physical)
	if err != nil {
		return fmt.Errorf("inspect S3 bucket %s: %w", resource.Name, err)
	}
	if !exists {
		create := fmt.Sprintf("s3.bucket.create -name=%s", physical)
		if err := d.runSeaweedShell(ctx, create); err != nil {
			return fmt.Errorf("create S3 bucket %s: %w", resource.Name, err)
		}
		if err := d.waitBucketExists(ctx, physical); err != nil {
			return fmt.Errorf("verify S3 bucket %s creation: %w", resource.Name, err)
		}
		d.createdBuckets[resource.Name] = struct{}{}
	}

	iamUser, err := currentBucketIAMUser(d.files, resource.Name, physical)
	if err != nil {
		return fmt.Errorf("resolve SeaweedFS IAM identity for %s: %w", resource.Name, err)
	}
	configure := fmt.Sprintf("s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply",
		credentials.AccessKeyID, credentials.SecretAccessKey, physical, iamUser)
	if err := d.runSeaweedShell(ctx, configure); err != nil {
		return fmt.Errorf("configure least-privilege S3 identity for %s: %w", resource.Name, err)
	}
	if err := d.waitBucketIdentityReady(ctx, physical, credentials); err != nil {
		return fmt.Errorf("activate least-privilege S3 identity for %s: %w", resource.Name, err)
	}
	return nil
}

func (d *Driver) Bind(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	if d.realization == nil {
		return errors.New("SeaweedFS realization is required")
	}
	instance, err := d.realization.Existing(ctx)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	if err := application.MaterializeObjectStorageBindingMaterial(
		d.app,
		d.files,
		resource.Name,
		physical,
		instance.Endpoint,
		instance.WorkloadEndpoint,
		instance.TrustBundle,
	); err != nil {
		return fmt.Errorf("materialize S3 application binding: %w", err)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	if d.realization == nil {
		return errors.New("SeaweedFS realization is required")
	}
	instance, err := d.realization.Existing(ctx)
	if err != nil {
		return err
	}
	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	endpoint := instance.Endpoint
	if d.client == nil {
		d.client = instance.HTTPClient
	}
	if d.client == nil {
		return errors.New("SeaweedFS realization did not provide an HTTP client")
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	var probe [18]byte
	if _, err := rand.Read(probe[:]); err != nil {
		return fmt.Errorf("generate S3 verification probe: %w", err)
	}
	key := ".baseharbor/verify-" + hex.EncodeToString(probe[:6])
	payload := []byte("baseharbor-s3-verification-" + hex.EncodeToString(probe[:]))

	status, _, err := signedS3Request(ctx, d.client, endpoint, http.MethodPut, physical, key, credentials, payload)
	if err != nil {
		return fmt.Errorf("S3 PutObject verification: %w", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("S3 PutObject verification returned HTTP %d", status)
	}
	defer signedS3Request(context.WithoutCancel(ctx), d.client, endpoint, http.MethodDelete, physical, key, credentials, nil)

	status, body, err := signedS3Request(ctx, d.client, endpoint, http.MethodGet, physical, key, credentials, nil)
	if err != nil {
		return fmt.Errorf("S3 GetObject verification: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("S3 GetObject verification returned HTTP %d", status)
	}
	if !bytes.Equal(body, payload) {
		return errors.New("S3 GetObject verification returned unexpected payload")
	}
	return nil
}

func VerifyApplicationBuckets(ctx context.Context, runtime Runtime, app application.Manifest, files application.RuntimeFiles) error {
	return VerifyApplicationBucketsAt(ctx, runtime, app, files, "", "")
}

func VerifyApplicationBucketsAt(ctx context.Context, runtime Runtime, app application.Manifest, files application.RuntimeFiles, dataDir, namespace string) error {
	driver := NewDriver(runtime, app, files, nil)
	if strings.TrimSpace(dataDir) != "" {
		driver = NewDriverAt(runtime, app, files, nil, dataDir, namespace)
	}
	for _, bucket := range application.ObjectStorageBucketNames(app) {
		resource := capability.Resource{
			Application: app.Name,
			Kind:        capability.ObjectStorageS3,
			Name:        bucket,
			Provider:    capability.ProviderSeaweedFS,
		}
		if err := driver.Verify(ctx, resource, capability.Binding{}); err != nil {
			return fmt.Errorf("verify S3 bucket %s: %w", bucket, err)
		}
	}
	return nil
}

func (d *Driver) Rollback(ctx context.Context) {
	for bucket := range d.createdBuckets {
		_ = d.DestroyBucket(ctx, bucket)
	}
}

func (d *Driver) DestroyBucket(ctx context.Context, logicalBucket string) error {
	if d.realization == nil {
		return errors.New("SeaweedFS realization is required")
	}
	if _, err := d.realization.Existing(ctx); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, logicalBucket)
	command := fmt.Sprintf("s3.bucket.delete -name=%s", physical)
	if err := d.runSeaweedShell(ctx, command); err != nil {
		return fmt.Errorf("destroy S3 bucket %s: %w", logicalBucket, err)
	}
	iamUser, err := currentBucketIAMUser(d.files, logicalBucket, physical)
	if err != nil {
		return fmt.Errorf("resolve S3 identity for %s: %w", logicalBucket, err)
	}
	revoke := fmt.Sprintf("s3.configure -user=%s -delete -apply", iamUser)
	if err := d.runSeaweedShell(ctx, revoke); err != nil {
		return fmt.Errorf("revoke S3 identity for %s: %w", logicalBucket, err)
	}
	_ = os.Remove(bucketCredentialIdentityStatePath(d.files, logicalBucket))
	return nil
}

func (d *Driver) runSeaweedShell(ctx context.Context, command string) error {
	_, err := d.runSeaweedShellOutput(ctx, command)
	return err
}

func (d *Driver) runSeaweedShellOutput(ctx context.Context, command string) (string, error) {
	if d.realization == nil {
		return "", errors.New("SeaweedFS realization is required")
	}
	return d.realization.Admin(ctx, command)
}

func (d *Driver) waitBucketExists(ctx context.Context, bucket string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		exists, err := d.bucketExists(waitCtx, bucket)
		if err == nil && exists {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = errors.New("bucket was not listed after create")
		}
		select {
		case <-waitCtx.Done():
			if lastErr == nil {
				lastErr = waitCtx.Err()
			}
			return lastErr
		case <-ticker.C:
		}
	}
}

func (d *Driver) bucketExists(ctx context.Context, bucket string) (bool, error) {
	out, err := d.runSeaweedShellOutput(ctx, "s3.bucket.list")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == bucket {
			return true, nil
		}
	}
	return false, nil
}

func PhysicalBucketName(m application.Manifest, logical string) string {
	base := "bh-" + m.Name + "-" + m.Environment + "-" + logical
	if len(base) <= 63 {
		return base
	}
	sum := sha256.Sum256([]byte(base))
	suffix := "-" + hex.EncodeToString(sum[:])[:10]
	return strings.TrimRight(base[:63-len(suffix)], "-") + suffix
}

func EnsureProviderFiles(ctx context.Context, issuer serviceaccess.Issuer) (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return EnsureProviderFilesAt(ctx, issuer, dataDir, "")
}

func EnsureProviderFilesAt(ctx context.Context, issuer serviceaccess.Issuer, dataDir, namespace string) (ProviderFiles, error) {
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "seaweedfs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ProviderFiles{}, fmt.Errorf("create SeaweedFS provider state: %w", err)
	}
	project := bhruntime.SharedProjectName(namespace)
	network := scopedProviderName(ProviderNetwork, namespace)
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Project: project, Network: network}
	values := map[string]string{}
	if data, err := os.ReadFile(files.Env); err == nil {
		values, err = parseEnv(data)
		if err != nil {
			return ProviderFiles{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	if values["BASEHARBOR_SEAWEEDFS_PORT"] == "" {
		port, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		values["BASEHARBOR_SEAWEEDFS_PORT"] = strconv.Itoa(port)
	}
	managementUI, err := managementUIRequested(dir)
	if err != nil {
		return ProviderFiles{}, err
	}
	if managementUI {
		if err := ensureManagementUIValues(values); err != nil {
			return ProviderFiles{}, err
		}
	}
	if err := writeEnv(files.Env, values); err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return ProviderFiles{}, err
	}
	accessPolicy.ServerName = "seaweedfs"
	accessSpec := s3AccessSpec()
	accessSpec.Networks = []string{"object-storage", "object-storage-internal"}
	accessFiles, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, accessPolicy, files.Dir, accessSpec)
	if err != nil {
		return ProviderFiles{}, err
	}
	rendered := providerComposeYAMLWithAccessAndNetwork(accessFiles, files.Network)
	if managementUI {
		adminPolicy, err := serviceaccess.Resolve("prod", "seaweedfs-admin", serviceaccess.AuthenticationNative)
		if err != nil {
			return ProviderFiles{}, err
		}
		adminAccess, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, adminPolicy, filepath.Join(files.Dir, "management-ui"), seaweedAdminAccessSpec())
		if err != nil {
			return ProviderFiles{}, err
		}
		rendered = providerComposeWithManagementUI(rendered, adminAccess)
	}
	if err := os.WriteFile(files.Compose, []byte(rendered), 0o600); err != nil {
		return ProviderFiles{}, fmt.Errorf("write SeaweedFS provider compose: %w", err)
	}
	if err := os.Chmod(files.Compose, 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	return ExistingProviderFilesAt(dataDir, "")
}

func ExistingProviderFilesAt(dataDir, namespace string) (ProviderFiles, error) {
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "seaweedfs")
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Project: bhruntime.SharedProjectName(namespace), Network: scopedProviderName(ProviderNetwork, namespace)}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return DestroySharedProviderAt(ctx, runtime, dataDir, "")
}

func DestroySharedProviderAt(ctx context.Context, runtime Runtime, dataDir, namespace string) error {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func scopedProviderName(base, namespace string) string {
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	if namespace == "" {
		return base
	}
	return base + "-" + namespace
}

func providerComposeYAML() string {
	access := serviceaccess.HTTPGatewayFiles{Caddyfile: "./service-access/Caddyfile", Material: serviceaccess.TLSMaterial{CA: "./service-access/runtime/ca.pem", ServerCertificate: "./service-access/runtime/server.pem", ServerKey: "./service-access/runtime/server-key.pem"}}
	return providerComposeYAMLWithAccess(access)
}

func providerComposeYAMLWithAccess(access serviceaccess.HTTPGatewayFiles) string {
	return providerComposeYAMLWithAccessAndNetwork(access, ProviderNetwork)
}

func providerComposeYAMLWithAccessAndNetwork(access serviceaccess.HTTPGatewayFiles, network string) string {
	const peers = "seaweedfs-node-1:9333,seaweedfs-node-2:9333,seaweedfs-node-3:9333"
	var b strings.Builder
	b.WriteString("services:\n")
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("seaweedfs-node-%d", i)
		volume := fmt.Sprintf("seaweedfs-data-%d", i)
		dc := fmt.Sprintf("dc%d", i)
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    image: %s\n", ProviderImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    user: \"1000:1000\"\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
		b.WriteString("    command:\n")
		b.WriteString("      - server\n")
		b.WriteString("      - -master=true\n")
		b.WriteString("      - -volume=true\n")
		b.WriteString("      - -filer=true\n")
		b.WriteString("      - -s3=true\n")
		fmt.Fprintf(&b, "      - -ip=%s\n", name)
		b.WriteString("      - -ip.bind=0.0.0.0\n")
		fmt.Fprintf(&b, "      - -dataCenter=%s\n", dc)
		fmt.Fprintf(&b, "      - -master.peers=%s\n", peers)
		b.WriteString("      - -master.defaultReplication=100\n")
		b.WriteString("      - -master.telemetry=false\n")
		b.WriteString("      - -filer.defaultReplicaPlacement=100\n")
		b.WriteString("      - -s3.port=8333\n")
		b.WriteString("      - -s3.iam=true\n")
		b.WriteString("      - -s3.iam.readOnly=false\n")
		b.WriteString("      - -s3.port.iceberg=0\n")
		b.WriteString("      - -s3.port.lance=0\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/data\n", volume)
		b.WriteString("    networks:\n      object-storage-internal: {}\n")
	}
	accessSpec := s3AccessSpec()
	b.WriteString(serviceaccess.HTTPGatewayComposeService(access, accessSpec))
	b.WriteString("\nvolumes:\n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&b, "  seaweedfs-data-%d:\n\n", i)
	}
	b.WriteString("networks:\n")
	fmt.Fprintf(&b, "  object-storage:\n    name: %s\n", strconv.Quote(network))
	fmt.Fprintf(&b, "  object-storage-internal:\n    name: %s\n    internal: true\n", strconv.Quote(network+"-internal"))
	return b.String()
}

func projectSeaweedNativeTLS(dir string, material serviceaccess.TLSMaterial) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for source, name := range map[string]string{
		material.CA:                "ca.pem",
		material.ServerCertificate: "server.pem",
		material.ServerKey:         "server-key.pem",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("SeaweedFS TLS material %s is empty", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func providerEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	values, err := parseEnv(data)
	if err != nil {
		return "", err
	}
	port := values["BASEHARBOR_SEAWEEDFS_PORT"]
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid SeaweedFS provider port")
	}
	return "https://127.0.0.1:" + port, nil
}

func s3AccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "seaweedfs-access",
		Upstreams:        []string{"http://seaweedfs-node-1:8333", "http://seaweedfs-node-2:8333", "http://seaweedfs-node-3:8333"},
		PublishedPortEnv: "BASEHARBOR_SEAWEEDFS_PORT",
		ContainerPort:    8443,
		Networks:         []string{"object-storage", "object-storage-internal"},
		NetworkAliases:   []string{"seaweedfs"},
		CertificateNames: []string{"seaweedfs"},
		RequireClient:    false,
		HealthStatus:     http.StatusForbidden,
	}
}

func s3HTTPClient(files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load S3 service access identity: %w", err)
	}
	return serviceaccess.NewHTTPClient(material, false)
}

func ServiceContainerEndpoint(files ProviderFiles) (string, error) {
	policy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return "", err
	}
	host := "seaweedfs"
	if policy.PKISource != serviceaccess.PKIManagedLocal && strings.TrimSpace(policy.ServerName) != "" {
		host = strings.TrimSpace(policy.ServerName)
	}
	return "https://" + host + ":8443", nil
}

func ServiceTrustBundle(files ProviderFiles) (string, error) {
	policy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return "", err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return "", err
	}
	return material.CA, nil
}
