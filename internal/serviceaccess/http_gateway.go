package serviceaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const GatewayImage = "docker.io/library/caddy:2.11.4-alpine"

type HTTPGatewayFiles struct {
	Dir       string
	Caddyfile string
	Material  TLSMaterial
	AuthToken string
}

type HTTPGatewaySpec struct {
	ServiceName      string
	Upstream         string
	PublishedPortEnv string
	ContainerPort    int
	Networks         []string
	RequireClient    bool
}

func EnsureHTTPGateway(ctx context.Context, issuer Issuer, policy Policy, providerDir string, spec HTTPGatewaySpec) (HTTPGatewayFiles, error) {
	if strings.TrimSpace(spec.ServiceName) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway name is required")
	}
	if strings.TrimSpace(spec.Upstream) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway upstream is required")
	}
	if spec.ContainerPort == 0 {
		spec.ContainerPort = 8443
	}
	if spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway container port is invalid")
	}
	dir := filepath.Join(providerDir, "service-access")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return HTTPGatewayFiles{}, fmt.Errorf("create service access state: %w", err)
	}
	material, err := EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), spec.ServiceName, "127.0.0.1")
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	gatewayMaterial, err := projectGatewayMaterial(dir, material)
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	files := HTTPGatewayFiles{
		Dir:       dir,
		Caddyfile: filepath.Join(dir, "Caddyfile"),
		Material:  gatewayMaterial,
	}
	authentication, err := reconcileGatewayAuthentication(dir, policy)
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	if authentication == AuthenticationMTLS && !spec.RequireClient {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway does not support the selected mTLS authentication path")
	}
	switch authentication {
	case AuthenticationNone, AuthenticationNative, AuthenticationMTLS, AuthenticationToken:
	case AuthenticationOIDC, AuthenticationOAuth2, AuthenticationExternal:
		return HTTPGatewayFiles{}, fmt.Errorf("HTTP service gateway authentication %q requires an external authentication adapter", authentication)
	default:
		return HTTPGatewayFiles{}, fmt.Errorf("unsupported HTTP service gateway authentication %q", authentication)
	}
	if authentication == AuthenticationToken {
		token, err := projectGatewayAuthToken(dir, policy.AuthTokenFile)
		if err != nil {
			return HTTPGatewayFiles{}, err
		}
		files.AuthToken = token
	}
	config := caddyfile(spec.Upstream, spec.ContainerPort, authentication)
	if err := writeAtomic(files.Caddyfile, []byte(config), 0o644); err != nil {
		return HTTPGatewayFiles{}, err
	}
	return files, nil
}

type gatewayState struct {
	Version                int                `json:"version"`
	AuthenticationRequired bool               `json:"authentication_required"`
	Authentication         AuthenticationMode `json:"authentication,omitempty"`
}

