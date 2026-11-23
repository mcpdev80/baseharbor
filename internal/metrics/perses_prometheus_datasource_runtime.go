package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Perses Prometheus datasource RUNTIME PROVISIONING (0428-083 correction round).
//
// The builder in perses_prometheus_datasource.go produces the validated
// datasource descriptor. This file realizes the step the review verdict
// required: the authorized datasource must reach the RUNNING Perses, not stay a
// builder/golden artifact. Perses consumes it through its provisioning
// contract: the server starts with --config=/etc/perses/config.yaml, that
// config declares a file database plus provisioning folders, and Perses loads
// every resource file in those folders into its database at startup and
// reconciles them at the configured interval (Perses v0.55 provisioning docs;
// loader: internal/api/provisioning -> internal/cli/file, which walks the
// folders recursively and accepts .json/.yaml resources by their "kind").
//
// The provisioning resource is the Perses-native GlobalDatasource (v0.55
// schema, the same shape the pinned image's own dev/data sample ships): kind,
// metadata.name, spec.default and spec.plugin{kind: PrometheusDatasource,
// spec: proxy{kind: HTTPProxy, spec{url, allowedEndpoints}}}. The endpoint is
// the materialized Prometheus management endpoint and access stays "proxy"
// (the server-side HTTPProxy), never a browser-direct URL. No secret, token or
// credential value is ever written into these files; the authentication class
// remains the owner reference the builder already carries.
//
// The artifacts land under the application-scoped projection root
// <dataDir>/applications/<resourceProject>/providers/management-ui/perses/,
// the providers/management-ui/perses/ layout the Perses runtime service uses
// for its TLS mounts; the deploy boundary passes the compose context dir
// (the directory holding the rendered compose file, RuntimeFiles.Dir on the
// lifecycle path) as dataDir, so the compose-relative mounts
// ./providers/management-ui/perses/... resolve against what Perses starts
// with. Fail-closed: an unselected surface or any builder error
// removes the provisioning artifacts (a deselected surface never serves a
// stale datasource) and propagates the error; it never writes a partial or
// empty provisioning.

// Perses datasource provisioning identity (0428-083).
const (
	// PersesDatasourceProvisioningResourceName is the GlobalDatasource
	// metadata.name of the BaseHarbor Prometheus datasource.
	PersesDatasourceProvisioningResourceName = "baseharbor-prometheus"
	// PersesDatasourceProvisioningKind is the Perses provisioning resource
	// kind of a global datasource.
	PersesDatasourceProvisioningKind = "GlobalDatasource"
	// PersesDatasourceProxyKind is the pinned Perses native proxy plugin kind.
	PersesDatasourceProxyKind = "HTTPProxy"
	// persesDatasourceConfigFile is the Perses config file the runtime command
	// consumes (--config=/etc/perses/config.yaml).
	persesDatasourceConfigFile = "config.yaml"
	// persesDatasourceProvisioningDir is the folder name holding the resources.
	persesDatasourceProvisioningDir = "provisioning"
	// persesDatasourceProvisioningFile is the provisioned datasource resource file.
	persesDatasourceProvisioningFile = "globaldatasource-" + PersesDatasourceProvisioningResourceName + ".json"
	// persesDatasourceProvisioningIntervalSeconds is the Perses provisioning
	// reconcile interval (seconds; the v0.55 default "1h" is too slow for
	// endpoint rotation within a managed window).
	persesDatasourceProvisioningIntervalSeconds = 300
	// persesDatasourceDatabaseFolder is the Perses file-database folder inside
	// the container, on the managed persistence volume.
	persesDatasourceDatabaseFolder = "/persistence/database"
	// persesDatasourceProvisioningContainerFolder is the in-container path of
	// the provisioning folder the compose mount delivers.
	persesDatasourceProvisioningContainerFolder = "/etc/perses/provisioning"
	// PersesDatasourceContainerConfigPath is the --config path the Perses
	// runtime command already uses; this file materializes exactly it.
	PersesDatasourceContainerConfigPath = "/etc/perses/config.yaml"
	// PersesDatasourceSecretKind is the Perses provisioning resource kind of
	// a global secret (the trust-anchor carrier of the datasource proxy).
	PersesDatasourceSecretKind = "GlobalSecret"
	// PersesDatasourceTLSSecretName is the metadata.name of the provisioned
	// GlobalSecret the datasource proxy references through proxy.spec.secret
	// (F3). It carries ONLY public trust material (the runtime CA bundle),
	// never a credential value.
	PersesDatasourceTLSSecretName = "baseharbor-prometheus-tls"
	// persesDatasourceSecretFile is the trust-anchor resource file name in
	// the provisioning folder.
	persesDatasourceSecretFile = "globalsecret-" + PersesDatasourceTLSSecretName + ".json"
)

