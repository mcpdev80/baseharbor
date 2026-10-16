package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppEditLoadRichRoundTrip(t *testing.T) {
	source := []byte("{\"name\":\"demo\",\"services\":{\"sql_instances\":{\"primary\":{},\"analytics\":{}}},\"extension\":{\"future\":[1,2,3]},\"identity\":{\"scopes\":[\"openid\"]}}")
	original := bytes.Clone(source)
	req := capability.Requirement{Name: "primary"}
	view := GuidedReadContext{ApplicationID: "existing-id", Name: "demo", Target: "dev",
		Capabilities: []capability.Requirement{req},
		Preferences: []application.ProviderPreference{{Requirement: req, InstanceID: "db-dev"}},
		Footprint: application.Footprint{Target: "dev"}, Complete: true}
	before, _ := json.Marshal(view)
	loaded, err := appEditLoad(context.Background(), "dev", source, func(context.Context, string) (GuidedReadContext, error) { return view, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(appEditLoadSource(loaded), original) || loaded.ApplicationID != "existing-id" ||
		len(loaded.Capabilities) != 1 || len(loaded.Preferences) != 1 || loaded.Preferences[0].InstanceID != "db-dev" {
		t.Fatalf("lost source or selections: %+v", loaded)
	}
	loaded.Source[0] = ' '
	if !bytes.Equal(source, original) {
		t.Fatal("source was mutated")
	}
	after, _ := json.Marshal(view)
	if !bytes.Equal(before, after) {
		t.Fatal("read authority mutated")
	}
}

func TestAppEditLoadTwoTargetsAndAutomatic(t *testing.T) {
	req := capability.Requirement{Name: "primary"}
	views := map[string]GuidedReadContext{
		"dev": {Target: "dev", Name: "app", ApplicationID: "same", Footprint: application.Footprint{Target: "dev"},
			Preferences: []application.ProviderPreference{{Requirement: req}}},
		"prod": {Target: "prod", Name: "app", ApplicationID: "same", Footprint: application.Footprint{Target: "prod"},
			Preferences: []application.ProviderPreference{{Requirement: req, InstanceID: "prod-database"}}},
	}
	read := func(_ context.Context, target string) (GuidedReadContext, error) { return views[target], nil }
	dev, err := appEditLoad(context.Background(), "dev", []byte("{}"), read)
	if err != nil {
		t.Fatal(err)
	}
	prod, err := appEditLoad(context.Background(), "prod", []byte("{}"), read)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Preferences[0].InstanceID != "" || prod.Preferences[0].InstanceID != "prod-database" {
		t.Fatalf("cross-target preference loss: %+v %+v", dev, prod)
	}
	prod.Preferences[0].InstanceID = "changed"
	if views["prod"].Preferences[0].InstanceID != "prod-database" {
		t.Fatal("authority changed")
	}
}

func TestAppEditLoadMissingAmbiguousIncompatible(t *testing.T) {
	for _, kind := range []string{"missing", "ambiguous", "incompatible", "unverifiable"} {
		_, err := appEditLoad(context.Background(), "dev", []byte("{}"), func(context.Context, string) (GuidedReadContext, error) {
			return GuidedReadContext{}, &GuidedReadError{Kind: kind, Cause: errors.New("/private/path?token=secret")}
		})
		var typed *appEditLoadError
		if !errors.As(err, &typed) || typed.Kind != kind {
		t.Fatalf("%s: %v", kind, err)
	}
	}
}

func TestAppEditLoadReadOnlyWrongTargetAndIncomplete(t *testing.T) {
	calls := 0
	view := GuidedReadContext{ApplicationID: "existing", Name: "app", Target: "dev",
		Footprint: application.Footprint{Target: "dev"}, Complete: false}
	reader := func(context.Context, string) (GuidedReadContext, error) { calls++; return view, nil }
	loaded, err := appEditLoad(context.Background(), "dev", []byte("{}"), reader)
	if err != nil || loaded.Complete {
		t.Fatalf("%+v %v", loaded, err)
	}
	_, err = appEditLoad(context.Background(), "prod", []byte("{}"), reader)
	var typed *appEditLoadError
	if !errors.As(err, &typed) || typed.Kind != "wrong_target" || calls != 2 {
		t.Fatalf("expected fail closed: %v calls=%d", err, calls)
	}
}
