package main

import (
	"strings"
	"testing"
)

func TestLocalAccessConflictExplainsSingleCoreAndRemoteAlternative(t *testing.T) {
	err := targetAccessConflictError("local")
	if err == nil {
		t.Fatal("local access conflict must fail closed")
	}
	for _, want := range []string{
		"access \"local\" already exists with different provider/reference",
		"one shared Core per host",
		"baha target list",
		"baha node add",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing local Core isolation guidance %q in %v", want, err)
		}
	}
	other := targetAccessConflictError("production-api")
	if strings.Contains(other.Error(), "shared Core") {
		t.Fatalf("non-local access conflict incorrectly describes local singleton: %v", other)
	}
}
