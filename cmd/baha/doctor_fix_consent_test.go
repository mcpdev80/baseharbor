package main
import (
 "bytes"
 "context"
 "strings"
 "testing"
)
func TestDoctorFixRequiresExplicitConsent(t *testing.T){
 var out,errOut bytes.Buffer
 err:=doctorCommand(context.Background(),[]string{"--yes"},&out,&errOut)
 if err==nil||!strings.Contains(err.Error(),"requires --fix"){t.Fatalf("unexpected --yes without fix: %v",err)}
 err=doctorCommand(context.Background(),[]string{"--fix","--json"},&out,&errOut)
 if err==nil||!strings.Contains(err.Error(),"read-only"){t.Fatalf("machine doctor may not mutate: %v",err)}
 err=doctorCommand(context.Background(),[]string{"--unknown"},&out,&errOut)
 if err==nil||!strings.Contains(err.Error(),"unknown argument"){t.Fatalf("unknown argument: %v",err)}
}
