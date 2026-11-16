package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalRemovalRejectsWritableParentOwnership(t *testing.T) {
	for _, relative := range []string{"targets", "targets/dev"} {
		t.Run(relative, func(t *testing.T) {
			root, _ := externalScopeFixture(t)
			if err := os.Chmod(filepath.Join(root, relative), 0770); err != nil {
				t.Fatal(err)
			}
			if _, err := PlanExternalProviderRemovalAt(root, "company-db"); err == nil {
				t.Fatal("group-writable registry parent granted provider-removal authority")
			}
		})
	}
}
