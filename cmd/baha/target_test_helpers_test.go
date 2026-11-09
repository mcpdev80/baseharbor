package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const testDeploymentID = "22222222-2222-4222-8222-222222222222"

// Only source-contract tests use this fixture. Actual Core readiness and
// bootstrap behavior are exercised separately; no global test bypass exists.
func assumeVerifiedCoreForSourceTest(t *testing.T) {
	t.Helper()
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error { return nil }
	t.Cleanup(func() { applicationCorePrerequisite = previous })
}

func configureTestTarget(t *testing.T) deployment.ResolvedTarget {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "docker-dev",
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"docker-dev": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-docker"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	// A managed deployment requires its authoritative Core installation to
	// govern the Target; register one so the production deployment-owner binding
	// passes. Coreless, discovery, and read-only decision tests that assert an
	// empty data root must instead use configureTestTargetEmpty (below).
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatal(err)
	}
	return target
}

// configureTestTargetEmpty registers a Target without a Core installation,
// for coreless / discovery / read-only tests that assert no installation-owned
// Target state was created.
func configureTestTargetEmpty(t *testing.T) deployment.ResolvedTarget {
	target := configureTestTarget(t)
	if stateRoot, err := deployment.TargetStateRoot(target.Name); err == nil {
		// A removal failure or unresolved target root is a setup error, not a
		// benign no-op: the test asserts a specific empty data root, so a
		// swallowed error would silently leave installation-owned state behind
		// and let the assertion pass on the wrong filesystem. Fail closed.
		if err := os.RemoveAll(stateRoot); err != nil {
			t.Fatalf("remove installation-owned state for %s: %v", target.Name, err)
		}
	} else {
		t.Fatalf("resolve target root for empty fixture: %v", err)
	}
	return target
}

// configureTestTargetWithOwner is retained for callers that register the
// authoritative Core installation explicitly; configureTestTarget now provisions
// an owner by default, so it is equivalent but documented for clarity.
func configureTestTargetWithOwner(t *testing.T) deployment.ResolvedTarget {
	return configureTestTarget(t)
}

// isolatedTargetName returns a per-process, per-test unique Target name whose
// shared control-plane project (bh-<name>-shared) and default network never
// collide with another test. configureTestTarget fixes the name at "docker-dev",
// so every test that boots a real control plane shares ONE docker network
// (bh-docker-dev-shared); a Core left running by another test on that network
// is picked up by an isolated-up assertion that expects "no Core here" and makes
// TestUpNameResumesThroughCommandEntrypoint pass in isolation but fail in the
// full matrix. A unique Target name gives each test its own isolated network.
func isolatedTargetName(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate isolated target name: %v", err)
	}
	return "itest-" + fmt.Sprintf("%x", buf) + "-" + strconv.FormatInt(time.Now().UnixNano()%100000, 10)
}

// configureIsolatedTestTarget is the hermetic twin of configureTestTarget. It
// clears any TARGET/DATA/ENV leaked by an earlier test in the same package
// (BASEHARBOR_TARGET, BASEHARBOR_STATE_DIR, BASEHARBOR_ENV are set globally by
// the runtime-integration bootstrap via os.Setenv and never rolled back), then
// provisions a single fresh XDG root and a UNIQUE Target so its control-plane
// docker project/network is isolated from every other test. registerTestDeployment
// and the up entrypoint both resolve their Target from this Test's config, so a
// leaked BASEHARBOR_TARGET cannot redirect them to a foreign Core.
//
// Isolation alone is not enough for TestUpNameResumesThroughCommandEntrypoint:
// the registered-NAME up must run from the repository root (not an empty dir) so
// `baha up NAME` resolves the registered deployment and fails at runtime
// realization (no running Core) instead of taking the fresh-core bootstrap path
// and booting a real control plane when a docker daemon is available.
func configureIsolatedTestTarget(t *testing.T) deployment.ResolvedTarget {
	t.Helper()
	// Clear leaked globals FIRST: a surviving BASEHARBOR_TARGET from another
	// lane's bootstrap would make effectiveTarget() ignore this Test's config
	// and resolve against a foreign, possibly running, Core.
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_ENV", "")

	name := isolatedTargetName(t)
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dataHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: name,
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			name: {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-docker"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	// A managed deployment requires its authoritative Core installation to
	// govern the Target; register one on the isolated Target so the production
	// deployment-owner binding passes. The no-op installation writes state only
	// and boots no container, so up NAME reaches control-plane setup and fails
	// at runtime realization (no Core) deterministically.
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatal(err)
	}
	return target
}

// registerInstallationForTarget persists a valid Core installation under the
// Target's runtime root (targets/<target>/runtime) if one is not already
// present, so deployment-owner binding passes for managed-deployment tests.
func registerInstallationForTarget(t *testing.T, targetName string) error {
	t.Helper()
	installRoot, err := installationRuntimeRoot(t, targetName)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(filepath.Join(installRoot, "installation.json")); !os.IsNotExist(statErr) {
		return statErr
	}
	_, runErr := coreinstallation.Run(context.Background(), installRoot, coreinstallation.Spec{Target: targetName, Runtime: "docker", MachineRole: coreinstallation.Development}, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	})
	return runErr
}

// installationRuntimeRoot resolves the Core installation runtime root
// (XDG_DATA_HOME/baseharbor/targets/<target>/runtime), the path
// deployment.TargetStateRoot derives (with the shared "baseharbor/" prefix).
func installationRuntimeRoot(t *testing.T, targetName string) (string, error) {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		return "", err
	}
	return filepath.Join(stateRoot, "runtime"), nil
}

