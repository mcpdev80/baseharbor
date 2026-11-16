package applicationbackup

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDocumentDatabaseRecoveryClassificationIsExplicit(t *testing.T) {
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "catalog",
		Environment:   "dev",
		Services: application.Services{
			DocumentDatabase:          true,
			DocumentDatabaseInstances: map[string]application.ServiceInstance{"documents": {}},
		},
	}
	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selection.Contributors {
		if c.StateClass != StateDocumentDatabase {
			continue
		}
		if c.LogicalResource != "documents" || c.Support != RecoveryUnsupported || !c.Durable || c.Selected {
			t.Fatalf("document database recovery contributor = %#v", c)
		}
		return
	}
	t.Fatal("database.document recovery contributor missing")
}
