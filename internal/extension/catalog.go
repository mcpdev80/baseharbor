package extension

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Catalog struct {
	entries map[string][]Metadata
}

func NewCatalog(entries ...Metadata) (Catalog, error) {
	catalog := Catalog{entries: map[string][]Metadata{}}
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return Catalog{}, err
		}
		list := catalog.entries[entry.ID]
		for _, current := range list {
			if current.Version == entry.Version {
				return Catalog{}, fmt.Errorf("extension %s version %s is registered more than once", entry.ID, entry.Version)
			}
		}
		catalog.entries[entry.ID] = append(list, entry)
	}
	for id := range catalog.entries {
		sort.Slice(catalog.entries[id], func(i, j int) bool {
			return catalog.entries[id][i].Version < catalog.entries[id][j].Version
		})
	}
	return catalog, nil
}

type ResolveRequest struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

type Resolution struct {
	Metadata     Metadata      `json:"metadata"`
	Trusted      bool          `json:"trusted"`
	Verification Verification  `json:"verification"`
	Decision     TrustDecision `json:"trust_decision"`
}

type TrustPolicy struct {
	RequireDigest       bool     `json:"require_digest,omitempty"`
	RequireSignature    bool     `json:"require_signature,omitempty"`
	RequireSBOM         bool     `json:"require_sbom,omitempty"`
	RequireAttestation  bool     `json:"require_attestation,omitempty"`
	RequireVerification bool     `json:"require_verification,omitempty"`
	AllowedPublishers   []string `json:"allowed_publishers,omitempty"`
	Reference           string   `json:"reference,omitempty"`
}

func (c Catalog) Resolve(request ResolveRequest, policy TrustPolicy) (Resolution, error) {
	return c.ResolveVerified(context.Background(), request, policy, nil)
}

// ResolveVerified keeps descriptor identity, verifier evidence and policy separate.
// A nil verifier produces an explicit unverifiable result, never a verified one.
func (c Catalog) ResolveVerified(ctx context.Context, request ResolveRequest, policy TrustPolicy, verifier Verifier) (Resolution, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return Resolution{}, fmt.Errorf("extension resolution requires id")
	}
	candidates := append([]Metadata(nil), c.entries[id]...)
	if len(candidates) == 0 {
		return Resolution{}, fmt.Errorf("extension %q was not found", id)
	}
	version := strings.TrimSpace(request.Version)
	if version != "" {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if candidate.Version == version {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	digest := strings.TrimSpace(request.Digest)
	if digest != "" {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if candidate.Artifact.Digest == digest {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return Resolution{}, fmt.Errorf("extension %q has no candidate matching requested version/digest", id)
	}
	if len(candidates) > 1 {
		return Resolution{}, fmt.Errorf("extension %q resolution is ambiguous; specify version or immutable digest", id)
	}
	selected := candidates[0]
	verification := VerifyArtifact(ctx, selected, verifier)
	decision := policy.Evaluate(selected, verification)
	result := Resolution{Metadata: selected, Verification: verification, Decision: decision, Trusted: decision.Status == TrustTrusted}
	if decision.Status != TrustTrusted {
		return result, &TrustError{Code: decision.Code, Next: decision.Next}
	}
	return result, nil
}

func (p TrustPolicy) Validate(metadata Metadata) error {
	if p.RequireDigest && strings.TrimSpace(metadata.Artifact.Digest) == "" {
		return fmt.Errorf("extension %q is rejected by trust policy: immutable artifact digest is required", metadata.ID)
	}
	if p.RequireSignature && strings.TrimSpace(metadata.Provenance.SignatureRef) == "" {
		return fmt.Errorf("extension %q is rejected by trust policy: signature reference is required", metadata.ID)
	}
	if p.RequireSBOM && strings.TrimSpace(metadata.Provenance.SBOMRef) == "" {
		return fmt.Errorf("extension %q is rejected by trust policy: SBOM reference is required", metadata.ID)
	}
	if p.RequireAttestation && strings.TrimSpace(metadata.Provenance.AttestationRef) == "" {
		return fmt.Errorf("extension %q is rejected by trust policy: attestation reference is required", metadata.ID)
	}
	return nil
}
