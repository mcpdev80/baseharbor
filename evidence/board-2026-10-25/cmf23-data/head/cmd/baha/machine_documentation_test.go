package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const registryStart = "<!-- BEGIN GENERATED MCP REGISTRY -->"
const registryEnd = "<!-- END GENERATED MCP REGISTRY -->"

func TestMachineDocumentationTracksRegistryAndCommandTree(t *testing.T) {
	var tools strings.Builder
	fmt.Fprintln(&tools, "| Tool | Safety | Policy required | Approval required |")
	fmt.Fprintln(&tools, "| --- | --- | --- | --- |")
	for _, operation := range machine.Operations() {
		fmt.Fprintf(&tools, "| `%s` | `%s` | %t | %t |\n", operation.MCPTool, operation.Safety, operation.PolicyRequired, operation.ConfirmationRequired)
	}
	docPath := filepath.Join("..", "..", "docs", "reference", "mcp.md")
	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(content)
	start, end := strings.Index(doc, registryStart), strings.Index(doc, registryEnd)
	if start < 0 || end < start {
		t.Fatal("MCP documentation lacks registry generation markers")
	}
	expected := doc[:start+len(registryStart)] + "\n\n" + tools.String() + "\n" + doc[end:]
	checkGeneratedDocumentation(t, docPath, expected)
	var coverage strings.Builder
	fmt.Fprint(&coverage, "# CLI / machine coverage\n\n")
	fmt.Fprint(&coverage, "Generated from the actual command tree and typed operation registry. Every command is semantic, an alias, presentation, or an explicitly justified host/transport exclusion. Optional and compound modes are documented in each row. Unknown product commands fail the inventory test.\n\n")
	fmt.Fprint(&coverage, "Inspect current data with `baha agent describe -o json` (`cli_coverage`). Update this reference after a code change with `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree`. Ordinary test runs reject drift.\n\n")
	fmt.Fprintln(&coverage, "| Command | Classification | CLI JSON / structured result | Operation / tool | Reason |")
	fmt.Fprintln(&coverage, "| --- | --- | --- | --- | --- |")
	seen := map[string]bool{}
	for _, row := range currentCommandCoverage() {
		if seen[row.Command] {
			t.Fatalf("duplicate command: %s", row.Command)
		}
		seen[row.Command] = true
		if row.Classification == "gap" {
			t.Fatalf("unclassified product command: %s", row.Command)
		}
		if row.Classification == "semantic" {
			if operation, ok := machine.OperationByID(row.Operation); !ok || operation.MCPTool != row.MCPTool {
				t.Fatalf("invalid semantic mapping: %#v", row)
			}
		}
		fmt.Fprintf(&coverage, "| `%s` | %s | %s | %s | %s |\n", row.Command, row.Classification, row.JSONSurface, row.MCPTool, strings.ReplaceAll(row.Reason, "|", "/"))
	}
	checkGeneratedDocumentation(t, filepath.Join("..", "..", "docs", "reference", "cli-machine-coverage.md"), coverage.String())
}

func checkGeneratedDocumentation(t *testing.T, path, expected string) {
	t.Helper()
	if os.Getenv("BASEHARBOR_UPDATE_MACHINE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(expected), 0644); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != expected {
		t.Fatalf("machine documentation drift in %s; regenerate from the registry/command tree", path)
	}
}
