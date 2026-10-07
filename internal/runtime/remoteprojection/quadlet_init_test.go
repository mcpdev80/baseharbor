package remoteprojection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteInitProjectionRequiresCompletionAwareApplierAndNoImplicitRerun(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	data := "services:\n  app:\n    image: example/app:fixture\n    depends_on:\n      init:\n        condition: service_completed_successfully\n  init:\n    image: example/init:fixture\n    command: [sh, -c, 'exit 0']\n"
	if err := os.WriteFile(compose, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectRemoteQuadletGraph(compose, "", "owned", nil); err == nil {
		t.Fatal("ordinary applier admitted completion dependencies")
	}
	for _, project := range []string{"owned", strings.Repeat("long-project-", 8)} {
		graph, err := ProjectRemoteQuadletInitGraph(compose, "", project, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(graph.InitUnits) != 1 || graph.Files[graph.InitUnits[0]] == "" {
			t.Fatal("init identity lost during unit compaction")
		}
		unit := strings.TrimSuffix(graph.InitUnits[0], ".container") + ".service"
		var ordered bool
		for _, content := range graph.Files {
			if strings.Contains(content, "Wants="+unit) || strings.Contains(content, "Requires="+unit) || strings.Contains(content, "ExecStartPre=") {
				t.Fatal("native init rerun or shell completion heuristic preserved")
			}
			ordered = ordered || strings.Contains(content, "After="+unit)
		}
		if !ordered {
			t.Fatal("completion ordering lost")
		}
	}
}
