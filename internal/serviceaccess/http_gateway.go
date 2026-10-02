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

	"golang.org/x/crypto/bcrypt"
)

const GatewayImage = "docker.io/library/caddy:2.11.4-alpine"

type HTTPGatewayFiles struct {
	Dir       string
	Caddyfile string
	Material  TLSMaterial
	AuthToken string
}

type NativeTLSFiles struct {
	Dir      string
	Material TLSMaterial
}

func EnsureNativeTLS(ctx context.Context, issuer Issuer, policy Policy, providerDir string, serverNames ...string) (NativeTLSFiles, error) {
	if issuer == nil {
		return NativeTLSFiles{}, errors.New("native TLS issuer is required")
	}
	dir := filepath.Join(providerDir, "service-access")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return NativeTLSFiles{}, fmt.Errorf("create native service access state: %w", err)
	}
	material, err := EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), serverNames...)
	if err != nil {
		return NativeTLSFiles{}, err
	}
	projected, err := projectGatewayMaterial(dir, material)
	if err != nil {
		return NativeTLSFiles{}, err
	}
	return NativeTLSFiles{Dir: dir, Material: projected}, nil
}

type HTTPGatewaySpec struct {
	ServiceName               string
	Upstream                  string
	Upstreams                 []string
	UpstreamTrustFile         string
	UpstreamServerName        string
	UpstreamClientCertificate string
	UpstreamClientKey         string
	PublishedPortEnv          string
	ContainerPort             int
	Networks                  []string
	NetworkAliases            []string
	CertificateNames          []string
	RequireClient             bool
	DenyPaths                 []string
	BasicAuthUsername         string
	BasicAuthPassword         string
	HealthURI                 string
	HealthStatus              int
}

