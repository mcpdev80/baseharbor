package applicationsecret

import (
	"strings"
	"testing"
)

func TestDynamicReferenceRoundTrip(t *testing.T) {
	key, err := newDynamicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, dynamicKeyPrefix) {
		t.Fatalf("key %q missing dynamic prefix", key)
	}
	ref := Reference(dynamicReferencePrefix + key)
	parsed, err := parseDynamicReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != key {
		t.Fatalf("parsed key = %q, want %q", parsed, key)
	}
}

func TestDynamicReferencesAreOpaqueAndStrict(t *testing.T) {
	validKey := dynamicKeyPrefix + strings.Repeat("a", 32)
	valid := Reference(dynamicReferencePrefix + validKey)
	if _, err := parseDynamicReference(valid); err != nil {
		t.Fatalf("valid reference rejected: %v", err)
	}
	for _, ref := range []Reference{
		"OPENAI_API_KEY",
		"baseharbor://other/" + validKey,
		"baseharbor://secrets/OPENAI_API_KEY",
		"baseharbor://secrets/dyn-short",
		Reference(dynamicReferencePrefix + validKey + "/nested"),
		Reference(dynamicReferencePrefix + validKey + "?x=1"),
	} {
		if _, err := parseDynamicReference(ref); err == nil {
			t.Fatalf("invalid reference accepted: %q", ref)
		}
	}
}

func TestDynamicValueValidation(t *testing.T) {
	if err := validateDynamicValue(nil); err == nil {
		t.Fatal("empty dynamic secret accepted")
	}
	if err := validateDynamicValue(make([]byte, (1<<20)+1)); err == nil {
		t.Fatal("oversized dynamic secret accepted")
	}
	if err := validateDynamicValue([]byte("ok")); err != nil {
		t.Fatalf("valid dynamic secret rejected: %v", err)
	}
}
