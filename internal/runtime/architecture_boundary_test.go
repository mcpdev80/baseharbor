package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestPortableCoreRuntimeBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	coreRoots := []string{
		"cmd/baha",
		"internal/application",
		"internal/deployment",
		"internal/devgateway",
		"internal/identityprovider",
		"internal/logs",
	}

	forbiddenIdentifiers := map[string]struct{}{
		"ProviderCompose":             {},
		"DetectCompose":               {},
		"ComposeContainer":            {},
		"ListComposeContainers":       {},
		"detectComposeForApplication": {},
		"detectComposeForTarget":      {},
	}
	forbiddenRuntimeSelectors := map[string]struct{}{
		"Compose":        {},
		"DockerProvider": {},
		"PodmanProvider": {},
	}

	for _, relativeRoot := range coreRoots {
		base := filepath.Join(root, filepath.FromSlash(relativeRoot))
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, src, 0)
			if err != nil {
				t.Errorf("%s: parse: %v", relativePath(root, path), err)
				return nil
			}

			runtimeAliases := map[string]struct{}{}
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					continue
				}
				if strings.Contains(importPath, "/internal/providers/runtime/") {
					t.Errorf("%s imports concrete runtime provider package %q", relativePath(root, path), importPath)
				}
				if importPath == "github.com/mcpdev80/baseharbor/internal/runtime" {
					name := "runtime"
					if imported.Name != nil {
						name = imported.Name.Name
					}
					runtimeAliases[name] = struct{}{}
				}
			}

			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.Ident:
					if _, forbidden := forbiddenIdentifiers[n.Name]; forbidden {
						t.Errorf("%s:%d uses forbidden legacy runtime identifier %s", relativePath(root, path), fileSet.Position(n.Pos()).Line, n.Name)
					}
				case *ast.SelectorExpr:
					if n.Sel.Name == "Engine" {
						t.Errorf("%s:%d calls product runtime Engine() from portable core", relativePath(root, path), fileSet.Position(n.Pos()).Line)
					}
					if ident, ok := n.X.(*ast.Ident); ok {
						if _, runtimeImport := runtimeAliases[ident.Name]; runtimeImport {
							if _, forbidden := forbiddenRuntimeSelectors[n.Sel.Name]; forbidden {
								t.Errorf("%s:%d references concrete runtime type %s.%s", relativePath(root, path), fileSet.Position(n.Pos()).Line, ident.Name, n.Sel.Name)
							}
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", relativeRoot, err)
		}
	}
}

func TestRuntimeProviderPackageBoundaries(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	entries, err := os.ReadDir(filepath.Join(root, "internal", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, "podman") || strings.HasPrefix(name, "quadlet") {
			t.Errorf("provider-specific production file remains in internal/runtime root: %s", name)
		}
	}

	contractRoot := filepath.Join(root, "internal", "runtime", "contract")
	err = filepath.WalkDir(contractRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				continue
			}
			if strings.Contains(importPath, "/internal/providers/runtime/") {
				t.Errorf("%s imports concrete provider package %q", relativePath(root, path), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