// persesPrometheusAllowedEndpoints are the Prometheus HTTP API paths the
// datasource proxy may serve. The list is the pinned native Prometheus plugin
// contract of the Perses v0.55 release (dev/data sample of the same tag):
// nothing outside it is reachable through the datasource, so the provisioning
// file never widens the Prometheus management surface.
var persesPrometheusAllowedEndpoints = []struct {
	EndpointPattern string `json:"endpointPattern"`
	Method          string `json:"method"`
}{
	{EndpointPattern: "/api/v1/labels", Method: "POST"},
	{EndpointPattern: "/api/v1/series", Method: "POST"},
	{EndpointPattern: "/api/v1/metadata", Method: "GET"},
	{EndpointPattern: "/api/v1/query", Method: "POST"},
	{EndpointPattern: "/api/v1/query_range", Method: "POST"},
	{EndpointPattern: "/api/v1/label/([a-zA-Z0-9_-]+)/values", Method: "GET"},
}

// PersesDatasourceConfig is the rendered Perses server configuration this
// projection produces. Only the keys BaseHarbor sets are represented; the
// marshalling is deterministic and golden-tested. The database section is
// required because provisioning injects resources into the (file) database.
// The datasource.proxy.allowed_networks exception is required because the
// pinned v0.55.1 SSRF guard denies loopback destinations by default and the
// Prometheus gateway is reached through the compose host-gateway mapping.
type PersesDatasourceConfig struct {
	Database struct {
		File struct {
			Folder    string `json:"folder" yaml:"folder"`
			Extension string `json:"extension" yaml:"extension"`
		} `json:"file" yaml:"file"`
	} `json:"database" yaml:"database"`
	Security struct {
		SecretFileAllowedDirectories []string `json:"secret_file_allowed_directories" yaml:"secret_file_allowed_directories"`
	} `json:"security" yaml:"security"`
	Provisioning struct {
		Folders     []string `json:"folders" yaml:"folders"`
		Interval    string   `json:"interval" yaml:"interval"`
		EnableWatch bool     `json:"enable_watch" yaml:"enable_watch"`
	} `json:"provisioning" yaml:"provisioning"`
	Datasource struct {
		Proxy struct {
			AllowedNetworks []string `json:"allowed_networks" yaml:"allowed_networks"`
		} `json:"proxy" yaml:"proxy"`
	} `json:"datasource" yaml:"datasource"`
}

// PersesDatasourceTrustBundleContainerDir is the directory the trust bundle
// mounts to, and the single entry the rendered config declares in
// security.secret_file_allowed_directories: the pinned release's secret-file
// guard (internal/api/secretfile) refuses ANY file reference — including a
// GlobalSecret tlsConfig.caFile — unless its directory is allowlisted, and
// the datasource proxy is provisioned exactly through such a reference. The
// directory is dedicated to the public CA material; no other mount may claim
// it (0428-083 F3).
const PersesDatasourceTrustBundleContainerDir = "/etc/perses/datasource-trust"

// PersesDatasourceTrustBundleContainerPath is the in-container path of the
// public trust material the datasource projection mounts (the gateway service
// CA, PEM). It is PUBLIC material — never a key or certificate private half —
// and the GlobalSecret's caFile and the serverName below are how the proxy
// validates and pins the gateway endpoint it serves. The name is reserved so
// no other file may claim it (0428-083).
const PersesDatasourceTrustBundleContainerPath = PersesDatasourceTrustBundleContainerDir + "/baseharbor-runtime-ca.pem"

// persesDatasourceTrustBundleFileName is the host-side file name of the trust
// bundle inside the datasource projection root.
const persesDatasourceTrustBundleFileName = "ca.pem"

