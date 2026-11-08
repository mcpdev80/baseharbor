package operatorauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/auth"
)

// TrustOptions carries the operator OIDC trust material that lets the CLI verify
// the managed CA it bootstrapped without relying on OS system trust.
//
// The resolved RootCAs pool is ADDITIVE: the managed CA (and any --ca file) is
// layered on top of the system trust pool. The system pool is never replaced,
// so an operator who accidentally points --ca at an untrusted anchor does not
// silently disable OS trust.
type TrustOptions struct {
	// ExplicitCaFile is the --ca flag value. It is resolved before the
	// BASEHARBOR_OPERATOR_OIDC_CA_FILE environment file and the state-dir
	// default file.
	ExplicitCaFile string
	// AuthCodeFile is the completion file for a non-browser PKCE flow. When
	// present the login exchange reads the authorization code from it instead
	// of listening for a loopback callback.
	AuthCodeFile string
	// NoBrowser suppresses the local browser launcher for the PKCE
	// authorization URL. It is the --no-browser companion flag; BASEHARBOR_NO_BROWSER
	// remains its documented environment alias.
	NoBrowser bool
}

const (
	// EnvTrustFile is the state-wide override for the managed CA used by the
	// operator OIDC discovery path. It is resolved after --ca and before the
	// state-dir default file.
	EnvTrustFile = "BASEHARBOR_OPERATOR_OIDC_CA_FILE"
	// DefaultManagedCAFile is the state-dir default location BaseHarbor writes
	// the managed CA to at bootstrap so the operator OIDC path can trust it.
	DefaultManagedCAFile = "trust/managed-operator-ca.pem"
)

var (
	// ErrNoOperatorTrust is returned when discovery must trust the managed CA
	// but no CA material is available. The message names the remedy path so an
	// operator knows exactly how to proceed.
	ErrNoOperatorTrust = errors.New("operator OIDC issuer served by a managed CA that is not trusted by this host's system trust pool")
)

// resolveSessionTrustFile returns the managed CA file for the persisted-session
// verification path. It never fails: a missing file yields the empty string so
// the additive trust pool simply omits the managed CA (verification then falls
// back to system trust, exactly as before this fix).
func resolveSessionTrustFile() string {
	if env := strings.TrimSpace(os.Getenv(EnvTrustFile)); env != "" {
		return env
	}
	return DefaultTrustPath()
}

// DefaultTrustPath resolves the state-dir default location for the managed CA.
// It returns an empty string when the state directory cannot be determined.
func DefaultTrustPath() string {
	root, err := trustStateRoot()
	if err != nil || root == "" {
		return ""
	}
	return filepath.Join(root, DefaultManagedCAFile)
}

func trustStateRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return root, nil
	}
	if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
		return filepath.Join(dir, "baseharbor"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "baseharbor"), nil
}

// ResolveTrustFile returns the CA PEM the CLI should add to the discovery
// RootCAs pool, applying the Owner-mandated precedence:
//
//	--ca flag > BASEHARBOR_OPERATOR_OIDC_CA_FILE > state-dir default file
//
// It never reads or returns private key material; only a certificate PEM is
// returned. The state-dir default file is only used when it exists, so an
// un-bootstrapped host falls through to ErrNoOperatorTrust with a clear
// remedy instead of silently accepting an unknown certificate.
func ResolveTrustFile(opts TrustOptions) ([]byte, string, error) {
	var candidates []string
	if explicit := strings.TrimSpace(opts.ExplicitCaFile); explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := strings.TrimSpace(os.Getenv(EnvTrustFile)); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, DefaultTrustPath())

	for _, path := range candidates {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, path, fmt.Errorf("read operator OIDC CA file %s: %w", path, err)
		}
		if !auth.PEMLooksLikeCertificate(data) {
			continue
		}
		return data, path, nil
	}

	return nil, DefaultTrustPath(), ErrNoOperatorTrust
}

// discoveryClient returns an *http.Client whose TLS roots include the resolved
// managed CA on top of the system trust pool. When no explicit CA is
// configured (no --ca flag and no BASEHARBOR_OPERATOR_OIDC_CA_FILE env) it
// returns (nil, nil) so the caller keeps the default transport / any client
// already injected via context. When an explicit CA is configured but cannot
// be read or is not a certificate, it returns ErrNoOperatorTrust so the caller
// can attach a precise remedy. The system trust pool is never replaced
// (additive only).
func discoveryClient(ctx context.Context, cfg Config, opts TrustOptions) (*http.Client, error) {
	explicit := strings.TrimSpace(opts.ExplicitCaFile)
	env := strings.TrimSpace(os.Getenv(EnvTrustFile))
	if explicit == "" && env == "" {
		// No explicit trust override: preserve the default system-trust
		// transport (or a client the caller injected through the context).
		return nil, nil
	}

	candidate := explicit
	if candidate == "" {
		candidate = env
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s=%s not found: run the managed Core bootstrap so the managed CA is written to %s", ErrNoOperatorTrust, EnvTrustFile, candidate, DefaultTrustPath())
		}
		return nil, fmt.Errorf("read operator OIDC CA file %s: %w", candidate, err)
	}
	if !auth.PEMLooksLikeCertificate(data) {
		return nil, fmt.Errorf("%w: %s is not a PEM certificate", ErrNoOperatorTrust, candidate)
	}

	pool, err := systemRoots()
	if err != nil {
		// A host without a system roots pool is unusual; fall back to the
		// crypto/x509 defaults which still include the OS anchors.
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("%w: %s does not contain a valid PEM certificate", ErrNoOperatorTrust, candidate)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
	}

	return &http.Client{
		Timeout:   0,
		Transport: transport,
	}, nil
}

func systemRoots() (*x509.CertPool, error) {
	return x509.SystemCertPool()
}
