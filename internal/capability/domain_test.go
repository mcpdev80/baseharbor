package capability

import "testing"

func TestResolveNegotiatesSupportedCapability(t *testing.T) {
	resource, err := Resolve("mailflow", Requirement{Kind: SQL, Name: "primary"}, PostgreSQL)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resource.Application != "mailflow" || resource.Kind != SQL || resource.Name != "primary" || resource.Provider != ProviderPostgreSQL {
		t.Fatalf("Resolve() resource = %#v", resource)
	}
}

func TestResolveFailsClosedForUnsupportedProviderCapability(t *testing.T) {
	_, err := Resolve("mailflow", Requirement{Kind: SQL, Name: "primary"}, Valkey)
	if err == nil {
		t.Fatal("Resolve() error = nil, want unsupported capability error")
	}
}

func TestResolveRequiresStableLogicalIdentity(t *testing.T) {
	tests := []struct {
		name        string
		application string
		requirement Requirement
		provider    Provider
	}{
		{
			name:        "missing application",
			requirement: Requirement{Kind: SQL, Name: "primary"},
			provider:    PostgreSQL,
		},
		{
			name:        "missing capability",
			application: "mailflow",
			requirement: Requirement{Name: "primary"},
			provider:    PostgreSQL,
		},
		{
			name:        "missing resource name",
			application: "mailflow",
			requirement: Requirement{Kind: SQL},
			provider:    PostgreSQL,
		},
		{
			name:        "missing provider",
			application: "mailflow",
			requirement: Requirement{Kind: SQL, Name: "primary"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Resolve(tt.application, tt.requirement, tt.provider); err == nil {
				t.Fatal("Resolve() error = nil, want validation error")
			}
		})
	}
}

func TestReferenceProvidersExposeOnlyTheirCurrentCapability(t *testing.T) {
	if !PostgreSQL.Supports(SQL) || PostgreSQL.Supports(KeyValue) {
		t.Fatalf("PostgreSQL capabilities = %#v", PostgreSQL.Capabilities)
	}
	if !Valkey.Supports(KeyValue) || Valkey.Supports(SQL) {
		t.Fatalf("Valkey capabilities = %#v", Valkey.Capabilities)
	}
	if !OpenBao.Supports(Secrets) || OpenBao.Supports(SQL) {
		t.Fatalf("OpenBao capabilities = %#v", OpenBao.Capabilities)
	}
	if !Caddy.Supports(ExposureHTTP) || Caddy.Supports(SQL) {
		t.Fatalf("Caddy capabilities = %#v", Caddy.Capabilities)
	}
}