func reconcileGatewayAuthentication(dir string, policy Policy) (AuthenticationMode, error) {
	path := filepath.Join(dir, "state.json")
	requested := policy.Authentication
	if !policy.AuthenticationRequired {
		requested = AuthenticationNone
	}
	state := gatewayState{Version: 2, AuthenticationRequired: policy.AuthenticationRequired, Authentication: requested}
	if data, err := os.ReadFile(path); err == nil {
		var previous gatewayState
		if err := json.Unmarshal(data, &previous); err != nil {
			return "", fmt.Errorf("decode service access state: %w", err)
		}
		switch previous.Version {
		case 1:
			if previous.AuthenticationRequired {
				previous.Authentication = AuthenticationMTLS
			} else {
				previous.Authentication = AuthenticationNone
			}
		case 2:
		default:
			return "", fmt.Errorf("unsupported service access state version %d", previous.Version)
		}
		// A shared provider that has ever served a managed environment must not
		// silently become anonymous merely because only development consumers
		// are currently being reconciled.
		if previous.AuthenticationRequired && !state.AuthenticationRequired {
			state.AuthenticationRequired = true
			state.Authentication = previous.Authentication
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := writeAtomic(path, data, 0o600); err != nil {
		return "", err
	}
	return state.Authentication, nil
}

func projectGatewayAuthToken(dir, source string) (string, error) {
	token, err := readAuthToken(source)
	if err != nil {
		return "", err
	}
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(runtimeDir, "access-token")
	if err := writeAtomic(path, []byte(token+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func readAuthToken(path string) (string, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("read service access authentication token: %w", err)
	}
	if len(data) > 64<<10 {
		return "", errors.New("service access authentication token is too large")
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("service access authentication token is empty or contains control characters")
	}
	return token, nil
}

func projectGatewayMaterial(dir string, material TLSMaterial) (TLSMaterial, error) {
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return TLSMaterial{}, fmt.Errorf("create service access runtime projection: %w", err)
	}
	project := func(source, name string) (string, error) {
		data, err := os.ReadFile(source)
		if err != nil {
			return "", err
		}
		target := filepath.Join(runtimeDir, name)
		// The enclosing directory is owner-only. Files mounted into the
		// unprivileged gateway must be readable by its runtime UID.
		if err := writeAtomic(target, data, 0o644); err != nil {
			return "", err
		}
		return target, nil
	}
	ca, err := project(material.CA, "ca.pem")
	if err != nil {
		return TLSMaterial{}, fmt.Errorf("project service trust bundle: %w", err)
	}
	cert, err := project(material.ServerCertificate, "server.pem")
	if err != nil {
		return TLSMaterial{}, fmt.Errorf("project service server certificate: %w", err)
	}
	key, err := project(material.ServerKey, "server-key.pem")
	if err != nil {
		return TLSMaterial{}, fmt.Errorf("project service server key: %w", err)
	}
	material.CA = ca
	material.ServerCertificate = cert
	material.ServerKey = key
	return material, nil
}

func HTTPGatewayComposeService(files HTTPGatewayFiles, spec HTTPGatewaySpec) string {
	if spec.ContainerPort == 0 {
		spec.ContainerPort = 8443
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:\n", spec.ServiceName)
	fmt.Fprintf(&b, "    image: %s\n", GatewayImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65532:65532\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    cap_add: [\"NET_BIND_SERVICE\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	b.WriteString("      - /config:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("      - /data:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
	b.WriteString("    command:\n")
	if files.AuthToken != "" {
		b.WriteString("      - export BASEHARBOR_ACCESS_TOKEN=\"$(cat /run/secrets/baseharbor-access-token)\"; exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	} else {
		b.WriteString("      - exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	}
	if strings.TrimSpace(spec.PublishedPortEnv) != "" {
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"127.0.0.1:$"+"{%s}:%d\"\n", spec.PublishedPortEnv, spec.ContainerPort)
	}
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Caddyfile+":/etc/caddy/Caddyfile:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.ServerCertificate+":/certs/server.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.ServerKey+":/certs/server-key.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.CA+":/certs/ca.pem:ro"))
	if files.AuthToken != "" {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.AuthToken+":/run/secrets/baseharbor-access-token:ro"))
	}
	if len(spec.Networks) > 0 {
		b.WriteString("    networks:\n")
		alias := strings.TrimSpace(files.Material.ServerName)
		aliasable := alias != "" && !strings.EqualFold(alias, "localhost") && net.ParseIP(alias) == nil && alias != spec.ServiceName
		for i, network := range spec.Networks {
			network = strings.TrimSpace(network)
			if network == "" {
				continue
			}
			if i == 0 && aliasable {
				fmt.Fprintf(&b, "      %s:\n", network)
				b.WriteString("        aliases:\n")
				fmt.Fprintf(&b, "          - %s\n", strconv.Quote(alias))
			} else {
				fmt.Fprintf(&b, "      %s: {}\n", network)
			}
		}
	}
	return b.String()
}

func NewHTTPClient(material TLSMaterial, requireClient bool) (*http.Client, error) {
	return newHTTPClient(material, requireClient, "")
}

func NewHTTPClientForPolicy(material TLSMaterial, policy Policy) (*http.Client, error) {
	requireClient := policy.AuthenticationRequired && policy.Authentication == AuthenticationMTLS
	token := ""
	if policy.AuthenticationRequired && policy.Authentication == AuthenticationToken {
		var err error
		token, err = readAuthToken(policy.AuthTokenFile)
		if err != nil {
			return nil, err
		}
	}
	switch policy.Authentication {
	case AuthenticationNone, AuthenticationNative, AuthenticationMTLS, AuthenticationToken:
	case AuthenticationOIDC, AuthenticationOAuth2, AuthenticationExternal:
		return nil, fmt.Errorf("service access authentication %q requires an external client adapter", policy.Authentication)
	default:
		return nil, fmt.Errorf("unsupported service access authentication %q", policy.Authentication)
	}
	return newHTTPClient(material, requireClient, token)
}

func newHTTPClient(material TLSMaterial, requireClient bool, bearerToken string) (*http.Client, error) {
	caPEM, err := os.ReadFile(material.CA)
	if err != nil {
		return nil, fmt.Errorf("read service access trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("service access trust bundle contains no certificates")
	}
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: strings.TrimSpace(material.ServerName),
	}
	if requireClient && (material.ClientCertificate == "" || material.ClientKey == "") {
		return nil, errors.New("service access client certificate/key are required")
	}
	if material.ClientCertificate != "" || material.ClientKey != "" {
		if material.ClientCertificate == "" || material.ClientKey == "" {
			return nil, errors.New("service access client certificate/key must be provided together")
		}
		cert, err := tls.LoadX509KeyPair(material.ClientCertificate, material.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("load service access client identity: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}
	transport := &http.Transport{
		TLSClientConfig:     config,
		TLSHandshakeTimeout: 5 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}
	var roundTripper http.RoundTripper = transport
	if bearerToken != "" {
		roundTripper = bearerTransport{base: transport, token: bearerToken}
	}
	return &http.Client{Transport: roundTripper, Timeout: 10 * time.Second}, nil
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func LoopbackHTTPSURL(port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", errors.New("service access port is invalid")
	}
	return "https://127.0.0.1:" + strconv.Itoa(port), nil
}

func WaitHTTPS(ctx context.Context, client *http.Client, endpoint, path string) error {
	if client == nil {
		return errors.New("service access HTTP client is required")
	}
	if path == "" {
		path = "/"
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func caddyfile(upstream string, port int, authentication AuthenticationMode) string {
	var tlsBlock string
	var authBlock string
	if authentication == AuthenticationMTLS {
		tlsBlock = ` {
    client_auth {
      mode require_and_verify
      trust_pool file {
        pem_file /certs/ca.pem
      }
    }
  }`
	}
	if authentication == AuthenticationToken {
		authBlock = `  @unauthorized not header Authorization "Bearer {$BASEHARBOR_ACCESS_TOKEN}"
  respond @unauthorized 401
`
	}
	return fmt.Sprintf(`:%d {
  tls /certs/server.pem /certs/server-key.pem%s
%s  reverse_proxy %s
}
`, port, tlsBlock, authBlock, upstream)
}