// NewPersesDatasourceConfig renders the deterministic Perses config for the
// pinned provisioning layout (file database on the persistence volume + the
// mounted provisioning folder + the reconcile interval). The datasource-proxy
// SSRF guard (perses/perses#4518, shipped in the pinned v0.55.1) denies
// loopback and link-local destinations by DEFAULT, so the config carries the
// explicit exception list: the datasource endpoint is the Prometheus gateway
// reached through the compose host-gateway mapping, i.e. the host loopback
// (127.0.0.0/8, ::1/128) as seen from the Perses netns. The exception is the
// narrowest possible (loopback only; the guard checks the RESOLVED IP at
// connect time, so it cannot be widened by DNS), and it is only meaningful
// read-only through the gateway's mTLS access boundary.
func NewPersesDatasourceConfig() PersesDatasourceConfig {
	var cfg PersesDatasourceConfig
	cfg.Database.File.Folder = persesDatasourceDatabaseFolder
	cfg.Database.File.Extension = "json"
	cfg.Security.SecretFileAllowedDirectories = []string{PersesDatasourceTrustBundleContainerDir}
	cfg.Provisioning.Folders = []string{persesDatasourceProvisioningContainerFolder}
	cfg.Provisioning.Interval = strconv.Itoa(persesDatasourceProvisioningIntervalSeconds) + "s"
	// Watch the provisioning folders (0428-083 reload): the file loader
	// re-applies folder changes on its own instead of a signal-based reload —
	// the pinned release has no SIGHUP handler (its listener takes
	// SIGINT/SIGTERM only; HUP kills the container).
	cfg.Provisioning.EnableWatch = true
	cfg.Datasource.Proxy.AllowedNetworks = []string{"127.0.0.0/8", "::1/128"}
	return cfg
}

// PersesGlobalDatasourceResource is the Perses-native GlobalDatasource
// provisioning resource (v0.55 schema) for the BaseHarbor Prometheus backend.
type PersesGlobalDatasourceResource struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Default bool `json:"default"`
		Plugin  struct {
			Kind string `json:"kind"`
			Spec struct {
				Proxy struct {
					Kind string `json:"kind"`
					Spec struct {
						URL string `json:"url"`
						// The pinned perses/spec proxy schema (cue:
						// github.com/perses/spec datasource/proxy/http) declares
						// "secret?: string" — the NAME of the Secret resource the
						// proxy anchors to. Not an object.
						Secret           string `json:"secret,omitempty"`
						AllowedEndpoints []struct {
							EndpointPattern string `json:"endpointPattern"`
							Method          string `json:"method"`
						} `json:"allowedEndpoints"`
					} `json:"spec"`
				} `json:"proxy"`
			} `json:"spec"`
		} `json:"plugin"`
	} `json:"spec"`
}

// NewPersesGlobalDatasourceResource converts the validated builder result into
// the provisioning resource. Fail-closed on the endpoint contract: an empty or
// unparsable management URL never produces a resource.
func NewPersesGlobalDatasourceResource(res PersesPrometheusDatasourceResult) (PersesGlobalDatasourceResource, error) {
	secretName := PersesDatasourceTLSSecretName
	var resource PersesGlobalDatasourceResource
	resource.Kind = PersesDatasourceProvisioningKind
	resource.Metadata.Name = PersesDatasourceProvisioningResourceName
	resource.Spec.Default = res.Datasource.Default
	resource.Spec.Plugin.Kind = res.Datasource.Kind // pinned native plugin kind (PrometheusDatasource)
	resource.Spec.Plugin.Spec.Proxy.Kind = PersesDatasourceProxyKind
	endpoint, err := persesDatasourceProxyURL(res.Datasource.Config.URL)
	if err != nil {
		return PersesGlobalDatasourceResource{}, err
	}
	resource.Spec.Plugin.Spec.Proxy.Spec.URL = endpoint
	// The proxy's TLS trust anchor (F3): the datasource references the
	// provisioned GlobalSecret by NAME (the pinned perses/spec HTTPProxy
	// schema carries trust through proxy.spec.secret -> a GlobalSecret whose
	// spec.tlsConfig pins the endpoint); no secret VALUE and no key material
	// ever lands in the datasource descriptor. The secret resource itself is
	// materialized beside it from the projected public CA bundle.
	resource.Spec.Plugin.Spec.Proxy.Spec.Secret = secretName
	for _, allowed := range persesPrometheusAllowedEndpoints {
		resource.Spec.Plugin.Spec.Proxy.Spec.AllowedEndpoints = append(
			resource.Spec.Plugin.Spec.Proxy.Spec.AllowedEndpoints,
			struct {
				EndpointPattern string `json:"endpointPattern"`
				Method          string `json:"method"`
			}{EndpointPattern: allowed.EndpointPattern, Method: allowed.Method})
	}
	return resource, nil
}

