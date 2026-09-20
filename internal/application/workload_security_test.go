package application

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComposeSecurityManagedDeniesIsolationBypass(t *testing.T) {
	m := New("demo", "production", false, false, false)
	rendered := []byte(`{
	  "services": {
	    "api": {
	      "privileged": true,
	      "network_mode": "host",
	      "pid": "host",
	      "ipc": "host",
	      "cap_add": ["SYS_ADMIN"],
	      "devices": [{"source":"/dev/kvm","target":"/dev/kvm"}],
	      "volumes": [
	        {"type":"bind","source":"/var/run/docker.sock","target":"/var/run/docker.sock"},
	        {"type":"bind","source":"/etc","target":"/host-etc"}
	      ]
	    }
	  }
	}`)
	report, err := AnalyzeRenderedComposeSecurity(m, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Denied() {
		t.Fatalf("expected managed policy deny: %#v", report)
	}
	if len(report.Findings) != 8 {
		t.Fatalf("findings=%d want=8: %#v", len(report.Findings), report.Findings)
	}
	for _, finding := range report.Findings {
		if finding.Decision != WorkloadSecurityDeny {
			t.Fatalf("managed finding not denied: %#v", finding)
		}
	}
	if err := report.Error(); err == nil || !strings.Contains(err.Error(), `"decision":"deny"`) {
		t.Fatalf("machine-readable deny missing: %v", err)
	}
}

func TestComposeSecurityDevelopmentWarnsDevicesAndAllowsAcknowledgedException(t *testing.T) {
	t.Setenv(WorkloadSecurityAllowEnv, "host-device,privileged")
	m := New("demo", "dev", false, false, false)
	rendered := []byte(`{"services":{"api":{"privileged":true,"devices":[{"source":"/dev/kvm","target":"/dev/kvm"}]}}}`)
	report, err := AnalyzeRenderedComposeSecurity(m, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if report.Denied() {
		t.Fatalf("acknowledged development exceptions should not deny: %#v", report)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("findings=%#v", report.Findings)
	}
	for _, finding := range report.Findings {
		if finding.Decision != WorkloadSecurityAllow {
			t.Fatalf("finding decision=%s want allow: %#v", finding.Decision, finding)
		}
	}
}

func TestComposeSecurityDevelopmentWarnsUnacknowledgedDevice(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	report, err := AnalyzeRenderedComposeSecurity(m, []byte(`{"services":{"api":{"devices":[{"source":"/dev/kvm","target":"/dev/kvm"}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if report.Denied() {
		t.Fatalf("device-only development workload should warn: %#v", report)
	}
	if len(report.Findings) != 1 || report.Findings[0].Decision != WorkloadSecurityWarn {
		t.Fatalf("findings=%#v", report.Findings)
	}
}

func TestComposeSecurityRejectsDevelopmentAllowInManagedMode(t *testing.T) {
	t.Setenv(WorkloadSecurityAllowEnv, "privileged")
	m := New("demo", "production", false, false, false)
	if _, err := AnalyzeRenderedComposeSecurity(m, []byte(`{"services":{"api":{}}}`)); err == nil {
		t.Fatal("managed environment accepted development-only security exception")
	}
}

func TestComposeSecurityReportIsMachineReadable(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	report, err := AnalyzeRenderedComposeSecurity(m, []byte(`{"services":{"api":{"network_mode":"host"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mode":"development"`, `"code":"host-network"`, `"service":"api"`, `"field":"network_mode"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("report missing %s: %s", want, data)
		}
	}
}

func TestComposeSecurityAllowsOrdinaryWorkload(t *testing.T) {
	m := New("demo", "production", false, false, false)
	rendered := []byte(`{"services":{"api":{"cap_add":["CHOWN"],"volumes":[{"type":"volume","source":"data","target":"/data"}]}}}`)
	report, err := AnalyzeRenderedComposeSecurity(m, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if report.Denied() || len(report.Findings) != 0 {
		t.Fatalf("ordinary workload unexpectedly flagged: %#v", report)
	}
}
