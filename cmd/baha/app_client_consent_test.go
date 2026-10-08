package main

import (
 "bytes"
 "context"
 "os"
 "strings"
 "testing"
)

func TestClientConsentFailsClosedWithoutTerminal(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 var out bytes.Buffer
 pipe,writer,pipeErr:=os.Pipe()
 if pipeErr!=nil { t.Fatal(pipeErr) }
 defer pipe.Close()
 if _,err:=writer.Write([]byte("yes\n"));err!=nil{t.Fatal(err)}
 writer.Close()
 err:=requireManagedClientConsent(context.Background(),pipe,&out,"dev","webshop","dev","postgres","default","managed-runtime")
 if err==nil {t.Fatal("unattended approval must be rejected")}
 if !strings.Contains(out.String(),"managed-runtime"){t.Fatalf("missing transparent method in prompt: %s",out.String())}
}

func TestClientConsentRejectsIncompleteScope(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 err:=requireManagedClientConsent(context.Background(),nil,&bytes.Buffer{},"","webshop","dev","postgres","default","managed-runtime")
 if err==nil {t.Fatal("empty target cannot be approved")}
}
