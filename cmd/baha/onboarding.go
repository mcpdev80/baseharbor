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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ma "github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/onboardingwizard"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// onboarding.go is the guided onboarding production path for card
// t-20261018-r27-onboarding-wizard (0427-043): one new user/organization
// reaches a functioning BaseHarbor installation step by step with the minimum
// number of human decisions. The presentation controller lives in
// internal/onboardingwizard; this file binds it to the accepted authorities:
//
//   - bootstrap and attach are distinct intents (core-777: onboarding is
//     separate from bootstrap); joining never re-installs services that
//     already exist (dev-remote onboarding rule);
//   - nothing already discoverable is asked (installation identity, Target
//     list, existing registration, valid reusable access);
//   - authentication reuses the existing operator session; a valid session
//     causes no questionnaire;
//   - the persistence choice is exactly "secure for future use" versus
//     "session-only"; session-only material lives in the protected per-user
//     runtime directory with owner-only permissions and is removed after use;
//   - Back/cancel/EOF abort before commit; commit is the single effect point
//     and conflicts fail closed;
//   - importing one trusted device bundle is validated before any mutation.
//
// Public command registration (the shared command tree in commands.go, the
// machine classification and the generated documentation indexes) belongs to
// t-20261018-r27-public-surfaces; this slice therefore keeps the command
// factory hidden from rootCommand() and provides the callable production path
// through the onboardingCommand() factory plus the seams below.

const (
	onboardingConnectionContract = "baseharbor.onboarding-connection/v1"
	onboardingConnectionFileName = "onboarding-connection.json"
	// onboardingProfileScope is the canonical lowercase scope slug the
	// onboarding store binding pins imported team profiles to; the profile
	// schema forbids UUID-shaped scopes.
	onboardingProfileScope = "management-access"
	onboardingEnvironment  = "dev"
)

// onboardingInput is the wizard stdin seam; tests replace it.
var onboardingInput io.Reader = os.Stdin

// onboardingAdapters are the injectable production authorities behind the
// commit effects. Defaults are nil (fail closed); init() binds the accepted
// implementations. Call sites resolve the seam at call time, which keeps the
// initialization graph acyclic and lets tests replace individual adapters.
var onboardingAdapters onboardingAuthorityAdapters

func init() {
	onboardingAdapters.Discover = discoverOnboardingState
	onboardingAdapters.ConnectionBound = boundManagementConnectionIdentity
	onboardingAdapters.AppAttach = attachExistingApplication
	onboardingAdapters.ImportProfile = importConnectionProfileTransactionally
	onboardingAdapters.ImportBundle = redeemOnboardingBundle
}

type onboardingAuthorityAdapters struct {
	Discover        func(context.Context) (onboardingwizard.InstallState, error)
	ConnectionBound func(context.Context, string) (string, error)
	AppAttach       func(context.Context) error
	ImportProfile   func(context.Context, string) error
	// ImportBundle redeems one accepted one-use workstation-enrollment bundle
	// through the production enrollment seam and returns the resulting
	// installation identity the join binds to. Nil fails closed.
	ImportBundle func(ctx context.Context, bundle *onboardingDeviceBundle) (string, error)
}

// onboardingConnectionFact is the local, cache-only record that this
// workstation holds a verified management connection for one Target of one
// installation. It carries the installation identity and the observed issuer
// serial only — never tokens, keys or remote secrets. The remote installation
// remains the authority; this file only proves the local join already happened
// so repeated operations do not re-run onboarding.
type onboardingConnectionFact struct {
	ContractVersion string    `json:"contract_version"`
	InstallationID  string    `json:"installation_id"`
	Target          string    `json:"target"`
	IssuerSerial    string    `json:"issuer_serial"`
	ManagementCAPEM string    `json:"management_ca_pem,omitempty"`
	EstablishedAt   time.Time `json:"established_at"`
}

func onboardingConnectionPath(target string) (string, error) {
	root, err := targetRuntimeStateRootByName(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, onboardingConnectionFileName), nil
}

