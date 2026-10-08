package main

import (
 "bytes"
 "context"
 "strings"
 "testing"
)

func TestClientConsentFailsClosedWithoutTerminal(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 var out bytes.Buffer
 err:=requireManagedClientConsent(context.Background(),strings.NewReader("yes\n"),&out,"dev","webshop","dev","postgres","default","managed-runtime")
 if err==nil {t.Fatal("unattended approval must be rejected")}
 if !strings.Contains(out.String(),"managed-runtime"){t.Fatalf("missing transparent method in prompt: %s",out.String())}
}

func TestClientConsentRejectsIncompleteScope(t *testing.T) {
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 err:=requireManagedClientConsent(context.Background(),nil,&bytes.Buffer{},"","webshop","dev","postgres","default","managed-runtime")
 if err==nil {t.Fatal("empty target cannot be approved")}
}