// persesDatasourceProxyURL validates the datasource endpoint for the proxy.
// The production form (0428-083 F2) is the IN-NETWORK endpoint the Perses
// container can actually route: the https gateway service DNS with NO port
// (ProviderNetworkEndpoint). The loopback host form (https://127.0.0.1:<port>)
// is accepted too — it is the shape the isolated acceptance cell materializes,
// where the endpoint namespace IS the container's namespace. Credentials,
// paths, queries and fragments never pass, in either form. Invalid endpoints
// fail closed.
func persesDatasourceProxyURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("perses datasource proxy endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("perses datasource proxy endpoint %q must be an http(s) management URL", raw)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("perses datasource proxy endpoint %q must carry host and port only", raw)
	}
	// Normalize to the URL grammar (RFC 3986 3.2.2): a bare IPv6 host carries
	// brackets with a bare port, e.g. [::1]:8080. A parsed.Host that still
	// holds a space (an unparsable address) fails closed — the proxy url is a
	// URL, never free text.
	hostport := parsed.Host
	if ip := net.ParseIP(hostport); ip != nil && ip.To4() == nil {
		hostport = "[" + hostport + "]"
	}
	if _, err := url.Parse(parsed.Scheme + "://" + hostport); err != nil {
		return "", fmt.Errorf("perses datasource proxy endpoint %q is not a valid URL: %w", raw, err)
	}
	return parsed.Scheme + "://" + hostport, nil
}

// PersesDatasourceGlobalSecretResource is the Perses-native GlobalSecret
// (v0.55 schema) carrying the PUBLIC trust material the datasource proxy
// validates the gateway endpoint against. It is a provisioning resource like
// the datasource itself: the loader keeps it in the Perses file database.
type PersesDatasourceGlobalSecretResource struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		TLSConfig struct {
			CAFile     string `json:"caFile,omitempty"`
			ServerName string `json:"serverName,omitempty"`
		} `json:"tlsConfig"`
	} `json:"spec"`
}

// NewPersesDatasourceGlobalSecretResource renders the trust-anchor resource
// from the materialized endpoint URL: the caFile names the mounted public CA
// bundle and the serverName pins the gateway identity the endpoint resolves to
// (the Prometheus service-access contract: the certificate serves "prometheus"
// behind the gateway's publish address). The loopback host form carries no
// name to pin (its certificate covers the loopback address). Fail-closed on an
// unparsable endpoint, like the datasource resource conversion.
func NewPersesDatasourceGlobalSecretResource(endpoint string) (PersesDatasourceGlobalSecretResource, error) {
	var secret PersesDatasourceGlobalSecretResource
	secret.Kind = PersesDatasourceSecretKind
	secret.Metadata.Name = PersesDatasourceTLSSecretName
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return PersesDatasourceGlobalSecretResource{}, fmt.Errorf("perses datasource trust anchor: invalid endpoint %q", endpoint)
	}
	secret.Spec.TLSConfig.CAFile = PersesDatasourceTrustBundleContainerPath
	host, _, splitErr := net.SplitHostPort(parsed.Host)
	if splitErr != nil {
		host = parsed.Hostname()
	}
	if ip := net.ParseIP(host); ip == nil {
		secret.Spec.TLSConfig.ServerName = host
	}
	return secret, nil
}

// PersesDatasourceGlobalSecretResourceJSON marshals the trust-anchor resource
// deterministically (json.MarshalIndent with trailing newline).
func PersesDatasourceGlobalSecretResourceJSON(r PersesDatasourceGlobalSecretResource) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PersesDatasourceTrustBundlePath is the host-side public CA bundle of a
// datasource projection root (the file the trust-anchor mount delivers to
// PersesDatasourceTrustBundleContainerPath).
func PersesDatasourceTrustBundlePath(projectionRoot string) string {
	return filepath.Join(filepath.Clean(projectionRoot), persesDatasourceTrustBundleFileName)
}

