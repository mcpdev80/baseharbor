package openbao

import "testing"

func TestUpgradePolicyCapabilitiesRequireExplicitRights(t *testing.T){
 if err:=verifyUpgradePolicyCapabilities(`["create","read","update","delete"]`);err!=nil{t.Fatal(err)}
 for _,reply:=range []string{`["read"]`,`["deny"]`,`{"capabilities":["read"]}`,`[]`,`null`} {
  if err:=verifyUpgradePolicyCapabilities(reply);err==nil{t.Fatalf("incomplete policy accepted: %s",reply)}
 }
}
