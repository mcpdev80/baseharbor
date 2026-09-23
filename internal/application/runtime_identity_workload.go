package application

import (
	"crypto/sha256"
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
	DefaultRuntimeAPIURL                   = "https://baseharbor-runtime:8443"
	RuntimeIdentityContainerTokenPath      = "/run/secrets/baseharbor-runtime-token"
	RuntimeIdentityContainerCAPath         = "/run/secrets/baseharbor-runtime-ca"
	RuntimeIdentityContainerClientCertPath = "/run/secrets/baseharbor-runtime-client-cert"
	RuntimeIdentityContainerClientKeyPath  = "/run/secrets/baseharbor-runtime-client-key"
	RuntimeIdentityContainerTLSCertPath    = "/run/secrets/baseharbor-tls-cert"
	RuntimeIdentityContainerTLSKeyPath     = "/run/secrets/baseharbor-tls-key"
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

// RequiresRuntimeBroker reports whether the application currently needs the
// per-application Application Runtime Broker. Keep broker lifecycle decisions
// centralized here so new authorized runtime capabilities can extend the
// requirement without duplicating lifecycle conditionals throughout the CLI.
func RequiresRuntimeBroker(m Manifest) bool {
	return m.Services.Secrets || len(m.Runtime.Permissions) > 0
}

func RuntimeAuthorizedServices(m Manifest) []string {
	seen := map[string]struct{}{}
	for _, permission := range m.Runtime.Permissions {
		for _, service := range permission.Services {
			if strings.TrimSpace(service) != "" {
				seen[service] = struct{}{}
			}
		}
	}
	services := make([]string, 0, len(seen))
	for service := range seen {
		services = append(services, service)
	}
	sort.Strings(services)
	return services
}

func runtimeServiceIdentitySecretName(service string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(service)))
	return fmt.Sprintf("baseharbor-runtime-token-%x", sum[:6])
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

func projectRuntimeIdentityWorkloadFile(files RuntimeFiles, source, targetName string) (string, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("inspect runtime identity binding %s: %w", targetName, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("runtime identity binding %s must be a regular file", targetName)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("runtime identity binding %s is writable by group or others (%o)", targetName, info.Mode().Perm())
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read runtime identity binding %s: %w", targetName, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("runtime identity binding %s is empty", targetName)
	}

	dir := filepath.Join(files.Bindings, "runtime-workload")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime workload binding directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("protect runtime workload binding directory: %w", err)
	}
	path := filepath.Join(dir, targetName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("write runtime workload binding %s: %w", targetName, err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("prepare runtime workload binding %s: %w", targetName, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("install runtime workload binding %s: %w", targetName, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve runtime workload binding %s: %w", targetName, err)
	}
	return absolute, nil
}

// MaterializeRuntimeIdentityWorkloadOverride adds only the protected bindings
// that selected application services actually consume. Runtime identity is
// injected through ordinary environment file paths plus Compose secret mounts;
// applications do not configure certificates, keys or broker topology.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles, plan WorkloadBindingPlan) (string, bool, error) {
	if !RequiresRuntimeBroker(m) || len(workload.Services) == 0 || plan.Empty() {
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
	serviceTokenNames := map[string]string{}
	runtimeTokenSecretFiles := map[string]string{}
	if len(runtimeServices) > 0 {
		serviceIdentities, err := EnsureRuntimeServiceIdentities(m, files)
		if err != nil {
			return "", false, err
		}
		legacyTokenPath := ""
		identityDir := RuntimeMTLSHostDir(files)
		sources := map[string]string{
			"baseharbor-runtime-ca":          filepath.Join(identityDir, "ca.pem"),
			"baseharbor-runtime-client-cert": filepath.Join(identityDir, "client-cert.pem"),
			"baseharbor-runtime-client-key": filepath.Join(identityDir, "client-key.pem"),
			"baseharbor-tls-cert":           filepath.Join(identityDir, "workload-cert.pem"),
			"baseharbor-tls-key":            filepath.Join(identityDir, "workload-key.pem"),
		}
		runtimeSecretFiles = make(map[string]string, len(sources))
		for name, hostPath := range sources {
			projected, err := projectRuntimeIdentityWorkloadFile(files, hostPath, name)
			if err != nil {
				return "", false, err
			}
			runtimeSecretFiles[name] = projected
		}
		for service := range runtimeServices {
			hostPath, explicitlyAuthorized := serviceIdentities[service]
			name := runtimeServiceIdentitySecretName(service)
			if !explicitlyAuthorized {
				if legacyTokenPath == "" {
					legacyTokenPath, err = EnsureRuntimeIdentity(m, files)
					if err != nil {
						return "", false, err
					}
					if strings.TrimSpace(legacyTokenPath) == "" {
						return "", false, errors.New("legacy runtime identity token is unavailable")
					}
				}
				hostPath = legacyTokenPath
				name = "baseharbor-runtime-token"
			}
			if _, exists := runtimeTokenSecretFiles[name]; !exists {
				projected, err := projectRuntimeIdentityWorkloadFile(files, hostPath, name)
				if err != nil {
					return "", false, err
				}
				runtimeTokenSecretFiles[name] = projected
			}
			serviceTokenNames[service] = name
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
			fmt.Fprintf(&b, "      TLS_CERT_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerTLSCertPath))
			fmt.Fprintf(&b, "      TLS_KEY_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerTLSKeyPath))
			b.WriteString("    secrets:\n")
			fmt.Fprintf(&b, "      - source: %s\n", serviceTokenNames[service])
			b.WriteString("        target: baseharbor-runtime-token\n")
			for _, name := range []string{"baseharbor-runtime-ca", "baseharbor-runtime-client-cert", "baseharbor-runtime-client-key", "baseharbor-tls-cert", "baseharbor-tls-key"} {
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
		ordered := []string{"baseharbor-runtime-ca", "baseharbor-runtime-client-cert", "baseharbor-runtime-client-key", "baseharbor-tls-cert", "baseharbor-tls-key"}
		for _, name := range ordered {
			absolute, err := filepath.Abs(runtimeSecretFiles[name])
			if err != nil {
				return "", false, fmt.Errorf("resolve runtime identity binding %s: %w", name, err)
			}
			fmt.Fprintf(&b, "  %s:\n    file: %s\n", name, strconv.Quote(absolute))
		}
		tokenNames := make([]string, 0, len(runtimeTokenSecretFiles))
		for name := range runtimeTokenSecretFiles {
			tokenNames = append(tokenNames, name)
		}
		sort.Strings(tokenNames)
		for _, name := range tokenNames {
			absolute, err := filepath.Abs(runtimeTokenSecretFiles[name])
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
