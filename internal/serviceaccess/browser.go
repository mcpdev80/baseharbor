package serviceaccess

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxBrowserSurfaceRedirects = 5

// VerifyBrowserSurface validates a user-facing HTTPS URL the way a browser
// would reach it, while keeping redirect traversal bounded to the same
// effective authority. A non-default port therefore cannot silently collapse
// to HTTPS/443 during a redirect.
func VerifyBrowserSurface(ctx context.Context, client *http.Client, rawURL string) error {
	if client == nil {
		return errors.New("browser surface HTTP client is required")
	}
	canonical, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || canonical.Scheme != "https" || canonical.Host == "" {
		return fmt.Errorf("browser surface URL %q must be an absolute HTTPS URL", rawURL)
	}
	expectedHost := strings.ToLower(canonical.Hostname())
	expectedPort, err := httpsAuthorityPort(canonical)
	if err != nil {
		return err
	}

	probe := *client
	probe.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxBrowserSurfaceRedirects {
			return fmt.Errorf("browser surface exceeded %d redirects", maxBrowserSurfaceRedirects)
		}
		if !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("browser surface redirect changed scheme to %q", req.URL.Scheme)
		}
		port, err := httpsAuthorityPort(req.URL)
		if err != nil {
			return err
		}
		if !strings.EqualFold(req.URL.Hostname(), expectedHost) || port != expectedPort {
			return fmt.Errorf(
				"browser surface redirect changed canonical authority from %s to %s",
				canonical.Host,
				req.URL.Host,
			)
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, canonical.String(), nil)
	if err != nil {
		return err
	}
	resp, err := probe.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("browser surface final response is HTTP %d", resp.StatusCode)
	}
	return nil
}

func httpsAuthorityPort(value *url.URL) (int, error) {
	if value == nil {
		return 0, errors.New("browser surface URL is required")
	}
	raw := strings.TrimSpace(value.Port())
	if raw == "" {
		return 443, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("browser surface URL has invalid port %q", raw)
	}
	return port, nil
}
