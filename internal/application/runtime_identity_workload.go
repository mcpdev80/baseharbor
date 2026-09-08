package application

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultRuntimeAPIURL                    = "https://baseharbor-secrets:8443"
	RuntimeIdentityContainerTokenPath      = "/run/secrets/baseharbor-runtime-token"
	RuntimeIdentityContainerCAPath         = "/run/secrets/baseharbor-runtime-ca"
	RuntimeIdentityContainerClientCertPath = "/run/secrets/baseharbor-runtime-client-cert"
	RuntimeIdentityContainerClientKeyPath  = "/run/secrets/baseharbor-runtime-client-key"
	SecretFileBindingContainerDir          = "/run/baseharbor/bindings/secrets"
)

var ErrRuntimeAPIURL = errors.New("application runtime API URL is invalid")

type WorkloadBindingPlan struct {
	RuntimeIdentityServices []string
	FileSecretsByService    map[string][]string
}

func (p WorkloadBindingPlan) Empty() bool {
	if len(p.RuntimeIdentityServices) > 0 {
		return false
	}
	for _, names := range p.FileSecretsByService {
		if len(names) > 0 {
			return false
		}
	}
	return true
}

func RequiredSecretUsesFileBinding(name string) bool {
	return strings.HasSuffix(name, "_FILE")
}

func HasRequiredFileSecrets(m Manifest) bool {
	for _, requirement := range m.Secrets.Required {
		if RequiredSecretUsesFileBinding(requirement.Name) {
			return true
		}
	}
	return false
}

func SecretFileHostDir(files RuntimeFiles) string {
	return filepath.Join(files.Bindings, "secrets")
}

func SecretFileHostPath(files RuntimeFiles, name string) string {
	return filepath.Join(SecretFileHostDir(files), name)
}

func SecretFileContainerPath(name string) string {
	return SecretFileBindingContainerDir + "/" + name
}

func RuntimeMTLSHostDir(files RuntimeFiles) string {
	return filepath.Join(files.Bindings, "runtime-identity")
}

func ConfiguredRuntimeAPIURL() (string, bool, error) {
	raw := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL"))
	if raw == "" {
		return DefaultRuntimeAPIURL, true, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("%w: BASEHARBOR_RUNTIME_API_URL must be an absolute HTTPS URL without credentials, query, or fragment", ErrRuntimeAPIURL)
	}
	return strings.TrimRight(parsed.String(), "/"), true, nil
}

// MaterializeRuntimeIdentityWorkloadOverride adds only the protected bindings
// that selected application services actually consume. Runtime identity is
// injected through ordinary environment file paths plus Compose secret mounts;
// applications do not configure certificates, keys or broker topology.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles, plan WorkloadBindingPlan) (string, bool, error) {
	if !m.Services.Secrets || len(workload.Services) == 0 || plan.Empty() {
		return "", false, nil
	}
	path := filepath.Join(files.Dir, "workload.runtime-identity.override.yaml")
	apiURL, _, err := ConfiguredRuntimeAPIURL()
	if err != nil {
		return "", false, err
	}

	runtimeServices := make(map[string]struct{}, len(plan.RuntimeIdentityServices))
	for _, service := range plan.RuntimeIdentityServices {
		runtimeServices[service] = struct{}{}
	}

	var runtimeSecretFiles map[string]string
	if len(runtimeServices) > 0 {
		tokenPath, err := EnsureRuntimeIdentity(m, files)
		if err != nil {
			return "", false, err
		}
		identityDir := RuntimeMTLSHostDir(files)
		runtimeSecretFiles = map[string]string{
			"baseharbor-runtime-token":       tokenPath,
			"baseharbor-runtime-ca":          filepath.Join(identityDir, "ca.pem"),
			"baseharbor-runtime-client-cert": filepath.Join(identityDir, "client-cert.pem"),
			"baseharbor-runtime-client-key":  filepath.Join(identityDir, "client-key.pem"),
		}
		for name, hostPath := range runtimeSecretFiles {
			info, err := os.Stat(hostPath)
			if err != nil || !info.Mode().IsRegular() {
				return "", false, fmt.Errorf("runtime identity binding %s is not materialized; run 'baha app apply' to reconcile the broker identity", name)
			}
		}
	}

	services := make(map[string]struct{})
	for service := range runtimeServices {
		services[service] = struct{}{}
	}
	for service, names := range plan.FileSecretsByService {
		if len(names) > 0 {
			services[service] = struct{}{}
		}
	}
	orderedServices := make([]string, 0, len(services))
	for service := range services {
		orderedServices = append(orderedServices, service)
	}
	sort.Strings(orderedServices)

	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range orderedServices {
		fmt.Fprintf(&b, "  %s:\n", service)
		if _, ok := runtimeServices[service]; ok {
			b.WriteString("    environment:\n")
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_API_URL: %s\n", strconv.Quote(apiURL))
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_TOKEN_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerTokenPath))
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_CA_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerCAPath))
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_CLIENT_CERT_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerClientCertPath))
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_CLIENT_KEY_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerClientKeyPath))
			b.WriteString("    secrets:\n")
			for _, name := range []string{"baseharbor-runtime-token", "baseharbor-runtime-ca", "baseharbor-runtime-client-cert", "baseharbor-runtime-client-key"} {
				fmt.Fprintf(&b, "      - %s\n", name)
			}
		}
		var mounts []string
		secretNames := append([]string(nil), plan.FileSecretsByService[service]...)
		sort.Strings(secretNames)
		for _, name := range secretNames {
			hostSecretPath := SecretFileHostPath(files, name)
			if err := os.Chmod(filepath.Dir(hostSecretPath), 0o700); err != nil {
				return "", false, fmt.Errorf("secure application secret binding directory: %w", err)
			}
			if err := os.Chmod(hostSecretPath, 0o644); err != nil {
				return "", false, fmt.Errorf("prepare application secret file binding %s: %w", name, err)
			}
			hostPath, err := filepath.Abs(hostSecretPath)
			if err != nil {
				return "", false, fmt.Errorf("resolve application secret file binding %s: %w", name, err)
			}
			mounts = append(mounts, hostPath+":"+SecretFileContainerPath(name)+":ro")
		}
		if len(mounts) > 0 {
			b.WriteString("    volumes:\n")
			for _, mount := range mounts {
				fmt.Fprintf(&b, "      - %s\n", strconv.Quote(mount))
			}
		}
	}
	if len(runtimeServices) > 0 {
		b.WriteString("\nsecrets:\n")
		ordered := []string{"baseharbor-runtime-token", "baseharbor-runtime-ca", "baseharbor-runtime-client-cert", "baseharbor-runtime-client-key"}
		for _, name := range ordered {
			absolute, err := filepath.Abs(runtimeSecretFiles[name])
			if err != nil {
				return "", false, fmt.Errorf("resolve runtime identity binding %s: %w", name, err)
			}
			fmt.Fprintf(&b, "  %s:\n    file: %s\n", name, strconv.Quote(absolute))
		}
	}
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return "", false, fmt.Errorf("write application runtime binding workload override: %w", err)
	}
	return path, true, nil
}
