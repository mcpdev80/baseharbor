package main

import (
	"strings"
	"sync"
)

// interactiveCandidateConsentBinding records the candidate an operator bound
// with the interactive prompt of THIS apply invocation (0428-292). The prompt
// runs deeper in the lifecycle than the context can propagate back, so the
// confirmation is held here, keyed by the exact deployment identity the write
// boundary re-derives the submission from. Every lifecycle mutation runs under
// the Target lifecycle lock, and the binding is consumed (cleared) by the very
// next candidate write attempt, so a confirmation can never authorize a later,
// unrelated apply. An empty key or digest means "no interactive binding".
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

// takeInteractiveCandidateConsentDigest consumes (exactly once) an interactive
// binding for the given deployment identity and returns its confirm digest.
// Any other identity — or no pending binding — returns an empty string and
// leaves the persisted fail-closed contract unchanged.
func takeInteractiveCandidateConsentDigest(applicationID, targetName string) string {
	interactiveCandidateConsent.mu.Lock()
	defer interactiveCandidateConsent.mu.Unlock()
	binding := interactiveCandidateConsent.binding
	if binding == nil {
		return ""
	}
	if binding.applicationID != applicationID || binding.targetName != targetName {
		return ""
	}
	interactiveCandidateConsent.binding = nil
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
// binds: the explicit argument first, then a one-shot interactive binding for
// this exact deployment identity (the interactive bind prompt of this same
// invocation). It never fabricates a digest: finalizeCandidateConsent still
// recomputes the submission from the CURRENT source and constant-time-compares,
// so a binding for a source that changed after review fails closed, and an
// unbound write stays fail closed exactly as before.
func candidateConsentForWrite(applicationID, targetName, raw string) string {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return takeInteractiveCandidateConsentDigest(applicationID, targetName)
}
