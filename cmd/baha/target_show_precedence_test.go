package main

import (
 "bytes"
 "context"
 "encoding/json"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestTargetShowHonorsGlobalExplicitOverride(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME", t.TempDir())
 t.Setenv("BASEHARBOR_TARGET","")
 cfg:=deployment.Config{Version:deployment.ConfigVersion,Targets:map[string]deployment.TargetDefinition{
  "one":{Runtime:deployment.RuntimeDefinition{Provider:"docker"},Access:deployment.TargetAccess{Reference:"local"}},
  "two":{Runtime:deployment.RuntimeDefinition{Provider:"podman"},Access:deployment.TargetAccess{Reference:"local"}},
 },Access:map[string]deployment.AccessDefinition{"local":{Provider:"local",Reference:"local"}}}
 if err:=cfg.Save();err!=nil{t.Fatal(err)}
 if err:=writePersistedTarget("one");err!=nil{t.Fatal(err)}
 for _,child:=range targetCommand().Children {
  if child.Name!="show" {continue}
  var stdout,stderr bytes.Buffer
  if err:=child.Run(withTargetOverride(context.Background(),"two"),[]string{"--json"},&stdout,&stderr);err!=nil{t.Fatal(err)}
  var result targetShowResult
  if err:=json.Unmarshal(stdout.Bytes(),&result);err!=nil{t.Fatal(err)}
  if result.Target.Name!="two"{t.Fatalf("expected global explicit target two, got %s",result.Target.Name)}
  return
 }
 t.Fatal("target show command not found")
}
