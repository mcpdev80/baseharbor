package main

import (
	"bytes"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"testing"
)

func TestEntireCLIHelpHasNoEscapedSections(t *testing.T) {
	var walk func(*cli.Command)
	walk = func(cmd *cli.Command) {
		var out bytes.Buffer
		cmd.Help(&out)
		for _, bad := range [][]byte{[]byte(`\nCommands:`), []byte(`\nAdvanced`), []byte(`\nGet started:`)} {
			if bytes.Contains(out.Bytes(), bad) {
				t.Fatalf("%s help has escaped section: %q", cmd.Name, out.Bytes())
			}
		}
		for _, child := range cmd.Children {
			walk(child)
		}
	}
	walk(rootCommand())
}
