package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testConnectivityRule() ConnectivityRule {
	return ConnectivityRule{
		Source: ConnectivityEndpoint{Application: "app-a", Environment: "dev", Service: "api"},
		Target: ConnectivityEndpoint{Application: "app-b", Environment: "dev", Service: "postgres", Port: 5432},
	}
}

func TestConnectivityPolicyPersistsOneDirectionalRule(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	rule := testConnectivityRule()

	if err := AddConnectivityRule(rule); err != nil {
		t.Fatal(err)
	}
	if err := AddConnectivityRule(rule); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadConnectivityRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0] != rule {
		t.Fatalf("rules=%#v", rules)
	}

	info, err := os.Stat(filepath.Join(state, connectivityPolicyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("connectivity policy mode=%o want=600", info.Mode().Perm())
	}

	if err := RemoveConnectivityRule(rule); err != nil {
		t.Fatal(err)
	}
	rules, err = LoadConnectivityRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("rules remain: %#v", rules)
	}
}

func TestConnectivityRuleRequiresResolvedTargetPort(t *testing.T) {
	rule := testConnectivityRule()
	rule.Target.Port = 0
	err := rule.Validate()
	if err == nil || !strings.Contains(err.Error(), "target port is required") {
		t.Fatalf("expected target-port rejection, got %v", err)
	}
}

func TestConnectivityRuleRejectsSourcePort(t *testing.T) {
	rule := testConnectivityRule()
	rule.Source.Port = 8443
	err := rule.Validate()
	if err == nil || !strings.Contains(err.Error(), "source must not define") {
		t.Fatalf("expected source-port rejection, got %v", err)
	}
}

func TestConnectivityRuleIDIncludesTargetPort(t *testing.T) {
	a := testConnectivityRule()
	b := a
	b.Target.Port = 15432
	if ConnectivityRuleID(a) == ConnectivityRuleID(b) {
		t.Fatal("connectivity rule ids collide across target ports")
	}
	if ConnectivityNetworkName(a) == ConnectivityNetworkName(b) {
		t.Fatal("connectivity network names collide across target ports")
	}
}

func TestApplicationDestroyRequiresConnectivityRelease(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	rule := testConnectivityRule()
	if err := AddConnectivityRule(rule); err != nil {
		t.Fatal(err)
	}
	err := CheckApplicationConnectivityReleased(Manifest{Name: "app-b", Environment: "dev"})
	if err == nil || !strings.Contains(err.Error(), "disconnect it before destroy") {
		t.Fatalf("expected destroy guard, got %v", err)
	}
	if err := CheckApplicationConnectivityReleased(Manifest{Name: "app-c", Environment: "dev"}); err != nil {
		t.Fatalf("unrelated app blocked: %v", err)
	}
}

func TestConnectivityTargetAliasIsStableUniqueAndDNSBounded(t *testing.T) {
	a := testConnectivityRule()
	b := a
	b.Target.Port = 15432

	aliasA := ConnectivityTargetAlias(a)
	aliasA2 := ConnectivityTargetAlias(a)
	aliasB := ConnectivityTargetAlias(b)
	if aliasA != aliasA2 {
		t.Fatalf("target alias is not stable: %q != %q", aliasA, aliasA2)
	}
	if aliasA == aliasB {
		t.Fatalf("target aliases collide across target ports: %q", aliasA)
	}
	if len(aliasA) > 63 {
		t.Fatalf("target alias exceeds DNS label limit: %d %q", len(aliasA), aliasA)
	}

	long := a
	long.Target.Application = strings.Repeat("verylong", 20)
	if got := ConnectivityTargetAlias(long); len(got) > 63 {
		t.Fatalf("long target alias exceeds DNS label limit: %d %q", len(got), got)
	}
}