// materializeKeycloakCoreIssuerForTarget writes the Core's Keycloak core issuer
// public TLS material and runtime environment under the plain target-state root
// (targets/<target>/providers/keycloak/shared/core) so the read-only
// VerifyCoreConnection boundary observes the Core's Keycloak-bound discovery
// identity from disk. This is the REAL production issuer location the Core
// materializes at bootstrap (Keycloak/OpenBao never write a local base CA under
// service-access/issuer), so use/activate verifies the genuine Core identity
// instead of a foreign local base CA.
func materializeKeycloakCoreIssuerForTarget(t *testing.T, plainTargetRoot string, identityIssuer string) {
	t.Helper()
	coreDir := filepath.Join(plainTargetRoot, "providers", "keycloak", "shared", "core")
	if _, err := os.Stat(coreDir); err == nil {
		return
	}
	if err := os.MkdirAll(coreDir, 0700); err != nil {
		t.Fatalf("create Keycloak core issuer dir: %v", err)
	}
	pkiDir := filepath.Join(coreDir, "native-tls", "pki")
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		t.Fatalf("create Keycloak core issuer pki dir: %v", err)
	}
	port, err := parseKeycloakPublicPort(identityIssuer)
	if err != nil {
		t.Fatalf("parse Keycloak public port from identity issuer: %v", err)
	}
	// Generate a self-signed public CA whose SAN binds identity.localhost:<port>,
	// the identity the verified Core publishes at runtime.
	cert, key, ref := selfSignedKeycloakPublicCA(t, pkiDir, port)
	_ = cert
	_ = ref
	_ = key
	// runtime.env binds the public discovery port (read by VerifyCoreConnection).
	envValues := map[string]string{
		"BASEHARBOR_KEYCLOAK_PUBLIC_PORT":    strconv.Itoa(port),
		"BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD": "0000000000000000000000000000000000000000000000000000000000000000",
		"BASEHARBOR_KEYCLOAK_DB_PASSWORD":    "0000000000000000000000000000000000000000000000000000000000000000",
	}
	if err := writeProtectedEnvForTest(filepath.Join(coreDir, "runtime.env"), envValues); err != nil {
		t.Fatalf("write Keycloak core runtime.env: %v", err)
	}
}

// parseKeycloakPublicPort extracts the public port from an identity issuer URL
// of the form https://identity.localhost:<port>/realms/baseharbor.
func parseKeycloakPublicPort(identityIssuer string) (int, error) {
	idx := strings.Index(identityIssuer, "://")
	if idx < 0 {
		return 0, fmt.Errorf("invalid identity issuer %q", identityIssuer)
	}
	rest := identityIssuer[idx+3:]
	hostPort := strings.SplitN(rest, "/", 2)[0]
	_, portStr, ok := strings.Cut(hostPort, ":")
	if !ok {
		return 0, fmt.Errorf("identity issuer missing port %q", identityIssuer)
	}
	port, perr := strconv.Atoi(portStr)
	if perr != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid Keycloak public port %q", portStr)
	}
	return port, nil
}

// selfSignedKeycloakPublicCA generates a self-signed public CA whose SAN binds
// identity.localhost:<port> and writes native-tls/pki/{ca.pem,server-cert.pem,
// server-key.pem}. It returns the issuer reference (https://identity.localhost:
// <port>/realms/baseharbor). It fails closed on any crypto error so a malformed
// test material never silently passes.
func selfSignedKeycloakPublicCA(t *testing.T, pkiDir string, port int) (x509.Certificate, *ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate Keycloak public CA key: %v", err)
	}
	notBefore := time.Now().Add(-time.Minute).UTC()
	notAfter := notBefore.Add(365 * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "identity.localhost"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		DNSNames:              []string{"identity.localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create Keycloak public CA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse Keycloak public CA: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certPEM, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	_ = certPEM // unused; server-key.pem below carries the signing key
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal Keycloak public CA private key: %v", err)
	}
	files := map[string][]byte{
		"ca.pem":          caPEM,
		"server-cert.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"server-key.pem":  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(pkiDir, name), data, 0600); err != nil {
			t.Fatalf("write Keycloak public CA file %q: %v", name, err)
		}
	}
	ref := fmt.Sprintf("https://identity.localhost:%d/realms/baseharbor", port)
	return *cert, key, ref
}

// writeProtectedEnvForTest writes a protected runtime.env (sorted key=value
// lines, 0600 mode) matching the Core's production runtime.env shape.
func writeProtectedEnvForTest(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

var _ = errors.New

func registerTestDeployment(t *testing.T, target deployment.ResolvedTarget, m application.Manifest, repositoryRoot, manifestPath string) deployment.DeploymentRecord {
	t.Helper()
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  testDeploymentID,
			ApplicationID: m.ApplicationID,
			Target:        target.Name,
			Application:   m.Name,
			Environment:   m.Environment,
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: repositoryRoot,
			Manifest:   manifestPath,
		},
		Applied: deployment.AppliedDeployment{
			Intent:          intent,
			RuntimeProvider: target.RuntimeProvider,
		},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func testDeploymentStore(t *testing.T, target deployment.ResolvedTarget, m application.Manifest) application.Store {
	t.Helper()
	identity := deployment.DeploymentIdentity{
		DeploymentID:  testDeploymentID,
		ApplicationID: m.ApplicationID,
		Target:        target.Name,
		Application:   m.Name,
		Environment:   m.Environment,
	}
	root, err := deployment.DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Applied:  deployment.AppliedDeployment{RuntimeProvider: target.RuntimeProvider},
	}); err != nil {
		t.Fatal(err)
	}
	return application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}
}
