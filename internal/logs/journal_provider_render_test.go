package logs

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func TestJournalProviderRelabelMatchesComposeContainerNames(t *testing.T) {
	registration := Registration{Application: "demo", Environment: "test"}
	source := observability.SignalSource{
		Class:            observability.SourceApplicationProvider,
		Provider:         capability.ProviderPostgreSQL,
		OwnerApplication: "demo",
		Target:           observability.RuntimeTarget("bh.demo", "postgres"),
	}
	config := alloyJournalConfig([]Registration{registration}, []observability.SignalSource{source})
	_, rules, ok := strings.Cut(config, `loki.relabel "application_provider_0" {`)
	if !ok {
		t.Fatalf("provider relabel rule missing from config:\n%s", config)
	}
	var pattern string
	for _, line := range strings.Split(rules, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && strings.TrimSpace(key) == "regex" {
			var err error
			pattern, err = strconv.Unquote(strings.TrimSpace(value))
			if err != nil {
				t.Fatalf("invalid rendered Alloy regex: %v", err)
			}
			break
		}
	}
	if pattern == "" {
		t.Fatal("provider relabel regex missing")
	}
	match, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile rendered Alloy regex: %v", err)
	}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"bh.demo-postgres-1", true},
		{"bh.demo-postgres-12", true},
		{"bh.demo_postgres_1", true},
		{"bh.demo-postgres", true},
		{"bh.demo-postgres1", false},
		{"bh.demo-postgres-", false},
		{"bhXdemo-postgres-1", false},
		{"bh.demo-postgres-backup-1", false},
		{"other-bh.demo-postgres-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := match.MatchString(tc.name); got != tc.want {
				t.Errorf("rendered regex %q matched %q = %v, want %v", pattern, tc.name, got, tc.want)
			}
		})
	}
}
