package coreupdate

import (
 "strings"
 "testing"
 "go.yaml.in/yaml/v3"
)
func TestRewriteOwnedComposeImagesPinsOnlySelectedProvider(t *testing.T){
 source:=[]byte("services:\n  postgres-member-1:\n    image: docker.io/library/postgres:18-alpine\n    restart: unless-stopped\n  openbao-member-1:\n    image: docker.io/openbao/openbao:2.7.0\n  foreign:\n    image: company/app:7\n")
 d:=Delta{Installed:Realization{Kind:SQL,Installation:"c",Scope:"shared",Instance:"postgres-member-1",Owner:"baseharbor",Image:"docker.io/library/postgres:18-alpine",Version:"18",Digest:digestA},Desired:Desired{Kind:SQL,Image:"docker.io/library/postgres:18-alpine",Version:"18",Digest:digestB},Classification:BackupRequired}
 result,err:=RewriteOwnedComposeImages(source,map[string]Delta{"postgres-member-1":d})
 if err!=nil{t.Fatal(err)}
 var parsed map[string]any
 if err:=yaml.Unmarshal(result,&parsed);err!=nil{t.Fatal(err)}
 text:=string(result)
 if !strings.Contains(text,"postgres:18-alpine@"+digestB)||!strings.Contains(text,"company/app:7")||!strings.Contains(text,"restart: unless-stopped"){t.Fatalf("image pin changed unrelated service: %s",text)}
 if !strings.Contains(text,"openbao/openbao:2.7.0"){t.Fatal("unselected OpenBao modified")}
}
func TestRewriteOwnedComposeImagesFailsOnUnverifiedProvider(t *testing.T){
 source:=[]byte("services:\n  keycloak-1:\n    image: quay.io/keycloak/keycloak:26.7.0\n")
 d:=Delta{Installed:Realization{Kind:Identity,Installation:"c",Scope:"shared",Instance:"keycloak-1",Owner:"baseharbor",Image:"quay.io/keycloak/keycloak:26.8.0",Version:"26.8.0",Digest:digestA},Desired:Desired{Kind:Identity,Image:"quay.io/keycloak/keycloak:26.8.0",Version:"26.8.0",Digest:digestB},Classification:MigrationRequired}
 if _,err:=RewriteOwnedComposeImages(source,map[string]Delta{"keycloak-1":d});err==nil{t.Fatal("accepted stale Compose image")}
 d.Installed.Image="quay.io/keycloak/keycloak:26.7.0"
 d.Installed.Owner="operator"
 if _,err:=RewriteOwnedComposeImages(source,map[string]Delta{"keycloak-1":d});err==nil{t.Fatal("modified foreign Keycloak")}
 d.Installed.Owner="baseharbor"
 if _,err:=RewriteOwnedComposeImages(source,map[string]Delta{"keycloak-2":d});err==nil{t.Fatal("created missing Keycloak member")}
}
