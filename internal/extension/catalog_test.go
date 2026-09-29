package extension

import "testing"

func TestCatalogResolutionRequiresDeterminism(t *testing.T) {
	a := Metadata{
		SchemaVersion: DescriptorVersion, ID: "example/adapter", Family: FamilyDevelopment, Version: "1.0.0",
		Artifact: Artifact{OCIReference: "ghcr.io/example/adapter:1.0.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	b := a
	b.Version = "2.0.0"
	b.Artifact = Artifact{OCIReference: "ghcr.io/example/adapter:2.0.0", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	catalog, err := NewCatalog(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Resolve(ResolveRequest{ID: a.ID}, TrustPolicy{}); err == nil {
		t.Fatal("expected ambiguous resolution without version/digest")
	}
	resolution, err := catalog.Resolve(ResolveRequest{ID: a.ID, Digest: b.Artifact.Digest}, TrustPolicy{RequireDigest: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Metadata.Version != "2.0.0" {
		t.Fatalf("resolved version = %s", resolution.Metadata.Version)
	}
}

func TestTrustPolicyFailsClosed(t *testing.T) {
	entry := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/provider",
		Family:        FamilyProvider,
		Version:       "1.0.0",
		Artifact:      Artifact{OCIReference: "ghcr.io/example/provider:1.0.0"},
	}
	catalog, err := NewCatalog(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Resolve(ResolveRequest{ID: entry.ID, Version: entry.Version}, TrustPolicy{RequireDigest: true}); err == nil {
		t.Fatal("expected digest trust policy rejection")
	}
}

func TestTrustPolicyAcceptsProvenanceReferences(t *testing.T) {
	entry := Metadata{
		SchemaVersion: DescriptorVersion,
		ID:            "example/provider",
		Family:        FamilyProvider,
		Version:       "1.0.0",
		Artifact: Artifact{
			OCIReference: "ghcr.io/example/provider:1.0.0",
			Digest:       "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
		Provenance: Provenance{
			SignatureRef:   "oci://signature",
			SBOMRef:        "oci://sbom",
			AttestationRef: "oci://attestation",
		},
	}
	catalog, err := NewCatalog(entry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.Resolve(ResolveRequest{ID: entry.ID, Version: entry.Version}, TrustPolicy{
		RequireDigest: true, RequireSignature: true, RequireSBOM: true, RequireAttestation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}
