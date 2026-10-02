package externalprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type Verification struct {
	ID           string `json:"id"`
	Endpoint     string `json:"endpoint"`
	Capability   string `json:"capability,omitempty"`
	Reachability string `json:"reachability"`
	TLS          string `json:"tls"`
	Semantic     string `json:"semantic,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

func Verify(ctx context.Context, registration Registration) (Verification, error) {
	if err := registration.Validate(); err != nil {
		return Verification{}, err
	}
	if result, handled, err := verifySemantic(ctx, registration); handled {
		return result, err
	}
	u, err := url.Parse(registration.Endpoint)
	if err != nil {
		return Verification{}, err
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = defaultPort(u.Scheme)
	}
	if host == "" || port == "" {
		return Verification{}, fmt.Errorf("external provider endpoint %q requires an explicit port for scheme %q", registration.Endpoint, u.Scheme)
	}
	address := net.JoinHostPort(host, port)
	result := Verification{ID: registration.ID, Endpoint: registration.Endpoint, Reachability: "pending", TLS: "not_applicable"}

	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if usesDirectTLS(u.Scheme) {
		config, err := tlsConfigForRegistration(registration, host)
		if err != nil {
			return Verification{}, err
		}
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: config}
		conn, err := dialer.DialContext(verifyCtx, "tcp", address)
		if err != nil {
			return Verification{}, fmt.Errorf("verify external provider TLS endpoint %s: %w", address, err)
		}
		state := conn.(*tls.Conn).ConnectionState()
		_ = conn.Close()
		if len(state.PeerCertificates) == 0 {
			return Verification{}, errors.New("external provider TLS endpoint did not present a certificate")
		}
		result.Reachability = "ok"
		result.TLS = "ok"
		result.Detail = "TLS chain and hostname/SAN verification succeeded"
		return result, nil
	}

	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(verifyCtx, "tcp", address)
	if err != nil {
		return Verification{}, fmt.Errorf("verify external provider endpoint %s: %w", address, err)
	}
	_ = conn.Close()
	result.Reachability = "ok"
	result.Detail = "Endpoint is reachable; capability-specific semantic verification is required before binding is READY"
	return result, nil
}

func usesDirectTLS(scheme string) bool {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https", "tls", "rediss", "amqps", "ldaps":
		return true
	default:
		return false
	}
}

func defaultPort(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "https", "tls":
		return "443"
	case "http":
		return "80"
	case "postgres", "postgresql":
		return "5432"
	case "redis", "rediss":
		return "6379"
	case "mongodb":
		return "27017"
	case "amqp", "amqps":
		if scheme == "amqps" {
			return "5671"
		}
		return "5672"
	case "ldap", "ldaps":
		if scheme == "ldaps" {
			return "636"
		}
		return "389"
	default:
		return ""
	}
}

func tlsConfigForRegistration(reg Registration, serverName string) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	mode := reg.Trust.Mode
	if mode == "" {
		mode = TrustAuto
	}

	var discovery CertificateDiscovery
	var err error
	if strings.TrimSpace(reg.Trust.Directory) != "" {
		discovery, err = DiscoverCertificateDirectory(reg.Trust.Directory, serverName)
		if err != nil {
			return nil, err
		}
	}

	effective := mode
	if mode == TrustAuto {
		switch {
		case strings.TrimSpace(reg.Trust.ClientCertificate) != "" && strings.TrimSpace(reg.Trust.ClientKey) != "":
			effective = TrustMTLS
		case len(discovery.ClientPairs) == 1:
			effective = TrustMTLS
		case strings.TrimSpace(reg.Trust.CAReference) != "" || len(discovery.CAReferences) > 0:
			effective = TrustCustomCA
		default:
			effective = TrustSystem
		}
	}

	if effective == TrustCustomCA || effective == TrustMTLS {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		added := false
		if strings.TrimSpace(reg.Trust.CAReference) != "" {
			ok, err := appendCAReference(pool, reg.Trust.CAReference)
			if err != nil {
				return nil, err
			}
			added = added || ok
		}
		for _, path := range discovery.CAReferences {
			ok, err := appendCAReference(pool, path)
			if err != nil {
				return nil, err
			}
			added = added || ok
		}
		if effective == TrustCustomCA && !added {
			return nil, errors.New("custom-ca trust resolved no usable CA certificates")
		}
		config.RootCAs = pool
	}

	if effective == TrustMTLS {
		certPath := strings.TrimSpace(reg.Trust.ClientCertificate)
		keyPath := strings.TrimSpace(reg.Trust.ClientKey)
		if certPath == "" || keyPath == "" {
			if len(discovery.ClientPairs) != 1 {
				return nil, errors.New("mTLS trust requires exactly one resolvable client certificate/private-key pair")
			}
			certPath = discovery.ClientPairs[0].Certificate
			keyPath = discovery.ClientPairs[0].PrivateKey
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load external provider mTLS client identity: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}
	return config, nil
}

func appendCAReference(pool *x509.CertPool, path string) (bool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read external provider CA reference %q: %w", path, err)
	}
	if ok := pool.AppendCertsFromPEM(pem); !ok {
		return false, fmt.Errorf("external provider CA reference %q contains no PEM certificates", path)
	}
	return true, nil
}
