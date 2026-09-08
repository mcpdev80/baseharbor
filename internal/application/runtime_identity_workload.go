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

var ErrRuntimeAPIURL = errors.New("application runtime API URL is invalid")

// MaterializeRuntimeIdentityWorkloadOverride adds only the optional BaseHarbor
// runtime identity binding. It never rewrites the application Compose file.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles) (string, bool, error) {
	if !m.Services.Secrets || len(workload.Services) == 0 {
		return "", false, nil
	}
	path := filepath.Join(files.Dir, "workload.runtime-identity.override.yaml")
	if strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL")) == "" {
		if err := validateExistingRuntimeIdentityOverride(path, files); err == nil {
			return path, true, nil
		} else if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		} else {
			return "", false, err
		}
	}

	apiURL, err := runtimeAPIURL()
	if err != nil {
		return "", false, err
	}
	tokenPath, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		return "", false, err
	}
	absoluteToken, err := filepath.Abs(tokenPath)
	if err != nil {
		return "", false, fmt.Errorf("resolve application runtime identity token: %w", err)
	}
	services := append([]string(nil), workload.Services...)
	sort.Strings(services)

	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n", service)
		b.WriteString("    environment:\n")
		fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_API_URL: %s\n", strconv.Quote(apiURL))
		fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_TOKEN_FILE: %s\n", strconv.Quote(RuntimeIdentityContainerTokenPath))
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(absoluteToken+":"+RuntimeIdentityContainerTokenPath+":ro"))
	}
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return "", false, fmt.Errorf("write application runtime identity workload override: %w", err)
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
	return ownerOnlyRuntimeIdentity(RuntimeIdentityTokenPath(files))
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