// projectionPersesDatasourceTrustBundle copies the provider's runtime trust
// bundle into the datasource projection under the reserved name (public PEM
// material only, never a key). Missing provider material is fail-closed: the
// datasource proxy must never be provisioned without its trust anchor.
func projectionPersesDatasourceTrustBundle(files ProviderFiles, projectionRoot string) error {
	data, err := os.ReadFile(files.RuntimeCA)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("perses datasource trust anchor: the Prometheus provider carries no runtime CA bundle (provision the existing provider first)")
		}
		return fmt.Errorf("perses datasource trust anchor: read provider runtime CA: %w", err)
	}
	if err := os.MkdirAll(projectionRoot, 0o700); err != nil {
		return fmt.Errorf("perses datasource trust bundle directory: %w", err)
	}
	return writePersesDatasourceFile(PersesDatasourceTrustBundlePath(projectionRoot), data)
}

// PersesDatasourceProviderPaths returns the datasource projection paths under
// the data root: the config file, the provisioning folder and the public trust
// bundle. They are rooted
// at <dataDir>/applications/<resourceProject>/providers/management-ui/perses
// — the same providers/management-ui/perses/ layout the Perses TLS material
// already uses (./providers/management-ui/perses/server.cert bind mounts), so
// the compose render mounts exactly these sources into the container
// (config.yaml:/etc/perses/config.yaml:ro, the folder:/etc/perses/
// provisioning:ro and the bundle:/etc/perses/baseharbor-runtime-ca.pem:ro,
// the F3 trust anchor mount).
func PersesDatasourceProviderPaths(dataDir, namespace string, m application.Manifest) (configFile, provisioningFolder, trustBundle string, err error) {
	root, err := persesDatasourceProjectionRoot(dataDir, namespace, m)
	if err != nil {
		return "", "", "", err
	}
	return root + "/config.yaml", root + "/" + persesDatasourceProvisioningDir, PersesDatasourceTrustBundlePath(root), nil
}

// MaterializePersesPrometheusDatasourceAt provisions the validated datasource
// for the RUNNING Perses on the given Target data root/namespace (empty = the
// default local root): it builds fail-closed and writes the config file, the
// provisioning resource, the public trust bundle and the GlobalSecret that
// references it, where the Perses runtime service reads them (the compose
// mounts of the datasource pair live in the Perses runtime render of card
// 0428-088/083; the trust-bundle mount is this card's pair addition, F2/F3).
// The build runs in the RUNTIME context: the proxy URL names the in-network
// gateway endpoint, never a host-loopback address the container cannot route.
// Fail-closed: every builder error also REMOVES any
// previously provisioned datasource (an unselected, unauthorized or
// unreachable surface never keeps serving a stale datasource), and a failed
// write is an error, never a silent skip. Returns the result and the config
// file / provisioning resource paths.
func MaterializePersesPrometheusDatasourceAt(m application.Manifest, dataDir, namespace string) (PersesPrometheusDatasourceResult, string, string, error) {
	root, rootErr := persesDatasourceProjectionRoot(dataDir, namespace, m)
	if rootErr != nil {
		return PersesPrometheusDatasourceResult{}, "", "", rootErr
	}
	return materializePersesDatasourceAtRoot(m, root, dataDir, namespace)
}

// MaterializePersesPrometheusDatasourceAtWithEndpoint is the acceptance-cell
// variant of the materialization entry: identical contract and artifacts, but
// the datasource endpoint is the caller-provided live endpoint (a host-
// namespace cell serves its fake Prometheus on the host loopback, while a
// production deployment shares the compose network with the provider). The
// production paths never call it.
func MaterializePersesPrometheusDatasourceAtWithEndpoint(m application.Manifest, dataDir, namespace, endpoint string) (PersesPrometheusDatasourceResult, string, string, error) {
	root, rootErr := persesDatasourceProjectionRoot(dataDir, namespace, m)
	if rootErr != nil {
		return PersesPrometheusDatasourceResult{}, "", "", rootErr
	}
	return materializePersesDatasourceAtRoot(m, root, dataDir, namespace, endpoint)
}

