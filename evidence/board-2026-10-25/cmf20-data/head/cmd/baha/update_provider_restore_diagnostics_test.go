package main

import (
	"strings"
	"testing"
)

func TestProviderRestoreDiagnosticsNeverExposeDatabaseValues(t *testing.T) {
	for _, input := range []string{"pg_restore: must be owner of extension pgcrypto; password=secret-row-value", "COPY failed for secret-row-value", "Permission Denied: secret-row-value"} {
		classified := classifyProviderRestoreFailure(input)
		if classified == "" || strings.Contains(classified, "secret-row-value") || strings.Contains(classified, "pgcrypto") {
			t.Fatalf("protected data leaked: %q", classified)
		}
	}
}
