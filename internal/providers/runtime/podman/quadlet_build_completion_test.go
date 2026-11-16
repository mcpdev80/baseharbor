package podman

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestQuadletBuildWaitsForCompletionAndPropagatesFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			marker := filepath.Join(dir, "completed")
			script := "#!/bin/sh\nfor arg; do if [ \"$arg\" = --no-block ]; then exit 0; fi; done\n"
			if fail {
				script += "exit 1\n"
			} else {
				script += "touch '" + marker + "'\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			project := QuadletProject{Project: "demo", Files: map[string]string{"demo-app.build": "[Build]\nImageTag=localhost/demo-app:quadlet\n"}}
			unitDir, err := quadletUserUnitDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteQuadletProject(unitDir, project); err != nil {
				t.Fatal(err)
			}
			err = quadletBuildProject(context.Background(), project, []string{"app"})
			if fail {
				if err == nil {
					t.Fatal("build failure reported as success")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("build returned before completion")
			}
		})
	}
}