func loadOnboardingConnection(target string) (onboardingConnectionFact, bool, error) {
	path, err := onboardingConnectionPath(target)
	if err != nil {
		return onboardingConnectionFact{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return onboardingConnectionFact{}, false, nil
	}
	if err != nil {
		return onboardingConnectionFact{}, false, err
	}
	var fact onboardingConnectionFact
	if err := json.Unmarshal(data, &fact); err != nil || fact.ContractVersion != onboardingConnectionContract || fact.Target != target {
		return onboardingConnectionFact{}, false, nil
	}
	return fact, true, nil
}

func saveOnboardingConnection(fact onboardingConnectionFact) error {
	fact.ContractVersion = onboardingConnectionContract
	if fact.EstablishedAt.IsZero() {
		fact.EstablishedAt = time.Now().UTC()
	}
	path, err := onboardingConnectionPath(fact.Target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writePrivateJSON(path, fact)
}

// removeOnboardingConnection removes only the local connection cache. It never
// reaches a remote authority: local cleanup cannot revoke or delete the
// installation (device-lifecycle boundary, ownership rule).
func removeOnboardingConnection(target string) error {
	path, err := onboardingConnectionPath(target)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// workstationIdentityPath is the protected per-user runtime location for
// session-only workstation identity material.
func workstationIdentityPath() (string, error) {
	root, err := deployment.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "workstation", "session-identity.pem"), nil
}

func removeWorkstationIdentity() error {
	path, err := workstationIdentityPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// boundManagementConnectionIdentity observes the live management-connection
// serial of the Target through the single accepted reading resolver
// (coreinstallation.ResolverForTargetSelection, used exactly as
// bindExplicitSelectionToConnection does). It returns the installation ID plus
// the observed issuer serial, or a typed error. Nothing is fabricated and
// nothing is persisted here.
func boundManagementConnectionIdentity(ctx context.Context, target string) (string, error) {
	runtimeRoot, err := targetRuntimeStateRootByName(target)
	if err != nil {
		return "", err
	}
	state, err := coreinstallation.Load(runtimeRoot)
	if err != nil {
		return "", machine.NewError(machine.ErrorConflict,
			fmt.Sprintf("target %q has no verified BaseHarbor installation: %v", target, err),
			"Install a BaseHarbor Core for this Target (baha core install) or select the correct Target, then re-run onboarding.", false)
	}
	selection := coreinstallation.ConnectionSelection{
		InstallationID: state.ID,
		ConnectionID:   "managed-connection",
		TargetID:       target,
	}
	if err := selection.AssertDistinct(); err != nil {
		return "", machine.Wrap(machine.ErrorConflict, err, "The Target, connection and installation are distinct; select a distinct Target.", false)
	}
	issuer, observed, err := coreinstallation.ResolverForTargetSelection(ctx, state, runtimeRoot)
	if err != nil {
		return "", machine.Wrap(machine.ErrorVerificationFailed, err, "The management connection did not verify; onboarding persists nothing.", false)
	}
	serial, err := issuer.BaseSerial(ctx)
	if err != nil {
		return "", machine.Wrap(machine.ErrorVerificationFailed, err, "Reconcile the Core installation and retry onboarding.", true)
	}
	_ = observed
	return state.ID + "|" + serial, nil
}

// trustedLocalConnection reports whether a valid reusable management
// connection for the active Target already exists on this workstation. When it
// does, onboarding continues silently with zero credential questions (UX rule:
// existing authentication success causes no questionnaire). An unverifiable
// fact is reported as not-trusted, never trusted. The adapter is resolved at
// call time so tests can replace it.
func trustedLocalConnection(ctx context.Context, target string) bool {
	fact, found, err := loadOnboardingConnection(target)
	if err != nil || !found || onboardingAdapters.ConnectionBound == nil {
		return false
	}
	bound, err := onboardingAdapters.ConnectionBound(ctx, target)
	return err == nil && bound == fact.InstallationID+"|"+fact.IssuerSerial
}

// discoverOnboardingState reads the current facts through the existing
// authorities. Every fact discovered here is information the wizard is
// forbidden to ask for.
func discoverOnboardingState(ctx context.Context) (onboardingwizard.InstallState, error) {
	var state onboardingwizard.InstallState
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return state, err
	}
	state.Targets = cfg.TargetNames()
	active, activeErr := readPersistedTarget()
	if activeErr != nil {
		return state, activeErr
	}
	if active == "" {
		active = cfg.DefaultTarget
	}
	if active != "" {
		root, rootErr := targetRuntimeStateRootByName(active)
		if rootErr == nil {
			if installed, loadErr := coreinstallation.Load(root); loadErr == nil {
				state.HasInstallation = true
				state.InstallationID = installed.ID
				state.TrustedLocal = trustedLocalConnection(ctx, active)
				if state.TrustedLocal {
					// The silent repeat path belongs to exactly this
					// Target: only its connection fact matters here.
					state.Targets = []string{active}
				}
				// The application registration of the current checkout is
				// discoverable through the existing repository authority;
				// the wizard asks the existing/new question only when this
				// stays false.
				state.ExistingAppFound = repositoryRegistrationDiscoverable()
				return state, nil
			}
		}
	}
	// No installation at the active Target: a genuine installation at any
	// other declared Target means the local installation already exists and
	// onboarding must not re-create any of its services.
	for _, name := range state.Targets {
		if name == active {
			continue
		}
		root, rootErr := targetRuntimeStateRootByName(name)
		if rootErr != nil {
			continue
		}
		if installed, loadErr := coreinstallation.Load(root); loadErr == nil {
			state.HasInstallation = true
			state.InstallationID = installed.ID
			break
		}
	}
	return state, nil
}

// repositoryRegistrationDiscoverable reports whether the current working
// directory is an application repository whose registration already exists.
// It reuses the existing repository-detection authority and never mutates.
func repositoryRegistrationDiscoverable() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	found, err := application.HasRepositoryApplication(cwd)
	return err == nil && found
}

// attachExistingApplication attaches the application that is already
// registered at the installation through the single semantic registration
// path (baha app init / app.adopt). It registers no duplicate services.
func attachExistingApplication(ctx context.Context) error {
	var quiet strings.Builder
	ctx = withLocalInvocation(ctx, "cli")
	return runWithIO(ctx, []string{"app", "init", "--yes"}, &quiet, io.Discard)
}

// importConnectionProfileTransactionally imports one NON-SECRET team
// connection profile strictly through the accepted profile-codec store
// (validate-before-mutate, transactional, explicit fail-closed conflicts).
// The wizard never parses the file itself.
func importConnectionProfileTransactionally(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return machine.NewError(machine.ErrorValidationFailed,
			"the connection profile must be a private regular file with mode 0600",
			"Fix the file permissions or re-receive the profile, then retry.", false)
	}
	root, err := deployment.DataRoot()
	if err != nil {
		return err
	}
	store, err := newConnectionProfileImportStore(filepath.Join(root, "profiles"))
	if err != nil {
		return err
	}
	container, err := readConnectionProfileContainer(path)
	if err != nil {
		return machine.Wrap(machine.ErrorValidationFailed, err, "Correct or re-receive the profile; nothing was imported.", false)
	}
	result, err := store.Import(container)
	if err != nil {
		if errors.Is(err, ma.ErrConflict) {
			return machine.NewError(machine.ErrorConflict,
				fmt.Sprintf("the profile import conflicts with existing state: %v", err),
				"Resolve the conflict explicitly (replace or remove the existing record); onboarding never replaces silently.", false)
		}
		return machine.Wrap(machine.ErrorValidationFailed, err, "Nothing was imported; resolve the reported field and retry.", false)
	}
	if len(result.Conflicts) > 0 {
		return machine.NewError(machine.ErrorConflict,
			fmt.Sprintf("the profile import conflicts with %d existing record identity(ies): the store keeps them unchanged", len(result.Conflicts)),
			"Resolve the conflict explicitly (replace or remove the existing record); onboarding never replaces silently.", false)
	}
	return nil
}

