package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/delivery"
)

// TestM157NoConfirmNoPanicIsUsageErrorNamingDigest is the BF-01 regression for
// the candidate choke point: an apply WITHOUT --confirm-candidate must
// (a) NOT print a Go stack trace (0 stderr bytes), and (b) return a clean
// *cli.UsageError whose message re-surfaces the EXACT confirm-hash the write
// would bind, so automation can parse and re-run.
func TestM157NoConfirmNoPanicIsUsageErrorNamingDigest(t *testing.T) {
	r := repoCandidateFixture(t)
	pr, pw, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	old := os.Stderr
	os.Stderr = pw
	defer func() { os.Stderr = old }()
	var errBuf bytes.Buffer
	go func() { _, _ = io.Copy(&errBuf, pr) }()
	defer func() { _ = pw.Close() }()

	_, err := recordDeploymentBeforeMutation(context.Background(), r, "applying", "")
	if err == nil {
		t.Fatal("expected fail-closed error without confirm digest")
	}
	if errBuf.Len() > 0 {
		t.Fatalf("BF-01 panic leaked to stderr: %d bytes: %.400q", errBuf.Len(), errBuf.String())
	}
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *cli.UsageError, got %T: %v", err, err)
	}
	if usageErr.Hint == "" {
		t.Fatal("usage error missing re-run hint")
	}
	submission, _ := candidateSubmissionForResolved(t, r)
	want := delivery.CandidateConsentDigest(submission)
	if want == "" {
		t.Fatal("computed candidate digest is empty")
	}
	if !strings.Contains(usageErr.Message, want) && !strings.Contains(usageErr.Hint, want) {
		t.Fatalf("usage error must name the exact --confirm-candidate digest %q; message=%q hint=%q", want, usageErr.Message, usageErr.Hint)
	}
}
