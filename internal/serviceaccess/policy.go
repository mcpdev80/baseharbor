package serviceaccess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvPKISource      = "BASEHARBOR_SERVICE_PKI_SOURCE"
	EnvTLSCertFile    = "BASEHARBOR_SERVICE_TLS_CERT_FILE"
	EnvTLSKeyFile     = "BASEHARBOR_SERVICE_TLS_KEY_FILE"
	EnvTrustBundle    = "BASEHARBOR_SERVICE_TLS_TRUST_FILE"
	EnvClientCertFile = "BASEHARBOR_SERVICE_TLS_CLIENT_CERT_FILE"
	EnvClientKeyFile  = "BASEHARBOR_SERVICE_TLS_CLIENT_KEY_FILE"
	EnvIssuerRef      = "BASEHARBOR_SERVICE_PKI_ISSUER_REF"
	EnvServerName     = "BASEHARBOR_SERVICE_TLS_SERVER_NAME"
	EnvAuthentication = "BASEHARBOR_SERVICE_AUTHENTICATION"
	EnvAuthTokenFile  = "BASEHARBOR_SERVICE_AUTH_TOKEN_FILE"
)

type PKISource string

const (
	PKIManagedLocal PKISource = "managed-local"
	PKIExternal     PKISource = "external-pki"
	PKIBYOC         PKISource = "byoc"
)

type AuthenticationMode string

const (
	AuthenticationNone     AuthenticationMode = "none"
	AuthenticationNative   AuthenticationMode = "native"
	AuthenticationMTLS     AuthenticationMode = "mtls"
	AuthenticationToken    AuthenticationMode = "token"
	AuthenticationOIDC     AuthenticationMode = "oidc"
	AuthenticationOAuth2   AuthenticationMode = "oauth2"
	AuthenticationExternal AuthenticationMode = "external"
)

type Policy struct {
	Environment            string             `json:"environment"`
	Provider               string             `json:"provider"`
	TLSRequired            bool               `json:"tls_required"`
	AuthenticationRequired bool               `json:"authentication_required"`
	Authentication         AuthenticationMode `json:"authentication"`
	PKISource              PKISource          `json:"pki_source"`
	ServerName             string             `json:"server_name,omitempty"`
	ServerCertificate      string             `json:"server_certificate,omitempty"`
	ServerKey              string             `json:"server_key,omitempty"`
	TrustBundle            string             `json:"trust_bundle,omitempty"`
	ClientCertificate      string             `json:"client_certificate,omitempty"`
	ClientKey              string             `json:"client_key,omitempty"`
	IssuerReference        string             `json:"issuer_reference,omitempty"`
	AuthTokenFile          string             `json:"auth_token_file,omitempty"`
}