// registerOnboardingConnection persists the verified join fact for one Target
// after the management connection was bound (verify-then-persist order).
func registerOnboardingConnection(ctx context.Context, target string) error {
	bound, err := onboardingAdapters.ConnectionBound(ctx, target)
	if err != nil {
		return err
	}
	installationID, serial, found := strings.Cut(bound, "|")
	if !found || installationID == "" || serial == "" {
		return machine.NewError(machine.ErrorVerificationFailed,
			"the management connection identity did not resolve",
			"Reconcile the Core and retry onboarding.", true)
	}
	return saveOnboardingConnection(onboardingConnectionFact{InstallationID: installationID, Target: target, IssuerSerial: serial})
}

// commitOnboardingDecision executes the single effect point of a reviewed
// decision. It runs at most once and fails closed; the caller persists
// nothing before this function is reached.
func commitOnboardingDecision(ctx context.Context, decision onboardingwizard.Decision, out io.Writer) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	switch decision.Mode {
	case onboardingwizard.ModeRemove:
		// Local-only cleanup: removal of the local connection cache only. It
		// cannot revoke or delete anything remote by construction.
		if err := removeOnboardingConnection(decision.Target); err != nil {
			return err
		}
		fmt.Fprintln(out, "Removed the local connection only. The installation and every enrolled device are unchanged.")
		return nil
	case onboardingwizard.ModeImport:
		if decision.ImportFile != "" {
			if err := onboardingAdapters.ImportProfile(ctx, decision.ImportFile); err != nil {
				return err
			}
			fmt.Fprintln(out, "Imported the non-secret team profile; no secrets were transferred.")
			return nil
		}
		// A device bundle is redeemed through the accepted one-use
		// workstation-enrollment boundary of the device-enrollment slice;
		// onboarding never invents a second path and never carries a private
		// key. The enrollment returns the installation the leaf binds, so the
		// join fact carries a concrete, verified target.
		if onboardingAdapters.ImportBundle == nil {
			return machine.NewError(machine.ErrorVerificationFailed,
				"the workstation enrollment authority is unavailable",
				"Reconcile the Core authority and retry onboarding.", false)
		}
		installationID, err := onboardingAdapters.ImportBundle(ctx, bundleForImportDecision(decision))
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Imported the trusted device bundle; enrolled to installation %q. Ordinary commands need no login while this connection is valid.\n", installationID)
		return nil
	case onboardingwizard.ModeAttach:
		if decision.Target == "" {
			return machine.NewError(machine.ErrorValidationFailed, "attach needs the explicit Target", "Select a Target and retry.", false)
		}
		if err := registerOnboardingConnection(ctx, decision.Target); err != nil {
			return err
		}
		if decision.Persistence == onboardingwizard.PersistenceSessionOnly {
			if err := ensureSessionOnlyWorkstationIdentity(); err != nil {
				return err
			}
			fmt.Fprintln(out, "Kept the workstation identity session-only (owner-only runtime file; remove it with local cleanup).")
		}
		fmt.Fprintf(out, "Attached to installation Target %q. Ordinary commands need no login while this connection is valid.\n", decision.Target)
		return nil
	case onboardingwizard.ModeBootstrap:
		if decision.App == onboardingwizard.AppExisting {
			if err := onboardingAdapters.AppAttach(ctx); err != nil {
				return err
			}
			fmt.Fprintln(out, "Attached the existing application; no duplicate services were installed.")
			return nil
		}
		if decision.Persistence == onboardingwizard.PersistenceSessionOnly {
			if err := ensureSessionOnlyWorkstationIdentity(); err != nil {
				return err
			}
		}
		return nil
	}
	return machine.NewError(machine.ErrorValidationFailed, "unknown onboarding mode", "Run 'baha onboard --help'.", false)
}

