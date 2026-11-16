package apierror

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	err := New(CodeNotFound, "", 0)
	if err.Status != 404 || err.Message != "resource not found" {
		t.Fatalf("unexpected defaults: %#v", err)
	}
}

func TestWrappedCauseIsNotSerialized(t *testing.T) {
	cause := errors.New("database password=super-secret")
	err := Wrap(CodeInternal, cause)

	raw, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	body := string(raw)
	if strings.Contains(body, "super-secret") || strings.Contains(body, "password") {
		t.Fatalf("internal cause leaked into JSON: %s", body)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause should remain available internally")
	}
}

func TestUnknownCodeFailsToInternalDefaults(t *testing.T) {
	err := New(Code("unknown"), "", 0)
	if err.Status != 500 || err.Message != "internal error" {
		t.Fatalf("unknown code must fail safely: %#v", err)
	}
}
