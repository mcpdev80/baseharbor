package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

type humanCommandDocumentation struct {
	path, summary, usage, guide string
}

func humanCommandGuide(path string) string {
	parts := strings.Fields(path)
	if len(parts) < 2 {
		return "index.md"
	}
	switch parts[1] {
	case "target":
		return "targets.md"
	case "provider":
		return "providers.md"
	case "stack", "workspace", "dev":
		return "development.md"
	case "config", "policy":
		return "organization-policy.md"
	case "login", "logout", "whoami", "trust", "openbao", "connect", "disconnect", "connections":
		return "security-trust.md"
	case "agent", "mcp", "serve":
		return "automation-agents.md"
	case "completion", "shell-init", "prompt", "tui":
		return "shell-ux.md"
	case "up", "down", "destroy", "plan", "status", "doctor", "update", "version":
		return "core.md"
	default:
		return "applications.md"
	}
}

func humanCommandDocumentationRows(root *cli.Command) []humanCommandDocumentation {
	var rows []humanCommandDocumentation
	seen := map[string]bool{}
	var walk func(*cli.Command, string)
	walk = func(command *cli.Command, parent string) {
		if command == nil || command.Hidden {
			return
		}
		path := strings.TrimSpace(parent + " " + command.Name)
		if seen[path] {
			return
		}
		seen[path] = true
		rows = append(rows, humanCommandDocumentation{path, command.Summary, command.Usage, humanCommandGuide(path)})
		for _, alias := range command.Aliases {
			aliasPath := strings.TrimSpace(parent + " " + alias)
			if !seen[aliasPath] {
				seen[aliasPath] = true
				rows = append(rows, humanCommandDocumentation{aliasPath, "Alias: " + path, command.Usage, humanCommandGuide(path)})
			}
		}
		for _, child := range command.Children {
			walk(child, path)
		}
	}
	walk(root, "")
	sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })
	return rows
}

func humanCommandIndex(rows []humanCommandDocumentation, german bool) string {
	var out strings.Builder
	if german {
		fmt.Fprint(&out, "# Vollständiger CLI-Befehlsindex\n\nDie Einträge werden aus dem ausgelieferten Befehlsbaum erzeugt. Kurzbeschreibung und Syntax geben die englischsprachige CLI-Hilfe unverändert wieder; die verlinkten Aufgabenanleitungen erklären den menschlichen Ablauf auf Deutsch.\n\nDer normale Einstieg ist `baha new` oder `baha init`. Explizite `app`-Befehle bleiben für Administration und Automation sichtbar. `baha update` betrifft BaseHarbor/Core; `baha app update` betrifft die Application-Source.\n\nExakte Optionen stehen in `baha COMMAND --help`. [Globale Optionen](global-options.md) und [JSON/MCP-Zuordnung](../reference/cli-machine-coverage.md) ergänzen jeden Eintrag. Entfernte, fehlende oder geänderte Befehle lassen den Dokumentations-Test scheitern.\n\n| Befehl | CLI-Kurzbeschreibung (verifiziert) | CLI-Syntax (verifiziert) | Aufgabenanleitung |\n| --- | --- | --- | --- |\n")
	} else {
		fmt.Fprint(&out, "# Complete CLI command index\n\nEntries are generated from the shipped command tree. Descriptions and syntax reproduce the actual CLI help; linked task guides explain the human workflow.\n\nThe normal entry point is `baha new` or `baha init`. Explicit `app` commands remain visible for administration and automation. `baha update` concerns BaseHarbor/Core; `baha app update` concerns application source.\n\nFind exact options with `baha COMMAND --help`. [Global options](global-options.md) and [JSON/MCP mapping](../reference/cli-machine-coverage.md) accompany every entry. Removed, missing or changed commands fail the documentation test.\n\n| Command | CLI description (verified) | CLI syntax (verified) | Task guide |\n| --- | --- | --- | --- |\n")
	}
	for _, row := range rows {
		clean := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }
		fmt.Fprintf(&out, "| `%s` | %s | `%s` | [↗](%s) |\n", row.path, clean(row.summary), clean(row.usage), row.guide)
	}
	return out.String()
}

func TestHumanCommandIndexTracksExactCommandMetadata(t *testing.T) {
	rows := humanCommandDocumentationRows(rootCommand())
	for _, row := range rows {
		if row.summary == "" {
			t.Fatalf("command lacks public purpose: %s", row.path)
		}
	}
	for _, german := range []bool{false, true} {
		dir := filepath.Join("..", "..", "docs")
		if german {
			dir = filepath.Join(dir, "de")
		}
		checkGeneratedDocumentation(t, filepath.Join(dir, "cli", "command-index.md"), humanCommandIndex(rows, german))
	}
}

func TestHumanDocumentationUsesFinalInitializationPath(t *testing.T) {
	for _, file := range []string{"README.md", "docs/index.md", "docs/de/index.md", "docs/cli/index.md", "docs/de/cli/index.md", "docs/tutorials/getting-started.md", "docs/de/tutorials/getting-started.md"} {
		data, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "baha init") || strings.Contains(text, "baha app init") || strings.Contains(text, "English is canonical") || strings.Contains(text, "Pre-Freeze-Reviews") {
			t.Fatalf("stale human onboarding contract: %s", file)
		}
	}
}