func EnsureHTTPGateway(ctx context.Context, issuer Issuer, policy Policy, providerDir string, spec HTTPGatewaySpec) (HTTPGatewayFiles, error) {
	if strings.TrimSpace(spec.ServiceName) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway name is required")
	}
	upstreams, err := normalizedGatewayUpstreams(spec.Upstream, spec.Upstreams)
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	spec.Upstream = upstreams[0]
	spec.Upstreams = upstreams
	if spec.ContainerPort == 0 {
		spec.ContainerPort = 8443
	}
	if spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway container port is invalid")
	}
	if (strings.TrimSpace(spec.UpstreamClientCertificate) == "") != (strings.TrimSpace(spec.UpstreamClientKey) == "") {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway upstream mTLS requires both client certificate and key")
	}
	upstreamMaterialDir := ""
	for _, candidate := range []string{spec.UpstreamTrustFile, spec.UpstreamClientCertificate, spec.UpstreamClientKey} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		dir := filepath.Dir(candidate)
		if upstreamMaterialDir == "" {
			upstreamMaterialDir = dir
			continue
		}
		if dir != upstreamMaterialDir {
			return HTTPGatewayFiles{}, errors.New("HTTP service gateway upstream TLS material must share one projection directory")
		}
	}
	dir := filepath.Join(providerDir, "service-access")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return HTTPGatewayFiles{}, fmt.Errorf("create service access state: %w", err)
	}
	certificateNames := append([]string{spec.ServiceName, "127.0.0.1"}, spec.CertificateNames...)
	material, err := EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), certificateNames...)
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	gatewayMaterial, err := projectGatewayMaterial(dir, material)
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return HTTPGatewayFiles{}, fmt.Errorf("create service access runtime config: %w", err)
	}
	if err := os.Chmod(configDir, 0o755); err != nil {
		return HTTPGatewayFiles{}, fmt.Errorf("set service access runtime config permissions: %w", err)
	}
	files := HTTPGatewayFiles{
		Dir:       dir,
		Caddyfile: filepath.Join(configDir, "Caddyfile"),
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
	basicAuthUsername := strings.TrimSpace(spec.BasicAuthUsername)
	basicAuthHash := ""
	if basicAuthUsername != "" || spec.BasicAuthPassword != "" {
		if basicAuthUsername == "" || spec.BasicAuthPassword == "" {
			return HTTPGatewayFiles{}, errors.New("HTTP service gateway basic auth requires username and password")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(spec.BasicAuthPassword), bcrypt.DefaultCost)
		if err != nil {
			return HTTPGatewayFiles{}, fmt.Errorf("hash HTTP service gateway basic auth password: %w", err)
		}
		basicAuthHash = string(hash)
	}
	config := caddyfileWithUpstreamsTLSHealthClient(
		spec.Upstreams,
		spec.UpstreamTrustFile,
		spec.UpstreamServerName,
		spec.UpstreamClientCertificate,
		spec.UpstreamClientKey,
		spec.ContainerPort,
		authentication,
		basicAuthUsername,
		basicAuthHash,
		spec.HealthURI,
		spec.HealthStatus,
		spec.DenyPaths...,
	)
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
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return TLSMaterial{}, fmt.Errorf("create service access runtime projection: %w", err)
	}
	if err := os.Chmod(runtimeDir, 0o755); err != nil {
		return TLSMaterial{}, fmt.Errorf("set service access runtime projection permissions: %w", err)
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
	if strings.TrimSpace(material.ClientCertificate) != "" || strings.TrimSpace(material.ClientKey) != "" {
		if strings.TrimSpace(material.ClientCertificate) == "" || strings.TrimSpace(material.ClientKey) == "" {
			return TLSMaterial{}, errors.New("project service client identity requires both certificate and key")
		}
		clientCert, err := project(material.ClientCertificate, "client-cert.pem")
		if err != nil {
			return TLSMaterial{}, fmt.Errorf("project service client certificate: %w", err)
		}
		clientKey, err := project(material.ClientKey, "client-key.pem")
		if err != nil {
			return TLSMaterial{}, fmt.Errorf("project service client key: %w", err)
		}
		material.ClientCertificate = clientCert
		material.ClientKey = clientKey
	}
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
	if spec.ContainerPort < 1024 {
		b.WriteString("    cap_add: [\"NET_BIND_SERVICE\"]\n")
	}
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	b.WriteString("      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777\n")
	b.WriteString("      - /config:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("      - /data:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
	b.WriteString("    command:\n")
	if files.AuthToken != "" {
		b.WriteString("      - export BASEHARBOR_ACCESS_TOKEN=\"$(cat /run/secrets/baseharbor-access-token)\"; cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --watch --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	} else {
		b.WriteString("      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --watch --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	}
	if strings.TrimSpace(spec.PublishedPortEnv) != "" {
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"127.0.0.1:$"+"{%s}:%d\"\n", spec.PublishedPortEnv, spec.ContainerPort)
	}
	b.WriteString("    volumes:\n")
	// Runtime config and active certificates are directory-mounted. The config
	// directory contains only the public Caddyfile and is traversable by the
	// unprivileged gateway UID; private service-access state remains owner-only.
	// Managed files are replaced atomically, so directory mounts make replacement
	// inodes visible to the running Caddy process and --watch can reload in place.
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(filepath.Dir(files.Caddyfile)+":/etc/caddy:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(filepath.Dir(files.Material.ServerCertificate)+":/certs:ro"))
	if files.AuthToken != "" {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.AuthToken+":/run/secrets/baseharbor-access-token:ro"))
	}
	upstreamMaterialDir := ""
	for _, candidate := range []string{spec.UpstreamTrustFile, spec.UpstreamClientCertificate, spec.UpstreamClientKey} {
		if strings.TrimSpace(candidate) != "" {
			upstreamMaterialDir = filepath.Dir(candidate)
			break
		}
	}
	if upstreamMaterialDir != "" {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(upstreamMaterialDir+":/upstream:ro"))
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
			var aliases []string
			if i == 0 && aliasable {
				aliases = append(aliases, alias)
			}
			if i == 0 {
				for _, candidate := range spec.NetworkAliases {
					candidate = strings.TrimSpace(candidate)
					if candidate != "" && !containsGatewayAlias(aliases, candidate) {
						aliases = append(aliases, candidate)
					}
				}
			}
			if len(aliases) == 0 {
				fmt.Fprintf(&b, "      %s: {}\n", network)
				continue
			}
			fmt.Fprintf(&b, "      %s:\n", network)
			b.WriteString("        aliases:\n")
			for _, candidate := range aliases {
				fmt.Fprintf(&b, "          - %s\n", strconv.Quote(candidate))
			}
		}
	}
	return b.String()
}