// ensureSessionOnlyWorkstationIdentity keeps session-only material in the
// protected per-user runtime directory with owner-only permissions. It never
// writes to Git, portable intent or OpenBao.
func ensureSessionOnlyWorkstationIdentity() error {
	path, err := workstationIdentityPath()
	if err != nil {
		return err
	}
	if _, statErr := os.Lstat(path); statErr == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	template := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "baseharbor-workstation-session"}, URIs: nil}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return err
	}
	material := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
	return writePrivateJSON0600(path, material)
}

func writePrivateJSON0600(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".baha-onboarding-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// enrollmentExchange is the workstation half of the accepted one-use
// workstation-enrollment boundary (internal/workstationenrollment/http): the
// install-time bootstrap plus the single enrollment POST. The private key is
// generated in the workstation process and never crosses the wire; every
// non-2xx, foreign installation or expired window fails closed before any
// local state is written.
type enrollmentExchange struct {
	baseURL        string
	client         *http.Client
	scope          ws.Scope
	certificateTTL time.Duration
}

// onboardingBundleContract names the workstation device-bundle wire contract
// consumed by the import intent. It mirrors the accepted
// contracts/workstationenrollment/v1 record (recipient-bound, short-lived,
// one-use); the bundle's own authority stays with device-enrollment.
const onboardingBundleContract = "baseharbor.workstation-enrollment/v1"

// onboardingDeviceBundle is the received, untrusted-until-validated device
// bundle the import intent consumes. Redeem performs the accepted bootstrap
// exchange; the one-use grant cannot be spent twice.
type onboardingDeviceBundle struct {
	contract       string
	baseURL        string
	client         *http.Client
	scope          ws.Scope
	certificateTTL time.Duration
	// capture observes the one-use credentials the exchange consumed, so a
	// caller can prove the spent grant cannot be replayed.
	capture func(token, nonce string)
}

func (b *onboardingDeviceBundle) redeem(ctx context.Context, csrPEM []byte) (enrollmentEnrollResponse, error) {
	if b.contract != onboardingBundleContract {
		return enrollmentEnrollResponse{}, machine.NewError(machine.ErrorValidationFailed,
			"the device bundle does not carry the accepted workstation-enrollment contract",
			"Re-receive the bundle from the owning installation.", false)
	}
	result, token, nonce, err := enrollmentExchange{baseURL: b.baseURL, client: b.client, scope: b.scope, certificateTTL: b.certificateTTL}.exchange(ctx, csrPEM)
	if err != nil {
		return result, err
	}
	if b.capture != nil {
		b.capture(token, nonce)
	}
	return result, nil
}

type enrollmentBootstrapResponse struct {
	BootstrapID           string    `json:"bootstrap_id"`
	Token                 string    `json:"token"`
	Nonce                 string    `json:"nonce"`
	ExpiresAt             time.Time `json:"expires_at"`
	CertificateTTLSeconds int       `json:"certificate_ttl_seconds"`
}

type enrollmentEnrollResponse struct {
	InstallationID string    `json:"installation_id"`
	Station        string    `json:"station"`
	Serial         string    `json:"serial"`
	ExpiresAt      time.Time `json:"expires_at"`
	LeafPEM        string    `json:"leaf_pem"`
	TrustPEM       string    `json:"trust_pem"`
	At             time.Time `json:"at"`
}

// postJSON sends one request and decodes at most limit bytes.
func onboardingPostJSON(ctx context.Context, client *http.Client, url string, body any, limit int64) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorProviderUnavailable, err, "Verify Core HTTPS reachability and trust.", true)
	}
	if resp.Body == nil {
		return resp, nil
	}
	resp.Body = io.NopCloser(io.LimitReader(resp.Body, limit))
	return resp, nil
}

