package main

import (
	"os"
	"strings"
	"testing"
)

// Provider-change failures must return through their typed error/result owner,
// never print raw debug outcomes into a JSON/CLI response stream.
func TestMachineProviderChangeHasNoRawDiagnosticStdout(t *testing.T) {
	source, err := os.ReadFile("app_machine_adapters.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range []string{"DIAG_" + "EXEC", "fmt." + "Printf"} {
		if strings.Contains(string(source), diagnostic) {
			t.Fatalf("production machine adapter contains forbidden raw stdout diagnostic %q", diagnostic)
		}
	}
}
