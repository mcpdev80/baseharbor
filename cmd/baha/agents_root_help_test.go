package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootAgentsHelpSeparatesDocumentationFromDeployment(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"app", "init", "--help"}, {"init", "--help"}} {
		var out bytes.Buffer
		if err := runWithIO(context.Background(), args, &out, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "init --agents [--json]") || strings.Contains(out.String(), "[--agents]") {
			t.Fatalf("root command advertises conflicting agents/lifecycle arguments: %s", out.String())
		}
	}
}
