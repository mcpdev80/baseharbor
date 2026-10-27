package remoteprojection

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRemoteConsolidatedProjectBoundsResourceUnitsWithoutRenamingResources(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	project := "bh-local-remote-sql-0123456789abcdef-dev"
	resource := "baseharbor-remote-sql-0123456789abcdef-dev"
	source := "services:\n  postgres:\n    image: example/sql:fixture\n    volumes:\n      - postgres-data:/data\nvolumes:\n  postgres-data:\n    name: " + resource + "_postgres-data\nnetworks:\n  default:\n    name: " + resource + "\n"
	if err := os.WriteFile(compose, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	graph, err := ProjectRemoteQuadletGraph(compose, "", project, nil)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := ProjectRemoteQuadletGraph(compose, "", project, nil)
	if err != nil || !reflect.DeepEqual(graph, repeated) {
		t.Fatal("remote graph identity changed on retry", err)
	}
	if len(graph.Units) != 3 {
		t.Fatal("missing project resource graph")
	}
	var network, volume, container string
	for _, name := range graph.Units {
		if len(strings.TrimSuffix(name, filepath.Ext(name))) > 64 {
			t.Fatal("oversized remote unit", name)
		}
		switch filepath.Ext(name) {
		case ".network":
			network = name
		case ".volume":
			volume = name
		case ".container":
			container = name
		}
	}
	for _, expected := range []string{"Network=" + network, "Volume=" + volume + ":/data", "Requires=" + generatedServiceName(network), "ContainerName=" + project + "-postgres", "Label=com.docker.compose.project=" + project} {
		if !strings.Contains(graph.Files[container], expected) {
			t.Fatal("lost dependency or ownership identity", expected)
		}
	}
	if !strings.Contains(graph.Files[network], "NetworkName="+resource) || !strings.Contains(graph.Files[volume], "VolumeName="+resource+"_postgres-data") {
		t.Fatal("native resource identity was renamed")
	}
}