// materializePersesDatasourceAtRoot is the single materialization core every
// provisioning path shares (the manifest-bound entry above and the lifecycle
// seam of the application apply boundary): the build runs in the RUNTIME
// context (F2 — the proxy URL names the in-network gateway endpoint, because
// a container cannot route the host's loopback and the published host port is
// meaningless inside the network), the public trust bundle of the SAME
// provider state the build resolved is projected beside the artifacts (F3),
// and the provisioning folder carries the GlobalDatasource plus its
// trust-anchor GlobalSecret. Fail-closed: every build error removes any prior
// projection at the root (a deselected, unauthorized or unreachable surface
// never keeps serving a stale datasource), and a failed write is an error,
// never a silent skip.
func materializePersesDatasourceAtRoot(m application.Manifest, root, dataDir, namespace string, endpointOverride ...string) (PersesPrometheusDatasourceResult, string, string, error) {
	res, err := buildPersesPrometheusDatasourceContextual(m, capability.PersesDefaultRelease, unavailablePrometheusProbe{}, dataDir, namespace, buildContextRuntime, endpointOverride...)
	if err != nil {
		// Fail-closed teardown: no datasource may remain provisioned when the
		// build (selection, pin, authorization, sources, endpoint) fails.
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	resource, err := NewPersesGlobalDatasourceResource(res)
	if err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	// F3: the trust anchor rides a provisioned GlobalSecret (public CA
	// material only). Resolve the provider files fail-closed and project the
	// bundle before any datasource artifact is written.
	providerFiles, err := persesDatasourceTrustSource(res)
	if err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	secret, err := NewPersesDatasourceGlobalSecretResource(resource.Spec.Plugin.Spec.Proxy.Spec.URL)
	if err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return PersesPrometheusDatasourceResult{}, "", "", errors.Join(err, rmErr)
		}
		return PersesPrometheusDatasourceResult{}, "", "", err
	}
	configData, err := PersesDatasourceConfigJSON(NewPersesDatasourceConfig())
	if err != nil {
		return res, "", "", fmt.Errorf("perses datasource config: %w", err)
	}
	resourceData, err := PersesGlobalDatasourceResourceJSON(resource)
	if err != nil {
		return res, "", "", fmt.Errorf("perses datasource provisioning resource: %w", err)
	}
	secretData, err := PersesDatasourceGlobalSecretResourceJSON(secret)
	if err != nil {
		return res, "", "", fmt.Errorf("perses datasource trust-anchor resource: %w", err)
	}
	configPath := filepath.Join(root, persesDatasourceConfigFile)
	provisioningDir := filepath.Join(root, persesDatasourceProvisioningDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return res, "", "", fmt.Errorf("perses datasource projection directory: %w", err)
	}
	// The loader loads EVERY file in the provisioning folder, so the folder
	// must never carry two datasource generations: clear the regular files of
	// an earlier build before writing this one's (0428-083 rematerialize).
	if err := clearPersesDatasourceProvisioningFolder(provisioningDir); err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return res, "", "", errors.Join(err, rmErr)
		}
		return res, "", "", err
	}
	if err := projectionPersesDatasourceTrustBundle(providerFiles, root); err != nil {
		if rmErr := removePersesDatasourceProjection(root); rmErr != nil {
			return res, "", "", errors.Join(err, rmErr)
		}
		return res, "", "", err
	}
	if err := writePersesDatasourceFile(configPath, configData); err != nil {
		return res, "", "", fmt.Errorf("write perses datasource config: %w", err)
	}
	if err := os.MkdirAll(provisioningDir, 0o755); err != nil {
		return res, "", "", fmt.Errorf("perses datasource provisioning folder: %w", err)
	}
	if err := writePersesDatasourceFile(filepath.Join(provisioningDir, persesDatasourceProvisioningFile), resourceData); err != nil {
		return res, "", "", fmt.Errorf("write perses datasource provisioning resource: %w", err)
	}
	if err := writePersesDatasourceFile(filepath.Join(provisioningDir, persesDatasourceSecretFile), secretData); err != nil {
		return res, "", "", fmt.Errorf("write perses datasource trust-anchor resource: %w", err)
	}
	return res, configPath, filepath.Join(provisioningDir, persesDatasourceProvisioningFile), nil
}

// persesDatasourceTrustSource resolves the public trust material of the
// provider state the build used (F3): the datasource proxy anchors to the
// SAME CA bundle that secures the endpoint it serves. Fail-closed: an absent
// provider runtime CA is an error, never a silent empty trust.
func persesDatasourceTrustSource(res PersesPrometheusDatasourceResult) (ProviderFiles, error) {
	if strings.TrimSpace(res.ProviderFiles.RuntimeCA) == "" {
		return ProviderFiles{}, errors.New("perses datasource trust anchor: the Prometheus provider state carries no runtime CA bundle (provision the existing provider first)")
	}
	if _, err := os.Stat(res.ProviderFiles.RuntimeCA); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProviderFiles{}, fmt.Errorf("perses datasource trust anchor: the provider runtime CA %s does not exist (provision the existing provider first)", res.ProviderFiles.RuntimeCA)
		}
		return ProviderFiles{}, fmt.Errorf("perses datasource trust anchor: inspect the provider runtime CA: %w", err)
	}
	return res.ProviderFiles, nil
}

