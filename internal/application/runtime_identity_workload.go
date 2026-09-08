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

const RuntimeIdentityContainerTokenPath = "/run/baseharbor/runtime/token"
const SecretFileBindingContainerDir = "/run/baseharbor/bindings/secrets"

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

func ConfiguredRuntimeAPIURL() (string, bool, error) {
	raw := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL"))
	if raw == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("%w: BASEHARBOR_RUNTIME_API_URL must be an absolute HTTPS URL without credentials, query, or fragment", ErrRuntimeAPIURL)
	}
	return strings.TrimRight(parsed.String(), "/"), true, nil
}

// MaterializeRuntimeIdentityWorkloadOverride adds only the protected bindings
// that selected application services actually consume. Sharing a Compose
// project does not grant access to another service's secret files or runtime
// identity token.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles, plan WorkloadBindingPlan) (string, bool, error) {
	if !m.Services.Secrets || len(workload.Services) == 0 || plan.Empty() {
		return "", false, nil
	}
	path := filepath.Join(files.Dir, "workload.runtime-identity.override.yaml")
	apiURL, apiConfigured, err := ConfiguredRuntimeAPIURL()
	if err != nil {
		return "", false, err
	}

	runtimeServices := make(map[string]struct{}, len(plan.RuntimeIdentityServices))
	for _, service := range plan.RuntimeIdentityServices {
		runtimeServices[service] = struct{}{}
	}
	if len(runtimeServices) > 0 && !apiConfigured {
		return "", false, fmt.Errorf("%w: a workload consumes BASEHARBOR_RUNTIME_TOKEN_FILE but BASEHARBOR_RUNTIME_API_URL is not configured", ErrRuntimeAPIURL)
	}

	absoluteToken := ""
	if len(runtimeServices) > 0 {
		tokenPath, err := EnsureRuntimeIdentity(m, files)
		if err != nil {
			return "", false, err
		}
		absoluteToken, err = filepath.Abs(tokenPath)
		if err != nil {
			return "", false, fmt.Errorf("resolve application runtime identity token: %w", err)
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
		}
		var mounts []string
		if _, ok := runtimeServices[service]; ok {
			mounts = append(mounts, absoluteToken+":"+RuntimeIdentityContainerTokenPath+":ro")
		}
		secretNames := append([]string(nil), plan.FileSecretsByService[service]...)
		sort.Strings(secretNames)
		for _, name := range secretNames {
			hostSecretPath := SecretFileHostPath(files, name)
			// The containing BaseHarbor state directory stays owner-only (0700),
			// which prevents other host users from reaching this file. The file
			// itself must be readable after a bind mount by an arbitrary non-root
			// container user, so the projection is read-only (0444) while Compose
			// additionally mounts it read-only into only the selected service.
			if err := os.Chmod(filepath.Dir(hostSecretPath), 0o700); err != nil {
				return "", false, fmt.Errorf("secure application secret binding directory: %w", err)
			}
			if err := os.Chmod(hostSecretPath, 0o444); err != nil {
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
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return "", false, fmt.Errorf("write application runtime binding workload override: %w", err)
	}
	return path, true, nil
}
