package repositoryinspect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type realWorldCorpus struct {
	Cases []struct {
		Category string   `yaml:"category"`
		Repo     string   `yaml:"repo"`
		Revision string   `yaml:"revision"`
		Subpath  string   `yaml:"subpath"`
		Tags     []string `yaml:"tags"`
		Proves   []string `yaml:"proves"`
	} `yaml:"cases"`
}

func TestRealWorldCorpusComposition(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adoption-realworld", "corpus.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus realWorldCorpus
	if err := yaml.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 30 {
		t.Fatalf("real-world corpus = %d cases, want 30", len(corpus.Cases))
	}
	counts := map[string]int{}
	seen := map[string]struct{}{}
	for i, tc := range corpus.Cases {
		counts[tc.Category]++
		if tc.Repo == "" || !strings.Contains(tc.Repo, "/") {
			t.Fatalf("case %d invalid repo %q", i, tc.Repo)
		}
		if len(tc.Revision) != 40 {
			t.Fatalf("case %s revision must be pinned SHA, got %q", tc.Repo, tc.Revision)
		}
		if tc.Subpath == "" {
			t.Fatalf("case %s missing subpath", tc.Repo)
		}
		if len(tc.Tags) == 0 || len(tc.Proves) == 0 {
			t.Fatalf("case %s must document tags and proof purpose", tc.Repo)
		}
		key := tc.Category + "\x00" + tc.Repo + "\x00" + tc.Revision + "\x00" + tc.Subpath
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate real-world case %s", key)
		}
		seen[key] = struct{}{}
	}
	for _, category := range []string{"compose", "quadlet", "kubernetes"} {
		if counts[category] != 10 {
			t.Fatalf("%s corpus = %d, want 10", category, counts[category])
		}
	}
}
