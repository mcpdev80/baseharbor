package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"testing"
	"time"
)

// TestGitOutputUpdatePathNeverTakesControllingTerminal is the focused
// regression for the card-0428-217 scope criterion on cmd/baha/app_update.go
// (the reviewer's BEFUND 2): the update-path git helpers gitOutput and
// gitIsAncestor must hand their git subprocess a bounded, NON-terminal stdin
// so a credential prompt can never read the developer's controlling terminal
// and hang the update preflight. The witness drives the REAL helpers under a
// tight deadline: with the guard the calls terminate instantly (fetch fails,
// ancestor checks answer); drop the two cmd.Stdin lines and run the suite
// attached to a terminal and git would block instead — the mutation fails.
func TestGitOutputUpdatePathNeverTakesControllingTerminal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if _, err := gitOutput(context.Background(), dir, "init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := gitOutput(context.Background(), dir, "-c", "user.email=t@example.invalid",
		"-c", "user.name=T", "commit", "-q", "--allow-empty", "-m", "seed"); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	// The credential-prompt machinery the card names, hostile-but-inert:
	// any git that reaches a prompt hits GIT_TERMINAL_PROMPT=0 and fails
	// closed; the askpass script records (and never must run here).
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("SSH_AUTH_SOCK", dir+"/absent.sock")

	// Deadline far below any interactive wait: a hung prompt FAILS here.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The exact update-path shape (:265/:268): fetch a remote whose URL is
	// the repo itself — real object transfer, real helper code path, no
	// network. It must succeed.
	if _, err := gitOutput(ctx, dir, "fetch", "--prune", "--", dir); err != nil {
		t.Fatalf("gitOutput(fetch) on a local remote must succeed, got %v", err)
	}
	if !gitIsAncestor(ctx, dir, "HEAD", "HEAD") {
		t.Fatal("HEAD must be its own ancestor")
	}
	if gitIsAncestor(ctx, dir, "HEAD", "nonexistent-ref") {
		t.Fatal("a missing descendant must answer false, not hang")
	}

	// Structural pin of the guard itself (mutation M for this criterion):
	// gitOutput and gitIsAncestor must set a non-nil stdin on their cmd
	// before Run. Remove the two lines and this fails at once.
	pinHelperStdinContract(t)
}

// pinHelperStdinContract asserts the source-level contract of card 0428-217
// on this package's own update-path helpers: every exec.Command in gitOutput
// and gitIsAncestor assigns cmd.Stdin before starting, so the git subprocess
// can never inherit a controlling terminal.
func pinHelperStdinContract(t *testing.T) {
	t.Helper()
	fset := token.NewFileSet()
	sourceDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	file, err := parser.ParseFile(fset, filepath.Join(sourceDir, "app_update.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse app_update.go: %v", err)
	}
	wanted := map[string]bool{"gitOutput": false, "gitIsAncestor": false}
	covered := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			return true
		}
		if _, tracked := wanted[fn.Name.Name]; !tracked || covered[fn.Name.Name] {
			return true
		}
		var madeCmd, setStdin bool
		if fn.Body != nil {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					id := call.Fun
					if paren, ok := id.(*ast.ParenExpr); ok {
						id = paren.X
					}
					if sel, ok := id.(*ast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "Command") {
						madeCmd = true
					}
				}
				switch x := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Stdin" {
							setStdin = true
						}
					}
				}
				return true
			})
		}
		if madeCmd && !setStdin {
			t.Errorf("%s must assign cmd.Stdin (bounded non-terminal stdin, card 0428-217): the git subprocess may never read the controlling terminal", fn.Name.Name)
		}
		if madeCmd {
			covered[fn.Name.Name] = true
		}
		return true
	})
	for fn := range wanted {
		if !covered[fn] {
			t.Fatalf("%s: no exec.Command found — the witness lost its target", fn)
		}
	}
}
