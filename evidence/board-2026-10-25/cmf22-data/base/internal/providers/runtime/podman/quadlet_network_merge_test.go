package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuadletMergePreservesAuthoredReceiverNetwork(t *testing.T) {
	for _, baseNetworks := range []string{"[receiver]", "\n      receiver: {}"} {
		for _, extraNetworks := range []string{"[managed]", "\n      managed: {}"} {
			t.Run(baseNetworks+"/"+extraNetworks, func(t *testing.T) {
				root := t.TempDir()
				base := "services:\n  api:\n    image: python:3.13-alpine\n    networks: " + baseNetworks + "\nnetworks:\n  receiver:\n    external: true\n    name: independent-receiver\n"
				override := "services:\n  api:\n    networks: " + extraNetworks + "\nnetworks:\n  managed:\n    external: true\n    name: managed-transport\n"
				paths := []string{filepath.Join(root, "compose.yaml"), filepath.Join(root, "override.yaml")}
				for i, source := range []string{base, override} {
					if err := os.WriteFile(paths[i], []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
				}
				project, err := RenderComposeProjectFilesQuadletsEnv(paths, "", nil, "handoff")
				if err != nil {
					t.Fatal(err)
				}
				unit := project.Files["handoff-api.container"]
				for _, network := range []string{"Network=independent-receiver:alias=api", "Network=managed-transport:alias=api"} {
					if !strings.Contains(unit, network) {
						t.Fatalf("authored receiver DNS attachment lost when adding transport: missing %s\n%s", network, unit)
					}
				}
				// Explicit Compose override remains an intentional replacement.
				override = strings.Replace(override, "networks: ", "networks: !override ", 1)
				if err := os.WriteFile(paths[1], []byte(override), 0600); err != nil {
					t.Fatal(err)
				}
				project, err = RenderComposeProjectFilesQuadletsEnv(paths, "", nil, "handoff")
				if err != nil || strings.Contains(project.Files["handoff-api.container"], "Network=independent-receiver") {
					t.Fatal("explicit attachment replacement ignored", err)
				}
			})
		}
	}
}
