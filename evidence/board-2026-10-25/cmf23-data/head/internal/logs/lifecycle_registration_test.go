package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestApplicationRegistrationTeardownReadOnly(t *testing.T) {
	for _, mode := range []string{"absent", "own", "other", "corrupt", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			m := application.New("demo", "dev", true, false, false)
			p, err := PlacementForAt(root, "fixture", m)
			if err != nil {
				t.Fatal(err)
			}
			files := providerFiles(p)
			if mode != "absent" {
				if err := os.MkdirAll(files.Dir, 0700); err != nil {
					t.Fatal(err)
				}
				if mode == "incomplete" {
					if err := os.WriteFile(files.Compose, []byte("retained"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					registration := Registration{Application: "demo", Environment: "dev", SyslogPort: 15140, ProviderSyslogPort: 15141}
					if mode == "other" {
						registration.Application = "foreign"
					}
					data, err := json.Marshal([]Registration{registration})
					if err != nil {
						t.Fatal(err)
					}
					if mode == "corrupt" {
						data = []byte("invalid")
					}
					if err := os.WriteFile(files.Registrations, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, _ := os.ReadFile(files.Registrations)
			found, err := ApplicationRegisteredAt(root, "fixture", m)
			if found != (mode == "own") || (err != nil) != (mode == "corrupt" || mode == "incomplete") {
				t.Fatalf("found=%v err=%v", found, err)
			}
			after, _ := os.ReadFile(files.Registrations)
			if string(before) != string(after) {
				t.Fatal("registration mutated")
			}
			if mode == "absent" {
				if _, err := os.Stat(filepath.Dir(files.Registrations)); !os.IsNotExist(err) {
					t.Fatal("provider state materialized")
				}
			}
		})
	}
}