// PersesDatasourceResourcePath is the provisioning resource file the
// materializer writes for the given data root/namespace (empty = the default
// local root): the globaldatasource-<name>.json inside the projection's
// provisioning folder. Lifecycle and acceptance consumers resolve the
// materialized artifact from its result without re-deriving the layout.
func PersesDatasourceResourcePath(m application.Manifest, dataDir, namespace string) (string, error) {
	root, err := persesDatasourceProjectionRoot(dataDir, namespace, m)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, persesDatasourceProvisioningDir, persesDatasourceProvisioningFile), nil
}

// RemovePersesPrometheusDatasourceAt tears down exactly this card's datasource
// projection subtree (config file + provisioning folder). Ownership-safe: it
// removes only the projection directory it owns (the Perses UI projection root
// of this application), never sibling TLS material or foreign content.
func RemovePersesPrometheusDatasourceAt(m application.Manifest, dataDir, namespace string) error {
	root, err := persesDatasourceProjectionRoot(dataDir, namespace, m)
	if err != nil {
		return err
	}
	return removePersesDatasourceProjection(root)
}

func removePersesDatasourceProjection(root string) error {
	if err := os.RemoveAll(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove perses datasource provisioning: %w", err)
	}
	return nil
}

// persesDatasourceProjectionRoot resolves the datasource projection root of a
// deployment addressed by its data root and manifest through the ONE layout
// convention of this route: the datasource root is the
// providers/management-ui/perses sibling of the manifest's RUNTIME dir, and
// the runtime dir under a data root is what application.RuntimeFilesFor
// computes. The lifecycle (which holds the real runtime dir) and this
// manifest-bound entry therefore address the same root on every layout, by
// construction (0428-083 F1).
func persesDatasourceProjectionRoot(dataDir, namespace string, m application.Manifest) (string, error) {
	root, err := bahaDataDir(dataDir)
	if err != nil {
		return "", err
	}
	// The datasource root is the providers/management-ui/perses SIBLING of the
	// deployment's runtime dir — the ONE application-side formula
	// (application.PersesDatasourceRuntimeRoot), never a second derivation.
	// A deployment is addressed by its DATA root: the namespace-scoped layout
	// is <dataRoot>/applications/<resourceProject>/runtime (what the lifecycle
	// store computes through RuntimeFilesFor); the local store layout roots
	// the runtime dir at <dataRoot>/<app>/runtime.
	if ns := strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-")); ns != "" {
		return application.PersesDatasourceRuntimeRoot(
			filepath.Join(filepath.Clean(root), "applications",
				"baseharbor-"+ns+"-"+m.Name+"-"+m.Environment, "runtime")), nil
	}
	return application.PersesDatasourceRuntimeRoot(
		application.RuntimeFilesFor(application.Store{Root: root}, m).Dir), nil
}

func bahaDataDir(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) != "" {
		return filepath.Clean(dataDir), nil
	}
	return bhruntime.DataDir("")
}

// BuildPersesPrometheusDatasourceAt is the Target-bound public build entry:
// the same fail-closed contract as BuildPersesPrometheusDatasource, resolved
// against dataDir/namespace (empty = the default local root), so the deployment
// path of a remote Target builds the datasource it actually runs.
func BuildPersesPrometheusDatasourceAt(m application.Manifest, dataDir, namespace string) (PersesPrometheusDatasourceResult, error) {
	return buildPersesPrometheusDatasourceAt(m, unavailablePrometheusProbe{}, dataDir, namespace)
}

// PersesDatasourceConfigJSON marshals the rendered Perses config
// deterministically (json.MarshalIndent with trailing newline).
func PersesDatasourceConfigJSON(cfg PersesDatasourceConfig) ([]byte, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PersesGlobalDatasourceResourceJSON marshals the provisioning resource
// deterministically (json.MarshalIndent with trailing newline).
func PersesGlobalDatasourceResourceJSON(r PersesGlobalDatasourceResource) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writePersesDatasourceFile writes the projection with 0600 then relaxes it to
// the readable projection mode the managed-UI pattern uses (the distroless
// Perses process cannot read owner-only bind mounts): owner write first, then
// chmod 0644, like projectUIReadableFile in the application package.
func writePersesDatasourceFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}
