package coreupdate

import (
 "context"
 "errors"
 "strings"
 "testing"
)

type resumablePatroniFake struct {
 fakePatroniRoll
 steps map[string]string
 recovered int
}
func (f *resumablePatroniFake) StepState(_ context.Context,name string)(string,error){return f.steps[name],nil}
func (f *resumablePatroniFake) RecoverInterrupted(_ context.Context,name,state string)error {
 f.recovered++
 if state!="applying" {return errors.New("unexpected state")}
 return nil
}
func (f *resumablePatroniFake) Record(ctx context.Context,name,state string)error{
 f.steps[name]=state
 return f.fakePatroniRoll.Record(ctx,name,state)
}
func TestPatroniReplicaResumeAvoidsDuplicateMutation(t *testing.T){
 members:=[]PatroniMemberState{{Name:"pg1",Primary:true,Healthy:true},{Name:"pg2",Replica:true,Healthy:true},{Name:"pg3",Replica:true,Healthy:true}}
 f:=&resumablePatroniFake{fakePatroniRoll:fakePatroniRoll{members:members},steps:map[string]string{"pg2":"verified","pg3":"applying"}}
 err:=RollPatroniMembersResumable(context.Background(),f,0)
 if err==nil||!strings.Contains(err.Error(),"UNSUPPORTED"){t.Fatalf("primary safety guard lost: %v",err)}
 for _,call:=range f.calls {if strings.HasPrefix(call,"recreate:"){t.Fatalf("resume blindly restarted member: %v",f.calls)}}
 if f.recovered!=1||f.steps["pg3"]!="verified"{t.Fatalf("interrupted member recovery missing: %+v",f)}
}
func TestPatroniReplicaResumeRejectsUnknownJournal(t *testing.T){
 members:=[]PatroniMemberState{{Name:"pg1",Primary:true,Healthy:true},{Name:"pg2",Replica:true,Healthy:true},{Name:"pg3",Replica:true,Healthy:true}}
 f:=&resumablePatroniFake{fakePatroniRoll:fakePatroniRoll{members:members},steps:map[string]string{"pg2":"garbled"}}
 if err:=RollPatroniMembersResumable(context.Background(),f,0);err==nil{t.Fatal("unknown state accepted")}
 for _,call:=range f.calls{if strings.HasPrefix(call,"recreate:"){t.Fatal("mutated invalid journal")}}
}
