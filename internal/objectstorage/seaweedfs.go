package objectstorage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	ProviderProject = "baseharbor-object-storage"
	ProviderService = "seaweedfs"
	ProviderNetwork = "baseharbor-object-storage"
	ProviderImage   = "chrislusf/seaweedfs:4.47"
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
	ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error)
	DestroyProject(context.Context, string, string, string) error
}

type Driver struct {
	runtime        Runtime
	app            application.Manifest
	files          application.RuntimeFiles
	client         *http.Client
	createdBuckets map[string]struct{}
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
}

func EnsureSharedProvider(ctx context.Context, runtime Runtime) (ProviderFiles, AdminCredentials, string, error) {
	if runtime == nil {
		return ProviderFiles{}, AdminCredentials{}, "", errors.New("SeaweedFS runtime is required")
	}
	files, err := EnsureProviderFiles()
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	if err := runtime.ConfigProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("validate SeaweedFS provider configuration: %w", err)
	}
	if err := runtime.UpProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("start SeaweedFS provider: %w", err)
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", err
	}
	if err := waitS3(ctx, &http.Client{Timeout: 10 * time.Second}, endpoint); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", fmt.Errorf("wait for SeaweedFS S3 readiness: %w", err)
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
	input := []byte(command + "\n")
	if _, err := runtime.ExecProjectInput(ctx, ProviderProject, files.Compose, files.Env, input, ProviderService, "weed", "shell"); err != nil {
		return ProviderFiles{}, AdminCredentials{}, "", errors.New("configure SeaweedFS runtime admin identity failed")
	}
	return files, credentials, credentialPath, nil
}

func NewDriver(runtime Runtime, app application.Manifest, files application.RuntimeFiles) *Driver {
	return &Driver{runtime: runtime, app: app, files: files, client: &http.Client{Timeout: 10 * time.Second}, createdBuckets: map[string]struct{}{}}
}

func (d *Driver) Descriptor() capability.Provider { return capability.SeaweedFS }

func (d *Driver) EnsureSharedProvider(ctx context.Context) (ProviderFiles, AdminCredentials, string, error) {
	return EnsureSharedProvider(ctx, d.runtime)
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
	providerFiles, _, _, err := EnsureSharedProvider(ctx, d.runtime)
	if err != nil {
		return err
	}

	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)

	exists, err := d.bucketExists(ctx, providerFiles, physical)
	if err != nil {
		return fmt.Errorf("inspect S3 bucket %s: %w", resource.Name, err)
	}
	if !exists {
		create := fmt.Sprintf("s3.bucket.create -name=%s", physical)
		if err := d.runSeaweedShell(ctx, providerFiles, create); err != nil {
			return fmt.Errorf("create S3 bucket %s: %w", resource.Name, err)
		}
		created, err := d.bucketExists(ctx, providerFiles, physical)
		if err != nil {
			return fmt.Errorf("verify S3 bucket %s creation: %w", resource.Name, err)
		}
		if !created {
			return fmt.Errorf("verify S3 bucket %s creation: bucket was not listed after create", resource.Name)
		}
		d.createdBuckets[resource.Name] = struct{}{}
	}

	configure := fmt.Sprintf("s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply",
		credentials.AccessKeyID, credentials.SecretAccessKey, physical, physical)
	if err := d.runSeaweedShell(ctx, providerFiles, configure); err != nil {
		return fmt.Errorf("configure least-privilege S3 identity for %s: %w", resource.Name, err)
	}
	return nil
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	providerFiles, err := ExistingProviderFiles()
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(providerFiles)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	if err := application.MaterializeObjectStorageBinding(d.app, d.files, resource.Name, physical, endpoint); err != nil {
		return fmt.Errorf("materialize S3 application binding: %w", err)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	providerFiles, err := ExistingProviderFiles()
	if err != nil {
		return err
	}
	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(providerFiles)
	if err != nil {
		return err
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
	driver := NewDriver(runtime, app, files)
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
	providerFiles, err := ExistingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, logicalBucket)
	command := fmt.Sprintf("s3.bucket.delete -name=%s", physical)
	if err := d.runSeaweedShell(ctx, providerFiles, command); err != nil {
		return fmt.Errorf("destroy S3 bucket %s: %w", logicalBucket, err)
	}
	revoke := fmt.Sprintf("s3.configure -user=%s -delete -apply", physical)
	if err := d.runSeaweedShell(ctx, providerFiles, revoke); err != nil {
		return fmt.Errorf("revoke S3 identity for %s: %w", logicalBucket, err)
	}
	return nil
}

