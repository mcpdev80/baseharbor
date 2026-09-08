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

// MaterializeRuntimeIdentityWorkloadOverride adds protected BaseHarbor runtime
// bindings without rewriting the application Compose file. App runtime identity
// is optional; declared *_FILE secrets still receive an owner-only read-only
// binding even when no runtime API endpoint is configured.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles) (string, bool, error) {
	if !m.Services.Secrets || len(workload.Services) == 0 {
		return "", false, nil
	}
	path := filepath.Join(files.Dir, "workload.runtime-identity.override.yaml")
	rawAPIURL := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL"))
	fileBindings := HasRequiredFileSecrets(m)
	if rawAPIURL == "" && !fileBindings {
		if err := validateExistingRuntimeIdentityOverride(path, files); err == nil {
			return path, true, nil
		} else if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		} else {
			return "", false, err
		}
	}

	apiURL := ""
	tokenPath := ""
	if rawAPIURL != "" {
		var err error
		apiURL, err = runtimeAPIURL()
		if err != nil {
			return "", false, err
		}
		tokenPath, err = EnsureRuntimeIdentity(m, files)
		if err != nil {
			return "", false, err
		}
	}

	absoluteToken := ""
	if tokenPath != "" {
		var err error
		absoluteToken, err = filepath.Abs(tokenPath)
		if err != nil {
			return "", false, fmt.Errorf("resolve application runtime identity token: %w", err)
		}
	}
	absoluteSecretDir := ""
	if fileBindings {
		secretDir := SecretFileHostDir(files)
		if err := os.MkdirAll(secretDir, 0o700); err != nil {
			return "", false, fmt.Errorf("create application secret file binding directory: %w", err)
		}
		if err := os.Chmod(secretDir, 0o700); err != nil {
			return "", false, fmt.Errorf("secure application secret file binding directory: %w", err)
		}
		var err error
		absoluteSecretDir, err = filepath.Abs(secretDir)
		if err != nil {
			return "", false, fmt.Errorf("resolve application secret file binding directory: %w", err)
		}
	}

	services := append([]string(nil), workload.Services...)
	sort.Strings(services)
	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n", service)
		if apiURL != "" {
			b.WriteString("    environment:\n")
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_API_URL: %s\n", strconv.Quote(apiURL))
			fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_TOKEN_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerTokenPath))
		}
		if absoluteToken != "" || absoluteSecretDir != "" {
			b.WriteString("    volumes:\n")
			if absoluteToken != "" {
				fmt.Fprintf(&b, "      - %s\n", strconv.Quote(absoluteToken+":"+RuntimeIdentityContainerTokenPath+":ro"))
			}
			if absoluteSecretDir != "" {
				fmt.Fprintf(&b, "      - %s\n", strconv.Quote(absoluteSecretDir+":"+SecretFileBindingContainerDir+":ro"))
			}
		}
	}
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return "", false, fmt.Errorf("write application runtime binding workload override: %w", err)
	}
	return path, true, nil
}

func validateExistingRuntimeIdentityOverride(path string, files RuntimeFiles) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("application runtime identity workload override is not owner-only")
	}
	if _, err := os.Stat(RuntimeIdentityTokenPath(files)); err == nil {
		return ownerOnlyRuntimeIdentity(RuntimeIdentityTokenPath(files))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func runtimeAPIURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL"))
	if raw == "" {
		return "", fmt.Errorf("%w: BASEHARBOR_RUNTIME_API_URL is not configured", ErrRuntimeAPIURL)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: BASEHARBOR_RUNTIME_API_URL must be an absolute HTTPS URL without credentials, query, or fragment", ErrRuntimeAPIURL)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
