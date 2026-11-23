package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// withOrganizationDefaultStack overrides the org-default source for the
// duration of a test, restoring the original afterwards.
func withOrganizationDefaultStack(name string, err error) func() {
	orig := organizationDefaultStackName
	organizationDefaultStackName = func(string) (string, error) { return name, err }
	return func() { organizationDefaultStackName = orig }
}

// TestGuidedStackProfileOrgDefaultMissingStackUsageError proves that an
// organization-default stack name that is not present in the effective stack
// catalog yields a usageError (user-facing hint), not a raw fmt.Errorf.
// This is the H-P1 hunt finding F6: app_new_wizard org-default missing-stack
// must return usageError parity, not a hard error.
//
// The missing-stack guard (orgDefaultStackNotInCatalog) fires before any
// Registry use, so a zero-value Registry exercises it deterministically. The
// org-default name is injected so the test does not depend on on-disk
// organization configuration.
func TestGuidedStackProfileOrgDefaultMissingStackUsageError(t *testing.T) {
	restore := withOrganizationDefaultStack("ghost", nil)
	defer restore()

	reader := bufio.NewReader(strings.NewReader(""))
	out := &bytes.Buffer{}
	sel, err := guidedStackProfile(reader, out, development.ProfileCatalogEntries{}, development.Registry{})
	if err == nil {
		t.Fatalf("expected an error for a missing org-default stack, got nil (selection %+v)", sel)
	}
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cli.UsageError for missing org-default stack, got %T: %v", err, err)
	}
	if usageErr.Message == "" {
		t.Fatalf("usageError must carry a user-facing message")
	}
	if _, ok := err.(*cli.UsageError); !ok {
		t.Fatalf("error must be a *cli.UsageError, got %T", err)
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("org-default missing-stack must not surface as EOF/abort")
	}
	// Regression: the message must expand the org-default stack name, not
	// print the literal format verb. Precedent usageError messages quote the
	// value (e.g. app_init_prompts fmt.Sprintf("... %q ...", line)).
	const wantQuoted = `"ghost"`
	if !strings.Contains(usageErr.Message, wantQuoted) {
		t.Fatalf("usageError message must contain the quoted stack name %q, got: %q", wantQuoted, usageErr.Message)
	}
	if strings.Contains(usageErr.Message, "%q") {
		t.Fatalf("usageError message must not contain a literal format verb, got: %q", usageErr.Message)
	}
}

// TestGuidedStackProfileOrgDefaultPresentStackResolves proves that a present
// org-default stack resolves successfully (no error) through the same entry.
func TestGuidedStackProfileOrgDefaultPresentStackResolves(t *testing.T) {
	restore := withOrganizationDefaultStack("dev", nil)
	defer restore()

	reader := bufio.NewReader(strings.NewReader(""))
	out := &bytes.Buffer{}
	catalog := development.ProfileCatalogEntries{"dev": {Profile: development.StackProfile{
		Metadata: development.ProfileMetadata{Name: "dev"},
		Components: []development.Component{
			{ID: "backend", Role: "backend", Adapter: "development/go"},
		},
	}}}
	sel, err := guidedStackProfile(reader, out, catalog, development.Registry{})
	if err != nil {
		t.Fatalf("present org-default stack must resolve, got %T: %v", err, err)
	}
	if sel.Effective.Metadata.Name != "dev" {
		t.Fatalf("expected resolved stack name dev, got %q", sel.Effective.Metadata.Name)
	}
}

// TestOrgDefaultStackNotInCatalog is the pure guard behind the missing-stack
// usageError: a non-empty name absent from the catalog is reported, an empty or
// present name is not.
func TestOrgDefaultStackNotInCatalog(t *testing.T) {
	catalog := development.ProfileCatalogEntries{
		"dev":    {},
		"server": {},
	}
	if orgDefaultStackNotInCatalog("", catalog) {
		t.Fatalf("empty org-default name must not be reported as missing")
	}
	if orgDefaultStackNotInCatalog("dev", catalog) {
		t.Fatalf("present org-default name must not be reported as missing")
	}
	if !orgDefaultStackNotInCatalog("ghost", catalog) {
		t.Fatalf("absent org-default name must be reported as missing")
	}
}
