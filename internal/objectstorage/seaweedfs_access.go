package objectstorage

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providertopology"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

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

func s3AccessSpec(requested ...int) serviceaccess.HTTPGatewaySpec {
	members := 1
	if len(requested) > 0 {
		members = requested[0]
	}
	var upstreams []string
	for _, member := range providertopology.Names("seaweedfs-node", members) {
		upstreams = append(upstreams, "http://"+member+":8333")
	}
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "seaweedfs-access",
		Upstreams:        upstreams,
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
