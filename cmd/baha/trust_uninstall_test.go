package main

import (
	"bytes"
	"context"
	"encoding/json"
 "os"
 "path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
)

func TestTrustUninstallCommandRegisteredAndExplicitConsent(t *testing.T) {
	cmd := trustCommand()
	var uninstall bool
	for _, child := range cmd.Children {
		if child.Name != "uninstall" {
			continue
		}
		uninstall = true
		if !strings.Contains(child.Usage, "--yes") {
			t.Fatalf("missing explicit consent in help: %q", child.Usage)
		}
		if !strings.Contains(child.Long, "only") {
			t.Fatalf("missing ownership boundary: %q", child.Long)
		}
		if err := child.Run(context.Background(), []string{"--unknown"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("unknown arguments silently accepted")
		}
	}
	if !uninstall {
		t.Fatal("trust uninstall not registered")
	}
}

func TestTrustUninstallMachineResultSchema(t *testing.T) {
	payload, err := json.Marshal(trustUninstallResult{ContractVersion: "v1", Removed: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"contract_version":"v1"`, `"removed":0`} {
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("missing %s: %s", expected, payload)
		}
	}
}

func TestDoctorHostTrustInvalidStateRequiresManualAction(t *testing.T) {
	findings := classifyDoctorFindings([]health.Check{{Name: "host-trust-ownership", OK: false, Message: "invalid ownership state"}})
	if len(findings) != 1 || findings[0].Class != doctorManualAction || !strings.Contains(findings[0].Action, "baha trust uninstall") {
		t.Fatalf("missing explicit trust cleanup guidance: %+v", findings)
	}
}

func TestTrustUninstallNoOwnershipIsSafeJSON(t *testing.T) {
 t.Setenv("BASEHARBOR_STATE_DIR",t.TempDir())
 configureTestTarget(t)
 var out,errOut bytes.Buffer
 err:=trustUninstallCommand().Run(context.Background(),[]string{"--json"},&out,&errOut)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(out.String(),`"removed":0`){t.Fatalf("invalid empty machine result: %s",out.String())}
}

func TestTrustUninstallRefusesUnapprovedJSONWithOwnedRecord(t *testing.T) {
 root:=t.TempDir()
 t.Setenv("BASEHARBOR_STATE_DIR",root)
 configureTestTarget(t)
 record:=`{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
 if err:=os.WriteFile(filepath.Join(root,"host-trust.json"),[]byte(record),0600);err!=nil{t.Fatal(err)}
 var out,errOut bytes.Buffer
 err:=trustUninstallCommand().Run(context.Background(),[]string{"--json"},&out,&errOut)
 if err==nil {t.Fatal("machine uninstall accepted without approval")}
 if !strings.Contains(err.Error(),"approval"){t.Fatalf("missing approval guidance: %v",err)}
 if _,err:=os.Stat(filepath.Join(root,"host-trust.json"));err!=nil{t.Fatalf("unapproved call removed ownership state: %v",err)}
}

func TestTrustUninstallWizardDeclinePreservesOwnedState(t *testing.T) {
 root:=t.TempDir()
 t.Setenv("BASEHARBOR_STATE_DIR",root)
 configureTestTarget(t)
 record:=`{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"/tmp/nonexistent-ca-test.crt","installed_at":"2026-10-08T12:00:00Z"}]}`
 if err:=os.WriteFile(filepath.Join(root,"host-trust.json"),[]byte(record),0600);err!=nil{t.Fatal(err)}
 saved:=guidedTrustInput
 guidedTrustInput=strings.NewReader("no\n")
 defer func(){guidedTrustInput=saved}()
 var out,errOut bytes.Buffer
 if err:=trustUninstallCommand().Run(context.Background(),nil,&out,&errOut);err!=nil{t.Fatal(err)}
 if !strings.Contains(out.String(),"unchanged"){t.Fatalf("missing cancellation notice: %s",out.String())}
 if _,err:=os.Stat(filepath.Join(root,"host-trust.json"));err!=nil{t.Fatalf("declined wizard mutated host trust ownership: %v",err)}
}
