package main

import (
 "bytes"
 "strings"
 "testing"
)

func TestDoctorRepairConsentWizardDefaultDeny(t *testing.T) {
 cases:=[]struct{input string;accepted bool}{
  {"\n",false},{"no\n",false},{"cancel\n",false},{"y\n",true},{"ja\n",true},
 }
 for _,c:=range cases{
  var out bytes.Buffer
  accepted,err:=confirmDoctorRepair(strings.NewReader(c.input),&out)
  if err!=nil {t.Fatal(err)}
  if accepted!=c.accepted {t.Fatalf("input %q accepted %v",c.input,accepted)}
  if !strings.Contains(out.String(),"EXISTING selected Core") {t.Fatalf("scope of operation omitted: %s",out.String())}
 }
}

func TestDoctorRepairConsentIncompleteInputFailsClosed(t *testing.T){
 approved,err:=confirmDoctorRepair(strings.NewReader("yes"),&bytes.Buffer{})
 if approved||err==nil{t.Fatalf("unconfirmed input accepted=%v err=%v",approved,err)}
}
