package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestTargetListRemainsAvailableWhenSelectionAmbiguous(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{
		"alpha": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
		"beta":  {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}},
	}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	for _, child := range targetCommand().Children {
		if child.Name != "list" {
			continue
		}
		if err := child.Run(context.Background(), []string{"--json"}, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "\"alpha\"") || !strings.Contains(out.String(), "\"beta\"") {
			t.Fatal(out.String())
		}
		if strings.Contains(out.String(), "\"effective\"") {
			t.Fatal("ambiguous selection must not claim effective Target")
		}
		return
	}
	t.Fatal("target list is not registered")
}

func TestTargetListActiveMarkerUsesExplicitPrecedence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{
		"one": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
		"two": {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}},
	}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := writePersistedTarget("one"); err != nil {
		t.Fatal(err)
	}
	for _, child := range targetCommand().Children {
		if child.Name != "list" {
			continue
		}
		var out, errOut bytes.Buffer
		if err := child.Run(withTargetOverride(context.Background(), "two"), []string{"--json"}, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Targets []targetListItem `json:"targets"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		for _, entry := range result.Targets {
			isActive := false
			for _, sel := range entry.Selectors {
				if sel == "active" {
					isActive = true
				}
			}
			if isActive != (entry.Name == "two") {
				t.Fatalf("wrong active marker for %s: %v", entry.Name, entry.Selectors)
			}
		}
		return
	}
	t.Fatal("missing target list")
}

func TestTargetListWorksWithStalePersistedSelection(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 t.Setenv("BASEHARBOR_TARGET","")
 cfg:=deployment.Config{Version:deployment.ConfigVersion,Targets:map[string]deployment.TargetDefinition{
  "current":{Runtime:deployment.RuntimeDefinition{Provider:"docker"},Access:deployment.TargetAccess{Reference:"local"}},
 },Access:map[string]deployment.AccessDefinition{"local":{Provider:"local",Reference:"local"}}}
 if err:=cfg.Save();err!=nil{t.Fatal(err)}
 if err:=writePersistedTarget("deleted");err!=nil{t.Fatal(err)}
 if _,err:=selectedTargetName("","",cfg);err==nil{t.Fatal("stale selection must fail closed for deployment")}
 for _,child:=range targetCommand().Children{
  if child.Name!="list"{continue}
  var out,errOut bytes.Buffer
  if err:=child.Run(context.Background(),[]string{"--json"},&out,&errOut);err!=nil{t.Fatalf("target discovery must work with stale selection: %v",err)}
  if !strings.Contains(out.String(),"\"current\""){t.Fatalf("expected configured target in list: %s",out.String())}
  return
 }
 t.Fatal("missing target list command")
}
