package cli

import (
	"bytes"
	"testing"
)

func TestHelpSectionNewlinesExactBytes(t *testing.T) {
	for _, width := range []string{"40", "80", "160"} {
		t.Run(width, func(t *testing.T) {
			t.Setenv("COLUMNS", width)
			tests := []struct {
				name    string
				command *Command
				want    string
			}{
				{"root", &Command{Name: "baha", HelpGroups: []HelpGroup{{Title: "Get started", Commands: []string{"up"}}}, Children: []*Command{{Name: "up", Summary: "Start"}, {Name: "doctor", Summary: "Inspect"}, {Name: "secret", Hidden: true}}}, "baha\n\nGet started:\n  up              Start\n\nAdvanced and operator commands:\n  doctor          Inspect\n"},
				{"group", &Command{Name: "app", Children: []*Command{{Name: "status", Summary: "Inspect"}}}, "app\n\nCommands:\n  status  Inspect\n"},
				{"subcommand", &Command{Name: "status", Usage: "baha app status", Examples: []string{"baha app status"}}, "status\n\nUsage:\n  baha app status\n\nExamples:\n  baha app status\n"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var out bytes.Buffer
					tt.command.Help(&out)
					section := bytes.SplitN(out.Bytes(), []byte("\nGlobal options:"), 2)[0]
					if !bytes.Equal(section, []byte(tt.want)) {
						t.Fatalf("help bytes:\ngot  %q\nwant %q", out.Bytes(), tt.want)
					}
				})
			}
		})
	}
}