func (d *Driver) runSeaweedShell(ctx context.Context, files ProviderFiles, command string) error {
	_, err := d.runSeaweedShellOutput(ctx, files, command)
	return err
}

func (d *Driver) runSeaweedShellOutput(ctx context.Context, files ProviderFiles, command string) (string, error) {
	input := []byte(command + "\n")
	out, err := d.runtime.ExecProjectInput(ctx, ProviderProject, files.Compose, files.Env, input, ProviderService, "weed", "shell")
	if err != nil {
		return "", errors.New("SeaweedFS administrative command failed")
	}
	return out, nil
}

func (d *Driver) bucketExists(ctx context.Context, files ProviderFiles, bucket string) (bool, error) {
	out, err := d.runSeaweedShellOutput(ctx, files, "s3.bucket.list")
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

func EnsureProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	dir := filepath.Join(dataDir, "providers", "seaweedfs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ProviderFiles{}, fmt.Errorf("create SeaweedFS provider state: %w", err)
	}
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
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
	if err := writeEnv(files.Env, values); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAML()), 0o600); err != nil {
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
	dir := filepath.Join(dataDir, "providers", "seaweedfs")
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	files, err := ExistingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func providerComposeYAML() string {
	return `services:
  seaweedfs:
    image: chrislusf/seaweedfs:4.47
    restart: unless-stopped
    user: "seaweed"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    command: server -s3 -iam=true -s3.iam.readOnly=false
    ports:
      - "127.0.0.1:${BASEHARBOR_SEAWEEDFS_PORT}:8333"
    volumes:
      - seaweedfs-data:/data
    networks:
      object-storage:
        aliases:
          - seaweedfs

volumes:
  seaweedfs-data:

networks:
  object-storage:
    name: baseharbor-object-storage
`
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
	return "http://127.0.0.1:" + port, nil
}

func waitS3(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func signedS3Request(ctx context.Context, client *http.Client, endpoint, method, bucket, key string, credentials application.ObjectStorageCredentials, payload []byte) (int, []byte, error) {
	path := "/"
	if bucket != "" {
		path += escapePath(bucket)
	}
	if key != "" {
		path += "/" + escapePath(key)
	}
	return signedAWSRequest(ctx, client, endpoint, "s3", method, path, "", credentials, payload)
}

func signedAWSRequest(ctx context.Context, client *http.Client, endpoint, service, method, path, contentType string, credentials application.ObjectStorageCredentials, payload []byte) (int, []byte, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(service) == "" {
		return 0, nil, errors.New("AWS SigV4 service is required")
	}
	if path == "" {
		path = "/"
	}
	base.Path = path
	base.RawQuery = ""
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])
	host := base.Host
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := method + "\n" + base.EscapedPath() + "\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	scope := dateStamp + "/us-east-1/" + service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	kDate := hmacSHA256([]byte("AWS4"+credentials.SecretAccessKey), dateStamp)
	kRegion := hmacSHA256(kDate, "us-east-1")
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	authorization := "AWS4-HMAC-SHA256 Credential=" + credentials.AccessKeyID + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req, err := http.NewRequestWithContext(ctx, method, base.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", authorization)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func hmacSHA256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func parseEnv(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, errors.New("invalid SeaweedFS provider environment")
		}
		values[key] = value
	}
	return values, nil
}

func writeEnv(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
