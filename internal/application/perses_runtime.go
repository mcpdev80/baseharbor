package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// PersesUIImage is the pinned, reviewed Perses visualization image. It is
// realization metadata (product version), never a provider or capability
// version. Bump only on an reviewed security-current candidate.
const PersesUIImage = "docker.io/persesdev/perses:v0.55.1"

// PersesUIHostPortEnv is the application-scoped loopback host port the optional
// Perses UI is published to. It is materialized in the runtime env only when
// Perses is selected.
const PersesUIHostPortEnv = "BASEHARBOR_PERSES_UI_HOST_PORT"

// persesUIContainerTLSPort is the in-container HTTPS port. Perses defaults to
// :8080 for plain HTTP; the TLS environment settings below make the server
// listen for HTTPS on the same port.
const persesUIContainerTLSPort = 8080

// Perses is an OPTIONAL, provider-owned visualization surface that realizes the
// observability management-ui seam defined by the accepted Perses contract
// (task t-20261114-r28-perses-contract-seam, task t-20261114-r28-perses-runtime).
// It is never an application capability and never a required Core bundle: it
// rides the existing Docker/Podman compose lifecycle as a single-instance,
// managed, fail-closed service that is materialized ONLY when an application
// selects it (services.perses) AND a pinned release validates.

// PersesRuntimeVolumeName is the managed, application-scoped data volume for the
// optional Perses UI. It is a pure volume (no HA member, no gateway): Perses is
// a single-instance surface.
func PersesRuntimeVolumeName(resourceProject, service string) string {
	return resourceProject + "_" + service + "-data"
}

// writePersesUIComposeService renders the optional Perses UI as a single-instance
// compose service. It is gated by the caller on services.perses being selected
// AND observability.management_ui being enabled; without that gate the surface
// must not appear in the compose definition at all (fail-closed not_configured).
func writePersesUIComposeService(b *strings.Builder, m Manifest, resourceProject string) {
	service := runtimeServiceName("perses", defaultServiceInstance)
	b.WriteString("  " + service + ":\n")
	b.WriteString("    image: " + PersesUIImage + "\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	// Perses ships a health probe on /healthz; BaseHarbor does not assert it as
	// "verified": the three-state contract stays configured_unverified. The
	// surface is HTTPS per the accepted contract (PersesManagementUIInterface
	// protocol "https"): EnsurePersesManagementUI projects the serviceaccess
	// TLS material and the entrypoint below hands the server exactly the
	// mounted /certs paths. The image default CMD serves plain HTTP, and the
	// perses web flags are flag-only (the PERSES_ environment namespace drives
	// the config file keys, not --web.*), so the command line is overridden.
	// The distroless nonroot image cannot read 0600 bind mounts, which is why
	// the projection uses projectUIReadableFile (0644) copies like the other
	// managed UIs.
	// Perses extracts the configured plugin archives (for example the shipped
	// UI) into /etc/perses/plugins at runtime; with read_only:true that path is
	// unwritable and the extraction fails (GET /api/v1/plugins would return 0
	// modules). The image ships the directory owned by uid/gid 1000, so an
	// isolated writable tmpfs at exactly that path with mode 1777 lets the
	// nonroot process extract the archives while the root filesystem stays
	// read-only.
	b.WriteString("    entrypoint: [\"/bin/perses\"]\n")
	fmt.Fprintf(b, "    command: [\"--config=/etc/perses/config.yaml\", \"--web.listen-address=0.0.0.0:%d\", \"--web.tls-cert-file=/certs/server.cert\", \"--web.tls-key-file=/certs/server.key\", \"--log.level=error\"]\n", persesUIContainerTLSPort)
	b.WriteString("    tmpfs:\n      - /etc/perses/plugins:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(b, "      - \"127.0.0.1:${%s}:%d\"\n", PersesUIHostPortEnv, persesUIContainerTLSPort)
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./providers/management-ui/perses/server.cert:/certs/server.cert:ro\n")
	b.WriteString("      - ./providers/management-ui/perses/server.key:/certs/server.key:ro\n")
	fmt.Fprintf(b, "      - %s-data:/persistence\n", service)
	b.WriteString("    networks:\n")
	b.WriteString("      default:\n")
	fmt.Fprintf(b, "        aliases:\n          - %q\n", devaccess.ApplicationAlias(m.Name, service))
	b.WriteString("\n")
}

// PersesUITLSRequired reports whether Perses TLS provider files must be projected
// for the current manifest. TLS is projected only for a selected, observability
// surface so a deselected surface never materializes any runtime artifact.
func PersesUITLSRequired(m Manifest) bool {
	return m.Services.Perses && m.Services.ObservabilityManagementUI
}

// EnsurePersesManagementUI projects the optional Perses provider files (TLS +
// persistence marker) under providers/management-ui/perses/runtime. It is
// fail-closed: it is a no-op unless Perses is selected with observability
// management_ui, so an unselected surface leaves no files behind.
func EnsurePersesManagementUI(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	if !PersesUITLSRequired(m) {
		return nil
	}
	if issuer == nil {
		return fmt.Errorf("perses TLS material requires a service issuer")
	}
	dir := filepath.Join(files.Dir, "providers", "management-ui", "perses")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Perses is a loopback-only HTTPS surface; the projection mirrors the
	// Postgres UI pattern (serviceaccess TLS material -> server.cert/server.key)
	// so the compose mounts always name files that exist. The three-state
	// contract still never declares a verified health path.
	policy, err := serviceaccess.Resolve(m.Environment, "perses-ui", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), "localhost", "127.0.0.1", "perses")
	if err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerCertificate, filepath.Join(dir, "server.cert")); err != nil {
		return err
	}
	if err := projectUIReadableFile(material.ServerKey, filepath.Join(dir, "server.key")); err != nil {
		return err
	}
	marker := filepath.Join(dir, "persistence")
	if err := writeUIRuntimeProjection(marker, []byte("perses\n")); err != nil {
		return err
	}
	return nil
}

// PersesManagementUIPath returns the runtime projection directory for the
// optional Perses UI, used by the lifecycle verify gate.
func PersesManagementUIPath(files RuntimeFiles) string {
	return filepath.Join(files.Dir, "providers", "management-ui", "perses")
}
