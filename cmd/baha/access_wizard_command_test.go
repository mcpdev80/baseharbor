package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func accessTestChild(t *testing.T, root *cli.Command, name string) *cli.Command {
	t.Helper()
	for _, child := range root.Children {
		if child.Name == name {
			return child
		}
	}
	t.Fatalf("missing command %q", name)
	return nil
}

// The access wizard changes policy assignments; it must not be mistaken for
// the separate login, Target selection or Node enrollment flows.
func TestAccessCommandsAreDistinctFromCoreLoginAndNodeConnect(t *testing.T) {
	root := rootCommand()
	access := accessTestChild(t, root, "access")
	for _, path := range []string{"login", "target", "node"} {
		accessTestChild(t, root, path)
	}
	for _, operation := range []string{"assign", "offboard"} {
		cmd := accessTestChild(t, access, operation)
		if cmd.Run == nil {
			t.Fatalf("access %s must have executable validation", operation)
		}
	}
	for _, child := range access.Children {
		switch child.Name {
		case "assign", "offboard":
		default:
			t.Fatalf("unexpected access subcommand %q; Core connection belongs to login/target/node", child.Name)
		}
	}
}

// Malformed subject input must be rejected before loading organization state
// or reaching the production policy authority.
func TestAccessCommandsFailClosedBeforeAuthorityOnMissingOrAmbiguousPerson(t *testing.T) {
	access := accessTestChild(t, rootCommand(), "access")
	for _, operation := range []string{"assign", "offboard"} {
		cmd := accessTestChild(t, access, operation)
		for _, args := range [][]string{nil, {"--json"}, {"alice", "bob"}} {
			t.Run(operation+"/"+strings.Join(args, "-"), func(t *testing.T) {
				var out, errOut bytes.Buffer
				err := cmd.Run(context.Background(), args, &out, &errOut)
				if err == nil || !strings.Contains(err.Error(), "requires exactly one PERSON") {
					t.Fatalf("expected fail-closed subject validation, got %v", err)
				}
				if out.Len() != 0 || errOut.Len() != 0 {
					t.Fatalf("invalid identity must not start wizard or produce a success response: stdout=%q stderr=%q", out.String(), errOut.String())
				}
			})
		}
	}
}

// The CLI positional person and the person typed into the wizard must agree
// exactly; a different name would otherwise grant or revoke the wrong account.
func TestAccessWizardRejectsSubjectSubstitution(t *testing.T) {
	for _, tc := range []struct {
		requested string
		reviewed  string
		allowed   bool
	}{
		{"alice", "alice", true},
		{"alice", "bob", false},
		{"alice", "Alice", false},
		{"alice", "alice ", false},
		{"", "alice", false},
		{"alice", "", false},
		{" ", " ", false},
	} {
		err := validateAccessWizardPerson(tc.requested, tc.reviewed)
		if (err == nil) != tc.allowed {
			t.Errorf("requested=%q reviewed=%q allowed=%t: %v", tc.requested, tc.reviewed, tc.allowed, err)
		}
	}
}

func TestAccessWizardMachineResponseContainsOnlyPersonAndDecision(t *testing.T) {
	data, err := json.Marshal(accessWizardJSON{Person: "alice", Decision: "deny"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["person"] != "alice" || fields["decision"] != "deny" {
		t.Fatalf("access machine response expanded its bounded contract: %s", data)
	}
}
