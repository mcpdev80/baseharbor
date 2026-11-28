package main

import (
	"strings"
	"sync"
)

// interactiveCandidateConsentBinding records the candidate an operator bound
// with the interactive prompt of THIS apply invocation (0428-292). The prompt
// runs deeper in the lifecycle than the context can propagate back, so the
// confirmation is held here, keyed by the exact deployment identity the write
// boundary re-derives the submission from. Invocation scoping is threefold
// (CODEX re-review finding): every read path that matches the identity
// CONSUMES the binding exactly once — including a write attempt that resolves
// an explicit --confirm-candidate instead — a write attempt for a DIFFERENT
// identity clears it, and every apply lifecycle entry and exit clears any
// residue unconditionally. A confirmation can therefore never authorize a
// later, unrelated apply. An empty key or digest means "no interactive
// binding".
type interactiveCandidateConsentEntry struct {
	applicationID string
	targetName    string
	digest        string
}

var interactiveCandidateConsent struct {
	mu      sync.Mutex
	binding *interactiveCandidateConsentEntry
}

func setInteractiveCandidateConsentBinding(applicationID, targetName, digest string) {
	if applicationID == "" || targetName == "" || digest == "" {
		return
	}
	interactiveCandidateConsent.mu.Lock()
	defer interactiveCandidateConsent.mu.Unlock()
	interactiveCandidateConsent.binding = &interactiveCandidateConsentEntry{
		applicationID: applicationID, targetName: targetName, digest: digest,
	}
}

// takeInteractiveCandidateConsentDigest consumes an interactive binding for the
// given deployment identity and returns its confirm digest. A matching
// identity retires the binding so it is used at most once. Any OTHER identity
// also retires a pending binding (a write attempt for a different application
// invalidates a confirmation the operator never gave for it, and it can never
// survive for the original identity either) and returns an empty string, so
// the persisted fail-closed contract stays unchanged.
func takeInteractiveCandidateConsentDigest(applicationID, targetName string) string {
	interactiveCandidateConsent.mu.Lock()
	defer interactiveCandidateConsent.mu.Unlock()
	binding := interactiveCandidateConsent.binding
	if binding == nil {
		return ""
	}
	interactiveCandidateConsent.binding = nil
	if binding.applicationID != applicationID || binding.targetName != targetName {
		return ""
	}
	return binding.digest
}

// clearInteractiveCandidateConsentBinding drops any unconsumed interactive
// binding, used by lifecycle exits so a refusal or failure never lingers for a
// later invocation.
func clearInteractiveCandidateConsentBinding() {
	interactiveCandidateConsent.mu.Lock()
	defer interactiveCandidateConsent.mu.Unlock()
	interactiveCandidateConsent.binding = nil
}

// candidateConsentForWrite resolves the confirm digest the write boundary
// binds: the explicit argument first — which also CONSUMES any pending
// interactive binding of this invocation, so an explicit flag and the inline
// prompt can never stack into consent for a later invocation — otherwise a
// one-shot interactive binding for this exact deployment identity (the
// interactive bind prompt of this same invocation). It never fabricates a
// digest: finalizeCandidateConsent still recomputes the submission from the
// CURRENT source and constant-time-compares, so a binding for a source that
// changed after review fails closed, and an unbound write stays fail closed
// exactly as before.
func candidateConsentForWrite(applicationID, targetName, raw string) string {
	if explicit := strings.TrimSpace(raw); explicit != "" {
		// Retire any pending interactive binding regardless of identity: the
		// operator's confirmation for THIS invocation is now resolved by the
		// explicit flag, and nothing may linger for a later same-identity run
		// that never answered a prompt (CODEX re-review finding).
		takeInteractiveCandidateConsentDigest(applicationID, targetName)
		clearInteractiveCandidateConsentBinding()
		return explicit
	}
	return takeInteractiveCandidateConsentDigest(applicationID, targetName)
}
