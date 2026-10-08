package main

import (
 "testing"
 "github.com/mcpdev80/baseharbor/internal/application"
)

func TestAppCapabilityInstancesAreContractDerived(t *testing.T) {
 m:=application.Manifest{}
 for _,name:=range []string{"document-db","queue","pubsub","stream","storage","identity"}{
  got,provider,ok:=applicationCapabilityInstances(m,name)
  if !ok||provider==""||len(got)!=0{t.Errorf("%s empty manifest: instances=%v provider=%q supported=%t",name,got,provider,ok)}
 }
 if _,_,ok:=applicationCapabilityInstances(m,"unsupported");ok{t.Fatal("unsupported capability must not be accepted")}
 m=application.WithIdentity(m)
 names,provider,ok:=applicationCapabilityInstances(m,"identity")
 if !ok||provider!="oidc"||len(names)!=1||names[0]!="default"{t.Fatalf("identity contract: %v %s %t",names,provider,ok)}
}
