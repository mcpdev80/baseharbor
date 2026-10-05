package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
)

// ArtifactReader resolves immutable OCI manifest bytes and public evidence
// references. Authentication belongs to the operator's transport, never metadata.
// The verifier always recomputes the manifest digest before trusting evidence.
type ArtifactReader interface {
	Open(context.Context, string) (io.ReadCloser, error)
}
type PublisherKey struct {
	Publisher string
	PublicKey any
}

// JWSVerifier supports asymmetric RFC 7515 signatures over in-toto v1
// statements. Keys and publisher identities are supplied by the operator.
// Neither embedded keys nor remote key URLs grant publisher trust.
type JWSVerifier struct {
	Reader ArtifactReader
	Keys   map[string]PublisherKey
}

const (
	InTotoStatementV1    = "https://in-toto.io/Statement/v1"
	SLSAProvenanceV1     = "https://slsa.dev/provenance/v1"
	SPDXPredicateV2      = "https://spdx.dev/Document"
	CycloneDXPredicateV1 = "https://cyclonedx.org/bom"
)

type artifactStatement struct {
	Type    string `json:"_type"`
	Subject []struct {
		Name   string            `json:"name"`
		Digest map[string]string `json:"digest"`
	} `json:"subject"`
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

func (v JWSVerifier) Verify(ctx context.Context, metadata Metadata) (Verification, error) {
	invalid := Verification{Status: VerificationInvalid}
	if v.Reader == nil {
		return Verification{}, errors.New("artifact reader unavailable")
	}
	if metadata.Validate() != nil || metadata.Artifact.Digest == "" {
		return invalid, nil
	}
	// Always request the declared digest, even when the descriptor uses a tag.
	ref := metadata.Artifact.OCIReference
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	ref += "@" + metadata.Artifact.Digest
	content, err := v.read(ctx, ref, 32<<20)
	if err != nil {
		return Verification{}, err
	}
	sum := sha256.Sum256(content)
	if "sha256:"+hex.EncodeToString(sum[:]) != metadata.Artifact.Digest {
		return invalid, nil
	}
	result := Verification{Status: VerificationVerified, Digest: metadata.Artifact.Digest}
	evidence := []struct{ ref, kind string }{
		{metadata.Provenance.SignatureRef, "signature"},
		{metadata.Provenance.SBOMRef, "sbom"},
		{metadata.Provenance.AttestationRef, "attestation"},
	}
	for _, item := range evidence {
		if item.ref == "" {
			continue
		}
		data, err := v.read(ctx, item.ref, 4<<20)
		if err != nil {
			return Verification{}, err
		}
		publisher, valid := v.statement(data, metadata, item.kind)
		if !valid || result.Publisher != "" && result.Publisher != publisher {
			return invalid, nil
		}
		result.Publisher = publisher
		switch item.kind {
		case "signature":
			result.SignatureVerified = true
		case "sbom":
			result.SBOMVerified = true
		case "attestation":
			result.AttestationVerified = true
		}
	}
	return result, nil
}

func (v JWSVerifier) read(ctx context.Context, ref string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := v.Reader.Open(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("artifact or evidence exceeds verification bounds")
	}
	return data, nil
}

func (v JWSVerifier) statement(data []byte, metadata Metadata, kind string) (string, bool) {
	signed, err := jose.ParseSigned(string(data), []jose.SignatureAlgorithm{jose.EdDSA, jose.ES256, jose.RS256, jose.PS256})
	if err != nil || len(signed.Signatures) != 1 {
		return "", false
	}
	key, ok := v.Keys[signed.Signatures[0].Protected.KeyID]
	if !ok || strings.TrimSpace(key.Publisher) == "" {
		return "", false
	}
	payload, err := signed.Verify(key.PublicKey)
	if err != nil {
		return "", false
	}
	var statement artifactStatement
	if json.Unmarshal(payload, &statement) != nil || statement.Type != InTotoStatementV1 || statement.PredicateType == "" {
		return "", false
	}
	found := false
	for _, subject := range statement.Subject {
		if subject.Digest["sha256"] == strings.TrimPrefix(metadata.Artifact.Digest, "sha256:") {
			found = true
		}
	}
	if !found {
		return "", false
	}
	switch kind {
	case "sbom":
		if statement.PredicateType != SPDXPredicateV2 && statement.PredicateType != CycloneDXPredicateV1 {
			return "", false
		}
		if len(statement.Predicate) == 0 || string(statement.Predicate) == "null" {
			return "", false
		}
	case "attestation":
		if statement.PredicateType != SLSAProvenanceV1 || len(statement.Predicate) == 0 || string(statement.Predicate) == "null" {
			return "", false
		}
	}
	return key.Publisher, true
}
