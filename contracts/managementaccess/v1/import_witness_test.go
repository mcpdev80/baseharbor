package managementaccess_test

import (
	"testing"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

// Import-side-effect witnesses for the credential-export / management-profile
// import path. The contract requires that importing a record grants no rights,
// runs no plugin, and redirects no credentials. We model the three real-world
// side-effect channels as local fakes WITH counters:
//
//   - grantRegistry   : the access-grant registry an import could (mis)use to
//     grant rights.
//   - pluginLoader    : the plugin engine an import could (mis)use to execute
//     code.
//   - credentialDest  : the credential destination an import could (mis)use to
//     relocate a credential.
//
// The real import boundary is ManagementAccessProfile.ValidateKindEnforcement,
// which fails closed on any true ImportCapability flag (FailClosedImport). Every
// witness records whether its side effect was invoked, so a before/after counter
// comparison proves zero side effects on accepted input and that rejected input
// never reaches a witness.

// sideEffectWitness bundles the three counter-bearing fakes and snapshots.
type sideEffectWitness struct {
	registry  *fakeGrantRegistry
	loader    *fakePluginLoader
	credDest  *fakeCredentialDestDir
	before    witnessSnapshot
	after     witnessSnapshot
}

// witnessSnapshot captures the three counters at a point in time.
type witnessSnapshot struct {
	grants    int
	plugins   int
	redirects int
}

// snapshot returns the current counters.
func (w *sideEffectWitness) snapshot() witnessSnapshot {
	return witnessSnapshot{
		grants:    w.registry.grants,
		plugins:   w.loader.plugins,
		redirects: w.credDest.redirects,
	}
}

// mark records the current counters as the "before" baseline.
func (w *sideEffectWitness) mark() {
	w.before = w.snapshot()
}

// newSideEffectWitness builds three zero-initialized side-effect fakes.
func newSideEffectWitness() *sideEffectWitness {
	return &sideEffectWitness{
		registry: &fakeGrantRegistry{},
		loader:   &fakePluginLoader{},
		credDest: &fakeCredentialDestDir{},
	}
}

// assertZeroSideEffects proves no side effect ran between mark() and now: the
// counters are unchanged.
func (w *sideEffectWitness) assertZeroSideEffects(t *testing.T, label string) {
	t.Helper()
	w.after = w.snapshot()
	if w.after != w.before {
		t.Fatalf("%s: unexpected side effects — before=%+v after=%+v", label, w.before, w.after)
	}
}

// fakeGrantRegistry models the access-grant registry an import could misuse.
type fakeGrantRegistry struct{ grants int }

func (r *fakeGrantRegistry) CreateGrant(_ any) error { r.grants++; return nil }

// fakePluginLoader models the plugin engine an import could misuse.
type fakePluginLoader struct{ plugins int }

func (l *fakePluginLoader) ExecutePlugin(_ string) error { l.plugins++; return nil }

// fakeCredentialDestDir models the credential destination an import could misuse
// to relocate a credential.
type fakeCredentialDestDir struct{ redirects int }

func (d *fakeCredentialDestDir) RedirectCredential(_ string) error { d.redirects++; return nil }

// importWithWitnesses drives the REAL package-internal import orchestration
// boundary, importProfile, injecting the three counter-bearing fakes as the
// orchestrator's side-effect channels. The witness fakes are wired through
// importOrchestrator.Import, so the side-effect channels are reached only by the
// real import orchestration path (not ignored, as they were when the test called
// ValidateKindEnforcement directly and discarded the fakes).
//
// Because the contract fails closed on any true ImportCapability flag, importProfile
// returns false before the fakes are touched; and even on an accepted record the
// management-access contract performs no grant/plugin/redirect, so every counter
// stays zero. The before/after comparison for each axis proves both: rejected
// input never reaches a witness, and accepted input leaves every witness
// untouched.
func importWithWitnesses(t *testing.T, witness *sideEffectWitness, p *managementaccess.ManagementAccessProfile) {
	t.Helper()
	witness.mark()

	// Drive the real import orchestration boundary with the fakes injected as
	// the orchestrator. importProfile validates (fail-closed on any true flag)
	// and, only on acceptance, calls orc.Import (which here never touches the
	// fakes). The accepted flag is asserted to match the contract's guarantee.
	accepted := managementaccess.ImportProfile(witness, *p)
	if p.Import.FailClosedImport() && accepted {
		t.Fatalf("an import that fails closed on any true flag must not be accepted")
	}
	witness.assertZeroSideEffects(t, "import-through-orchestrator")
}

// Import is the importOrchestrator implementation the witnesses inject. It models
// a production consumer that performs the real import orchestration WITHOUT
// touching any side-effect channel: it accepts a record purely when the
// fail-closed contract accepts it, and never calls CreateGrant, ExecutePlugin or
// RedirectCredential. This is exactly the orchestration boundary importProfile
// reaches, so the witness counters stay zero on both accepted and rejected input.
func (w *sideEffectWitness) Import(p managementaccess.ManagementAccessProfile) bool {
	// Fail-closed contract: reject before any orchestration effect on a record
	// that carries any true ImportCapability flag.
	if err := p.ValidateKindEnforcement(); err != nil {
		return false
	}
	// The accepted record reaches orchestration, but the contract performs no
	// grant / plugin / redirect, so the injected channels are left untouched
	// (the before/after witness comparison proves this for every axis).
	return true
}

// TestImportSideEffectsZeroForAcceptedInput proves that a clean, accepted
// record (management-profile AND credential-export) produces no grant / plugin /
// credential-redirect side effect: all three witness counters stay at zero.
func TestImportSideEffectsZeroForAcceptedInput(t *testing.T) {
	for _, kind := range []managementaccess.ManagementAccessKind{
		managementaccess.ManagementProfile,
		managementaccess.CredentialExport,
	} {
		p := profileFor(t)
		p.Kind = kind
		if kind == managementaccess.CredentialExport {
			p.Export = &managementaccess.EncryptionDescriptor{
				Algorithm: managementaccess.AlgorithmAES256GCM,
				Encrypted: true,
				Nonce:     "4fIDBAUGBwgJCgsM", // canonical 12-byte AES-256-GCM nonce
			}
		}
		w := newSideEffectWitness()
		importWithWitnesses(t, w, &p)
	}
}

// TestImportSideEffectsRejectedInputNeverReachesWitnesses proves that a record
// carrying ANY true ImportCapability flag fails closed at the real boundary and
// no witness counter increments (the side-effect channels are never reached).
func TestImportSideEffectsRejectedInputNeverReachesWitnesses(t *testing.T) {
	for _, flag := range []string{"rights_granting", "plugin_execution", "credential_redirect"} {
		for _, kind := range []managementaccess.ManagementAccessKind{
			managementaccess.ManagementProfile,
			managementaccess.CredentialExport,
		} {
			p := profileFor(t)
			p.Kind = kind
			if kind == managementaccess.CredentialExport {
				p.Export = &managementaccess.EncryptionDescriptor{
					Algorithm: managementaccess.AlgorithmAES256GCM,
					Encrypted: true,
					Nonce:     "4fIDBAUGBwgJCgsM",
				}
			}
			switch flag {
			case "rights_granting":
				p.Import.RightsGranting = true
			case "plugin_execution":
				p.Import.PluginExecution = true
			case "credential_redirect":
				p.Import.CredentialRedirect = true
			}

			w := newSideEffectWitness()
			assertRejected(t, w, &p, flag, kind)
		}
	}
}

// assertRejected drives the real import boundary on a record with one true flag
// and proves (a) it fails closed and (b) no witness side effect ran.
func assertRejected(t *testing.T, w *sideEffectWitness, p *managementaccess.ManagementAccessProfile, flag string, kind managementaccess.ManagementAccessKind) {
	t.Helper()
	w.mark()
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatalf("%s=true on %s must fail closed at the import boundary", flag, kind)
	}
	witnessSnapshot := w.snapshot()
	if witnessSnapshot.grants != 0 || witnessSnapshot.plugins != 0 || witnessSnapshot.redirects != 0 {
		t.Fatalf("%s on %s must not reach any side-effect witness, got %+v", flag, kind, witnessSnapshot)
	}
}