func Resolve(environment, provider string, authentication AuthenticationMode) (Policy, error) {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment == "" {
		return Policy{}, errors.New("service access environment is required")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return Policy{}, errors.New("service access provider is required")
	}
	switch authentication {
	case AuthenticationNone, AuthenticationNative, AuthenticationMTLS, AuthenticationToken, AuthenticationOIDC, AuthenticationOAuth2, AuthenticationExternal:
	default:
		return Policy{}, fmt.Errorf("unsupported service authentication mode %q", authentication)
	}
	if authentication != AuthenticationNative {
		if raw := strings.ToLower(strings.TrimSpace(value(provider, EnvAuthentication))); raw != "" {
			switch AuthenticationMode(raw) {
			case AuthenticationNone, AuthenticationMTLS, AuthenticationToken, AuthenticationOIDC, AuthenticationOAuth2, AuthenticationExternal:
				authentication = AuthenticationMode(raw)
			default:
				return Policy{}, fmt.Errorf("%s has unsupported authentication mode %q", envName(provider, EnvAuthentication), raw)
			}
		}
	}

	p := Policy{
		Environment:    environment,
		Provider:       provider,
		TLSRequired:    true,
		Authentication: authentication,
		PKISource:      PKIManagedLocal,
		ServerName:     "localhost",
	}
	switch environment {
	case "dev", "development":
		p.AuthenticationRequired = false
	case "test", "testing", "stage", "staging", "prod", "production":
		p.AuthenticationRequired = true
	default:
		// Custom environments are treated as managed rather than silently
		// weakening transport/authentication guarantees.
		p.AuthenticationRequired = true
	}

	if raw := strings.ToLower(strings.TrimSpace(value(provider, EnvPKISource))); raw != "" {
		switch PKISource(raw) {
		case PKIManagedLocal, PKIExternal, PKIBYOC:
			p.PKISource = PKISource(raw)
		default:
			return Policy{}, fmt.Errorf("%s must be managed-local, external-pki, or byoc", envName(provider, EnvPKISource))
		}
	}
	if raw := strings.TrimSpace(value(provider, EnvServerName)); raw != "" {
		if strings.ContainsAny(raw, "\r\n/") {
			return Policy{}, fmt.Errorf("%s contains an invalid server name", envName(provider, EnvServerName))
		}
		p.ServerName = raw
	}

	p.ServerCertificate = strings.TrimSpace(value(provider, EnvTLSCertFile))
	p.ServerKey = strings.TrimSpace(value(provider, EnvTLSKeyFile))
	p.TrustBundle = strings.TrimSpace(value(provider, EnvTrustBundle))
	p.ClientCertificate = strings.TrimSpace(value(provider, EnvClientCertFile))
	p.ClientKey = strings.TrimSpace(value(provider, EnvClientKeyFile))
	p.IssuerReference = strings.TrimSpace(value(provider, EnvIssuerRef))
	p.AuthTokenFile = strings.TrimSpace(value(provider, EnvAuthTokenFile))

	if p.AuthenticationRequired && p.Authentication == AuthenticationNone {
		return Policy{}, errors.New("managed service access requires an authentication mechanism")
	}
	if p.Authentication == AuthenticationToken {
		if p.AuthTokenFile == "" {
			return Policy{}, fmt.Errorf("%s is required for token authentication", envName(provider, EnvAuthTokenFile))
		}
		if err := validateFile("authentication token", p.AuthTokenFile, true); err != nil {
			return Policy{}, err
		}
	}

	if p.PKISource == PKIManagedLocal {
		if p.ServerCertificate != "" || p.ServerKey != "" || p.TrustBundle != "" || p.ClientCertificate != "" || p.ClientKey != "" || p.IssuerReference != "" {
			return Policy{}, errors.New("managed-local PKI cannot be combined with external certificate/trust inputs")
		}
		return p, nil
	}

	if p.PKISource == PKIExternal && p.IssuerReference != "" {
		if p.ServerCertificate != "" || p.ServerKey != "" || p.TrustBundle != "" || p.ClientCertificate != "" || p.ClientKey != "" {
			return Policy{}, errors.New("issuer-backed external-pki cannot be combined with static certificate/trust files")
		}
		if strings.ContainsAny(p.IssuerReference, "\r\n") {
			return Policy{}, errors.New("external PKI issuer reference contains invalid control characters")
		}
		return p, nil
	}
	if p.PKISource == PKIBYOC && p.IssuerReference != "" {
		return Policy{}, errors.New("BYOC certificate material is operator-owned and cannot declare an issuer adapter")
	}

	if p.ServerCertificate == "" || p.ServerKey == "" || p.TrustBundle == "" {
		return Policy{}, errors.New("static external PKI/BYOC requires server certificate, server key, and trust bundle")
	}
	for label, path := range map[string]string{
		"server certificate": p.ServerCertificate,
		"server key":         p.ServerKey,
		"trust bundle":       p.TrustBundle,
	} {
		if err := validateFile(label, path, strings.Contains(label, "key")); err != nil {
			return Policy{}, err
		}
	}

	if p.AuthenticationRequired && p.Authentication == AuthenticationMTLS {
		if p.ClientCertificate == "" || p.ClientKey == "" {
			return Policy{}, errors.New("managed test/prod mTLS requires external client certificate and key")
		}
		if err := validateFile("client certificate", p.ClientCertificate, false); err != nil {
			return Policy{}, err
		}
		if err := validateFile("client key", p.ClientKey, true); err != nil {
			return Policy{}, err
		}
	}
	if p.IssuerReference != "" && strings.ContainsAny(p.IssuerReference, "\r\n") {
		return Policy{}, errors.New("external PKI issuer reference contains invalid control characters")
	}
	return p, nil
}

func value(provider, base string) string {
	if specific := strings.TrimSpace(os.Getenv(envName(provider, base))); specific != "" {
		return specific
	}
	return os.Getenv(base)
}

func envName(provider, base string) string {
	suffix := strings.TrimPrefix(base, "BASEHARBOR_SERVICE_")
	var b strings.Builder
	b.WriteString("BASEHARBOR_")
	for _, r := range strings.ToUpper(provider) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteByte('_')
	b.WriteString(suffix)
	return b.String()
}

func validateFile(label, path string, secret bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect external %s %s: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("external %s %s must be a regular non-symlink file", label, path)
	}
	if secret && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("external %s %s is writable by group or others (%o)", label, path, info.Mode().Perm())
	}
	absolute, err := filepath.Abs(path)
	if err != nil || strings.TrimSpace(absolute) == "" {
		return fmt.Errorf("resolve external %s %s", label, path)
	}
	return nil
}
