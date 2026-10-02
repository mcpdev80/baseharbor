package objectstorage

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// SeaweedFSInstance is the runtime-neutral result consumed by the S3 capability
// lifecycle. Runtime-native resource names remain private to the realization.
type SeaweedFSInstance struct {
	Endpoint         string
	WorkloadEndpoint string
	TrustBundle      []byte
	HTTPClient       *http.Client
}

// SeaweedFSRealization owns runtime-specific provider deployment and
// administrative execution. The capability Driver must not require Compose,
// container, Kubernetes, OpenShift or cloud resource vocabulary.
type SeaweedFSRealization interface {
	Apply(context.Context) (SeaweedFSInstance, error)
	Existing(context.Context) (SeaweedFSInstance, error)
	Admin(context.Context, string) (string, error)
	Destroy(context.Context) error
}

type runtimeSeaweedFSRealization struct {
	runtime   Runtime
	issuer    serviceaccess.Issuer
	dataDir   string
	namespace string
	app       application.Manifest
}

func newRuntimeSeaweedFSRealization(runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace string, app application.Manifest) SeaweedFSRealization {
	return &runtimeSeaweedFSRealization{
		runtime:   runtime,
		issuer:    issuer,
		dataDir:   strings.TrimSpace(dataDir),
		namespace: strings.TrimSpace(namespace),
		app:       app,
	}
}

func (r *runtimeSeaweedFSRealization) dataDirPath() (string, error) {
	if strings.TrimSpace(r.dataDir) != "" {
		return filepath.Clean(r.dataDir), nil
	}
	return bhruntime.DataDir("")
}

func (r *runtimeSeaweedFSRealization) Apply(ctx context.Context) (SeaweedFSInstance, error) {
	dataDir, err := r.dataDirPath()
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	if r.app.Services.ObjectStorageManagementUI {
		if err := RegisterManagementUIConsumerAt(dataDir, r.namespace, r.app); err != nil {
			return SeaweedFSInstance{}, err
		}
	}
	files, _, _, err := EnsureSharedProviderAt(ctx, r.runtime, r.issuer, dataDir, r.namespace)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	return seaweedFSInstanceFromFiles(files)
}

func (r *runtimeSeaweedFSRealization) Existing(ctx context.Context) (SeaweedFSInstance, error) {
	dataDir, err := r.dataDirPath()
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	files, _, _, err := ExistingReadySharedProviderAt(ctx, dataDir, r.namespace)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	return seaweedFSInstanceFromFiles(files)
}

func (r *runtimeSeaweedFSRealization) Admin(ctx context.Context, command string) (string, error) {
	dataDir, err := r.dataDirPath()
	if err != nil {
		return "", err
	}
	files, err := ExistingProviderFilesAt(dataDir, r.namespace)
	if err != nil {
		return "", err
	}
	input := []byte(strings.TrimSpace(command) + "\n")
	out, err := r.runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, ProviderService, "weed", "shell")
	if err != nil {
		return "", fmt.Errorf("SeaweedFS administrative command failed")
	}
	return out, nil
}

func (r *runtimeSeaweedFSRealization) Destroy(ctx context.Context) error {
	dataDir, err := r.dataDirPath()
	if err != nil {
		return err
	}
	return DestroySharedProviderAt(ctx, r.runtime, dataDir, r.namespace)
}

func readSeaweedFSTrustBundle(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("S3 service trust bundle path is empty")
	}
	trustBundle, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read S3 service trust bundle: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(trustBundle) {
		return nil, fmt.Errorf("S3 service trust bundle contains no valid certificates")
	}
	return trustBundle, nil
}

func seaweedFSInstanceFromFiles(files ProviderFiles) (SeaweedFSInstance, error) {
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	policy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return SeaweedFSInstance{}, fmt.Errorf("load S3 service trust material: %w", err)
	}
	trustBundle, err := readSeaweedFSTrustBundle(material.CA)
	if err != nil {
		return SeaweedFSInstance{}, err
	}
	workloadHost := "seaweedfs"
	if policy.PKISource != serviceaccess.PKIManagedLocal && strings.TrimSpace(policy.ServerName) != "" {
		workloadHost = strings.TrimSpace(policy.ServerName)
	}
	return SeaweedFSInstance{
		Endpoint:         endpoint,
		WorkloadEndpoint: "https://" + workloadHost + ":8443",
		TrustBundle:      append([]byte(nil), trustBundle...),
		HTTPClient:       client,
	}, nil
}