// perform exchanges the one-use credentials for the workstation leaf exactly
// once: the install-time bootstrap and the single enrollment POST. It carries
// the workstation-owned CSR and returns the signed leaf and public trust.
func (e enrollmentExchange) perform(ctx context.Context, csrPEM []byte) (enrollmentEnrollResponse, error) {
	result, _, _, err := e.exchange(ctx, csrPEM)
	return result, err
}

// exchange performs the bootstrap+enroll pair and additionally reports the
// consumed one-use credentials so callers can prove replay fails closed.
func (e enrollmentExchange) exchange(ctx context.Context, csrPEM []byte) (enrollmentEnrollResponse, string, string, error) {
	var zero enrollmentEnrollResponse
	base := strings.TrimRight(e.baseURL, "/")
	client := e.client
	if client == nil {
		client = http.DefaultClient
	}
	reqBody := map[string]any{
		"installation_id":         e.scope.TenantID,
		"station":                 e.scope.Station,
		"certificate_ttl_seconds": int(e.certificateTTL / time.Second),
	}
	resp, err := onboardingPostJSON(ctx, client, base+"/api/v1/workstations/bootstrap", reqBody, 16384)
	if err != nil {
		return zero, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zero, "", "", machine.NewError(machine.ErrorVerificationFailed,
			fmt.Sprintf("Core refused the workstation bootstrap (HTTP %d)", resp.StatusCode),
			"Confirm the enrollment bundle is unexpired and unused, then retry.", false)
	}
	var bootstrap enrollmentBootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&bootstrap); err != nil {
		return zero, "", "", errors.New("Core returned an invalid workstation bootstrap response")
	}
	if bootstrap.Token == "" || bootstrap.Nonce == "" {
		return zero, "", "", errors.New("Core returned an incomplete workstation bootstrap response")
	}
	resp2, err := onboardingPostJSON(ctx, client, base+"/api/v1/workstations/enroll", map[string]any{
		"installation_id": e.scope.TenantID,
		"station":         e.scope.Station,
		"token":           bootstrap.Token,
		"nonce":           bootstrap.Nonce,
		"csr_pem":         string(csrPEM),
	}, 65536)
	if err != nil {
		return zero, "", "", err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return zero, "", "", machine.NewError(machine.ErrorVerificationFailed,
			fmt.Sprintf("Core refused the workstation enrollment (HTTP %d)", resp2.StatusCode),
			"The one-use bootstrap is consumed or invalid; request a fresh bundle.", false)
	}
	var result enrollmentEnrollResponse
	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		return zero, "", "", errors.New("Core returned an invalid workstation enrollment response")
	}
	if result.InstallationID != e.scope.TenantID || result.LeafPEM == "" || !result.ExpiresAt.After(time.Now()) {
		return zero, "", "", errors.New("Core returned workstation identity material that does not bind this installation")
	}
	return result, bootstrap.Token, bootstrap.Nonce, nil
}
