package providertopology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompletedPrerequisitesUseDeclarationsWithoutNameGuessing(t *testing.T) {
	for _, test := range []struct {
		name, restart, condition string
		want                     bool
		bad                      bool
	}{
		{"declared", "no", "service_completed_successfully", true, false},
		{"running-init-name", "unless-stopped", "service_started", false, false},
		{"unused-one-shot", "no", "service_started", false, false},
		{"inconsistent", "unless-stopped", "service_completed_successfully", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "compose.yaml")
			source := "services:\n  arbitrary-helper:\n    restart: \"" + test.restart + "\"\n  native-provider:\n    depends_on:\n      arbitrary-helper:\n        condition: " + test.condition + "\n"
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			services, err := CompletedPrerequisiteServices(path)
			if test.bad {
				if err == nil {
					t.Fatal("inconsistent role declaration accepted")
				}
				return
			}
			if err != nil || (len(services) == 1) != test.want {
				t.Fatal("role inferred from name or restart alone", err)
			}
		})
	}
}