func normalizedGatewayUpstreams(single string, many []string) ([]string, error) {
	values := append([]string(nil), many...)
	if strings.TrimSpace(single) != "" {
		values = append([]string{single}, values...)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://") {
			return nil, fmt.Errorf("HTTP service gateway upstream %q must use http:// or https://", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, errors.New("HTTP service gateway upstream is required")
	}
	return out, nil
}

func containsGatewayAlias(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func NewHTTPClient(material TLSMaterial, requireClient bool) (*http.Client, error) {
	return newHTTPClient(material, requireClient, "")
}

func NewHTTPClientWithBasicAuth(material TLSMaterial, username, password string) (*http.Client, error) {
	client, err := newHTTPClient(material, false, "")
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("service access basic auth requires username and password")
	}
	client.Transport = basicAuthTransport{base: client.Transport, username: username, password: password}
	return client, nil
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
	if requireClient {
		if material.ClientCertificate == "" || material.ClientKey == "" {
			return nil, errors.New("service access client certificate/key are required")
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

type basicAuthTransport struct {
	base     http.RoundTripper
	username string
	password string
}

func (t basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.SetBasicAuth(t.username, t.password)
	return t.base.RoundTrip(clone)
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

func caddyfile(upstream string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash string, denyPaths ...string) string {
	return caddyfileWithUpstreamTLS(upstream, "", "", port, authentication, basicAuthUsername, basicAuthHash, denyPaths...)
}

func caddyfileWithUpstreamTLS(upstream, upstreamTrustFile, upstreamServerName string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash string, denyPaths ...string) string {
	return caddyfileWithUpstreamsTLS([]string{upstream}, upstreamTrustFile, upstreamServerName, port, authentication, basicAuthUsername, basicAuthHash, denyPaths...)
}

func caddyfileWithUpstreamsTLS(upstreams []string, upstreamTrustFile, upstreamServerName string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash string, denyPaths ...string) string {
	return caddyfileWithUpstreamsTLSStatus(upstreams, upstreamTrustFile, upstreamServerName, port, authentication, basicAuthUsername, basicAuthHash, 0, denyPaths...)
}

func caddyfileWithUpstreamsTLSStatus(upstreams []string, upstreamTrustFile, upstreamServerName string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash string, healthStatus int, denyPaths ...string) string {
	return caddyfileWithUpstreamsTLSHealth(upstreams, upstreamTrustFile, upstreamServerName, port, authentication, basicAuthUsername, basicAuthHash, "", healthStatus, denyPaths...)
}

func caddyfileWithUpstreamsTLSHealth(upstreams []string, upstreamTrustFile, upstreamServerName string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash, healthURI string, healthStatus int, denyPaths ...string) string {
	return caddyfileWithUpstreamsTLSHealthClient(upstreams, upstreamTrustFile, upstreamServerName, "", "", port, authentication, basicAuthUsername, basicAuthHash, healthURI, healthStatus, denyPaths...)
}

func caddyfileWithUpstreamsTLSHealthClient(upstreams []string, upstreamTrustFile, upstreamServerName, upstreamClientCertificate, upstreamClientKey string, port int, authentication AuthenticationMode, basicAuthUsername, basicAuthHash, healthURI string, healthStatus int, denyPaths ...string) string {
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
	var denyBlock strings.Builder
	for i, path := range denyPaths {
		path = strings.TrimSpace(path)
		if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n{}") {
			continue
		}
		fmt.Fprintf(&denyBlock, "  @baseharbor_deny_%d path %s*\n  respond @baseharbor_deny_%d 404\n", i, path, i)
	}
	if authentication == AuthenticationToken {
		authBlock = `  @unauthorized not header Authorization "Bearer {$BASEHARBOR_ACCESS_TOKEN}"
  respond @unauthorized 401
`
	}
	if basicAuthUsername != "" {
		authBlock += fmt.Sprintf("  basic_auth {\n    %s %s\n  }\n", basicAuthUsername, basicAuthHash)
	}
	normalized, err := normalizedGatewayUpstreams("", upstreams)
	if err != nil {
		normalized = []string{"http://127.0.0.1:1"}
	}
	proxyTargets := strings.Join(normalized, " ")
	healthURI = strings.TrimSpace(healthURI)
	activeHealth := ""
	if healthURI != "" && strings.HasPrefix(healthURI, "/") && !strings.ContainsAny(healthURI, "\r\n{}") {
		activeHealth = "    health_uri " + healthURI + "\n"
		if healthStatus >= 100 && healthStatus <= 599 {
			activeHealth += fmt.Sprintf("    health_status %d\n", healthStatus)
		}
		activeHealth += "    health_interval 5s\n    health_timeout 2s\n"
	}
	proxy := "  reverse_proxy " + proxyTargets + " {\n    lb_policy round_robin\n    lb_try_duration 5s\n    lb_try_interval 250ms\n" + activeHealth + "    fail_duration 30s\n    max_fails 2\n  }\n"
	allHTTPS := true
	for _, upstream := range normalized {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(upstream)), "https://") {
			allHTTPS = false
			break
		}
	}
	if allHTTPS && strings.TrimSpace(upstreamTrustFile) != "" {
		serverName := strings.TrimSpace(upstreamServerName)
		proxy = "  reverse_proxy " + proxyTargets + " {\n    lb_policy round_robin\n    lb_try_duration 5s\n    lb_try_interval 250ms\n" + activeHealth + "    fail_duration 30s\n    max_fails 2\n    transport http {\n      tls\n      tls_trust_pool file /upstream/" + filepath.Base(upstreamTrustFile) + "\n"
		if serverName != "" {
			proxy += "      tls_server_name " + serverName + "\n"
		}
		if strings.TrimSpace(upstreamClientCertificate) != "" || strings.TrimSpace(upstreamClientKey) != "" {
			if strings.TrimSpace(upstreamClientCertificate) != "" && strings.TrimSpace(upstreamClientKey) != "" {
				proxy += "      tls_client_auth /upstream/" + filepath.Base(upstreamClientCertificate) + " /upstream/" + filepath.Base(upstreamClientKey) + "\n"
			}
		}
		proxy += "    }\n  }\n"
	}
	return fmt.Sprintf(`{
  auto_https disable_redirects
}

:%d {
  tls /certs/server.pem /certs/server-key.pem%s
%s%s%s}
`, port, tlsBlock, denyBlock.String(), authBlock, proxy)
}
