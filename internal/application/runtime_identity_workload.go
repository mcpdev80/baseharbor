package application

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const RuntimeIdentityContainerTokenPath = "/run/baseharbor/runtime/token"

// MaterializeRuntimeIdentityWorkloadOverride adds only the optional BaseHarbor
// runtime identity binding. It never rewrites the application Compose file.
func MaterializeRuntimeIdentityWorkloadOverride(m Manifest, workload WorkloadFiles, files RuntimeFiles) (string, bool, error) {
	if !m.Services.Secrets || len(workload.Services) == 0 {
		return "", false, nil
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
	path := filepath.Join(files.Dir, "workload.runtime-identity.override.yaml")
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return "", false, fmt.Errorf("write application runtime identity workload override: %w", err)
	}
	return path, true, nil
}

func runtimeAPIURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_API_URL"))
	if raw == "" {
		return "", errorsNewRuntimeAPIURL("BASEHARBOR_RUNTIME_API_URL is required when a workload uses managed dynamic secrets")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errorsNewRuntimeAPIURL("BASEHARBOR_RUNTIME_API_URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func errorsNewRuntimeAPIURL(message string) error {
	return fmt.Errorf("application runtime API configuration: %s", message)
}
