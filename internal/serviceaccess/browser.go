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
	return verifyBrowserSurface(ctx, client, rawURL, nil, func(status int) bool {
		return status >= 200 && status < 300
	})
}

// VerifyBrowserRoute validates bounded same-authority redirects while preserving
// route-level reachability semantics for surfaces that intentionally answer
// with authentication challenges.
func VerifyBrowserRoute(ctx context.Context, client *http.Client, rawURL string) error {
	return verifyBrowserSurface(ctx, client, rawURL, nil, func(status int) bool {
		return status < 500
	})
}

// VerifyBrowserSurfaceWithAllowedAuthorities permits only the canonical URL
// plus explicitly listed HTTPS authorities. It is intended for products such
// as Keycloak where an admin alias may redirect to the canonical identity host.
func VerifyBrowserSurfaceWithAllowedAuthorities(ctx context.Context, client *http.Client, rawURL string, allowedURLs ...string) error {
	return verifyBrowserSurface(ctx, client, rawURL, allowedURLs, func(status int) bool {
		return status >= 200 && status < 300
	})
}

func verifyBrowserSurface(ctx context.Context, client *http.Client, rawURL string, allowedURLs []string, acceptFinal func(int) bool) error {
	if client == nil {
		return errors.New("browser surface HTTP client is required")
	}
	canonical, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || canonical.Scheme != "https" || canonical.Host == "" {
		return fmt.Errorf("browser surface URL %q must be an absolute HTTPS URL", rawURL)
	}
	allowed := map[string]struct{}{}
	canonicalAuthority, err := normalizedHTTPSAuthority(canonical)
	if err != nil {
		return err
	}
	allowed[canonicalAuthority] = struct{}{}
	for _, rawAllowed := range allowedURLs {
		value, parseErr := url.Parse(strings.TrimSpace(rawAllowed))
		if parseErr != nil || value.Scheme != "https" || value.Host == "" {
			return fmt.Errorf("allowed browser surface URL %q must be an absolute HTTPS URL", rawAllowed)
		}
		authority, authorityErr := normalizedHTTPSAuthority(value)
		if authorityErr != nil {
			return authorityErr
		}
		allowed[authority] = struct{}{}
	}

	probe := *client
	probe.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxBrowserSurfaceRedirects {
			return fmt.Errorf("browser surface exceeded %d redirects", maxBrowserSurfaceRedirects)
		}
		if !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("browser surface redirect changed scheme to %q", req.URL.Scheme)
		}
		authority, err := normalizedHTTPSAuthority(req.URL)
		if err != nil {
			return err
		}
		if _, ok := allowed[authority]; !ok {
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
	if !acceptFinal(resp.StatusCode) {
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


func normalizedHTTPSAuthority(value *url.URL) (string, error) {
	port, err := httpsAuthorityPort(value)
	if err != nil {
		return "", err
	}
	return strings.ToLower(value.Hostname()) + ":" + strconv.Itoa(port), nil
}
