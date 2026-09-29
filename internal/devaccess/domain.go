package devaccess

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const DefaultDomain = "baha.localhost"

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func EnsureDomain(target string) (string, error) {
	if value, err := LoadDomain(target); err == nil {
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return ConfigureDomain(target, DefaultDomain)
}

func LoadDomain(target string) (string, error) {
	path, err := domainPath(target)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("development domain state must be a regular non-symlink file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return normalizeDomain(string(data))
}

func ConfigureDomain(target, value string) (string, error) {
	domain, err := normalizeDomain(value)
	if err != nil {
		return "", err
	}
	path, err := domainPath(target)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create development domain state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(domain+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return domain, nil
}

func ApplicationHost(target, app, service string) (string, error) {
	domain, err := EnsureDomain(target)
	if err != nil {
		return "", err
	}
	app = normalizeHostToken(app)
	service = normalizeHostToken(service)
	if app == "" || service == "" {
		return "", errors.New("application canonical host requires app and service")
	}
	if service == "api" {
		return app + "." + domain, nil
	}
	return app + "-" + service + "." + domain, nil
}

func SharedHost(target, service string) (string, error) {
	domain, err := EnsureDomain(target)
	if err != nil {
		return "", err
	}
	service = normalizeHostToken(service)
	if service == "" {
		return "", errors.New("shared canonical host requires service")
	}
	switch service {
	case "openbao":
		service = "secrets"
	case "prometheus":
		service = "metrics"
	case "identity":
		service = "auth"
	case "identity-admin":
		service = "auth-admin"
	}
	return service + "." + domain, nil
}

func ExposureService(routeName string, publicExposureCount int) string {
	if publicExposureCount == 1 {
		return "api"
	}
	return normalizeHostToken(routeName)
}

func CanonicalURL(host string) string {
	return "https://" + strings.TrimSpace(host)
}

func ApplicationAlias(app, service string) string {
	return developmentAlias(app, service)
}

func ProviderAlias(project, service string) string {
	return developmentAlias(project, service)
}

func developmentAlias(owner, service string) string {
	owner = normalizeHostToken(owner)
	service = normalizeHostToken(service)
	if owner == "" || service == "" {
		return ""
	}
	alias := "bh-dev-" + owner + "-" + service
	if len(alias) <= 63 {
		return alias
	}
	sum := sha256.Sum256([]byte(alias))
	suffix := fmt.Sprintf("-%x", sum[:8])
	prefixLen := 63 - len(suffix)
	prefix := strings.Trim(alias[:prefixLen], "-")
	return prefix + suffix
}

func normalizeDomain(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".")
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "/:@ \t\r\n") {
		return "", fmt.Errorf("invalid development domain %q", value)
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("development domain %q must contain at least two DNS labels", value)
	}
	for _, label := range labels {
		if len(label) > 63 || !domainLabel.MatchString(label) {
			return "", fmt.Errorf("development domain %q contains invalid DNS label %q", value, label)
		}
	}
	return value, nil
}

func normalizeHostToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func domainPath(target string) (string, error) {
	root, err := deployment.TargetStateRoot(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "developer-access", "dev", "domain"), nil
}
