package capability

import "testing"

func TestDocumentDatabaseSpecificationAndMongoDBReferenceProvider(t *testing.T) {
	spec, err := ParseSpecificationID(DocumentDatabaseV1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != DocumentDatabase {
		t.Fatalf("kind = %q", spec.Kind)
	}
	service, err := ServiceKindForCapability(DocumentDatabase)
	if err != nil {
		t.Fatal(err)
	}
	if service != ServiceDocument {
		t.Fatalf("service = %q", service)
	}
	if !MongoDB.Supports(DocumentDatabase) {
		t.Fatal("MongoDB does not support database.document")
	}
	integration, err := ReferenceIntegration(ProviderMongoDB)
	if err != nil {
		t.Fatal(err)
	}
	if integration.Provider.Kind != ProviderMongoDB {
		t.Fatalf("provider = %q", integration.Provider.Kind)
	}
	if len(integration.SupportedScopes) != 1 || integration.SupportedScopes[0] != ScopeApplication {
		t.Fatalf("scopes = %#v", integration.SupportedScopes)
	}
	if len(integration.Interfaces) != 1 || integration.Interfaces[0].Protocol != "mongodb" {
		t.Fatalf("interfaces = %#v", integration.Interfaces)
	}
}
