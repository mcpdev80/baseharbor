package remoteprojection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteQuadletProjectionUsesBundleFilesAndDropsCoreHostProcessDefaults(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	env := filepath.Join(dir, "runtime.env")
	if err := os.WriteFile(compose, []byte("services:\n  sql:\n    image: example/sql:fixture\n    environment:\n      PASSWORD: ${PASSWORD}\n    volumes:\n      - ./tls/key.pem:/run/key.pem:ro\n      - sql-data:/data\nvolumes:\n  sql-data: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("PASSWORD=fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINERS_STORAGE_CONF", "/core/host/storage.conf")
	graph, err := ProjectRemoteQuadletGraph(compose, env, "owned", []string{"tls/key.pem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Units) != 3 || graph.Files["owned-sql.env"] != "PASSWORD=fixture\n" {
		t.Fatal("complete graph/environment not projected", graph.Units)
	}
	container := graph.Files["owned-sql.container"]
	for _, expected := range []string{"Volume=@BASEHARBOR_BUNDLE@/tls/key.pem:/run/key.pem:ro", "EnvironmentFile=@BASEHARBOR_BUNDLE@/owned-sql.env", "Network=owned-default.network"} {
		if !strings.Contains(container, expected) {
			t.Fatal("missing protected portable binding", expected)
		}
	}
	for _, content := range graph.Files {
		if strings.Contains(content, dir) || strings.Contains(content, "/core/host/storage.conf") {
			t.Fatal("Core host filesystem configuration leaked into Node realization")
		}
	}
	if _, err := ProjectRemoteQuadletGraph(compose, env, "owned", nil); err == nil {
		t.Fatal("unapproved host bind file admitted")
	}
	if err := os.WriteFile(compose, []byte("services:\n  app:\n    build: .\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectRemoteQuadletGraph(compose, env, "owned", nil); err == nil {
		t.Fatal("Core checkout build instruction crossed remote runtime boundary")
	}
}
