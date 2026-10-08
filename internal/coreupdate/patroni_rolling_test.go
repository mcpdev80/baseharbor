package coreupdate

import (
 "context"
 "errors"
 "strings"
 "testing"
)

type fakePatroniRoll struct {
 members []PatroniMemberState
 calls []string
 fail string
}
func (f *fakePatroniRoll) VerifyRecovery(context.Context)error {
 f.calls=append(f.calls,"backup")
 if f.fail=="backup"{return errors.New("missing DCS snapshot")}
 return nil
}
func (f *fakePatroniRoll) Inspect(context.Context)([]PatroniMemberState,error){
 f.calls=append(f.calls,"inspect")
 return f.members,nil
}
func (f *fakePatroniRoll) Recreate(_ context.Context,name string)error{
 f.calls=append(f.calls,"recreate:"+name)
 if f.fail=="recreate"{return errors.New("restart failed")}
 return nil
}
func (f *fakePatroniRoll) VerifyMemberImage(_ context.Context,name string)error {
 f.calls=append(f.calls,"image:"+name)
 if f.fail=="image"{return errors.New("wrong digest")}
 return nil
}
func (f *fakePatroniRoll) Record(_ context.Context,name,state string)error {
 f.calls=append(f.calls,state+":"+name)
 if f.fail=="journal"{return errors.New("journal write failed")}
 return nil
}
func TestPatroniRollingRequiresRecoveryBeforeMutation(t *testing.T){
 f:=&fakePatroniRoll{fail:"backup"}
 if err:=RollPatroniMembers(context.Background(),f,0);err==nil{t.Fatal("missing backup accepted")}
 if strings.Contains(strings.Join(f.calls,","),"recreate"){t.Fatal("mutated without recovery")}
}
func TestPatroniRollingReplicasBeforePrimaryAndStop(t *testing.T) {
 f:=&fakePatroniRoll{members:[]PatroniMemberState{
  {Name:"postgres-member-1",Primary:true,Healthy:true},
  {Name:"postgres-member-2",Replica:true,Healthy:true},
  {Name:"postgres-member-3",Replica:true,Healthy:true},
 }}
 err:=RollPatroniMembers(context.Background(),f,0)
 if err==nil||!strings.Contains(err.Error(),"UNSUPPORTED"){t.Fatalf("primary switchover guard missing: %v",err)}
 all:=strings.Join(f.calls,",")
 if strings.Contains(all,"recreate:postgres-member-1"){t.Fatal("primary recreated without switchover")}
 if !strings.Contains(all,"verified:postgres-member-2")||!strings.Contains(all,"verified:postgres-member-3"){t.Fatalf("replica confirmations missing: %s",all)}
}
func TestPatroniRollingPreflightGuards(t *testing.T) {
 for _,failure:=range []string{"journal","recreate","image"}{
  t.Run(failure,func(t *testing.T){
   f:=&fakePatroniRoll{fail:failure,members:[]PatroniMemberState{
    {Name:"postgres-member-1",Primary:true,Healthy:true},
    {Name:"postgres-member-2",Replica:true,Healthy:true},
    {Name:"postgres-member-3",Replica:true,Healthy:true},
   }}
   if err:=RollPatroniMembers(context.Background(),f,0);err==nil{t.Fatal("partial failure accepted")}
   if strings.Contains(strings.Join(f.calls,","),"recreate:postgres-member-1"){t.Fatal("primary mutated")}
  })
 }
}
