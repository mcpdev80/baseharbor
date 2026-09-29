package podman

import "testing"

func TestParsePodmanPublishedPorts(t *testing.T) {
	got, err := parsePodmanPublishedPorts(`[{"NetworkSettings":{"Ports":{"8080/tcp":[{"HostIp":"127.0.0.1","HostPort":"18080"}],"9090/udp":[{"HostIp":"0.0.0.0","HostPort":"19090"}]}}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("publishers = %d, want 2: %+v", len(got), got)
	}
	if got[0].TargetPort != 8080 || got[0].PublishedPort != 18080 || got[0].URL != "127.0.0.1" || got[0].Protocol != "tcp" {
		t.Fatalf("first publisher = %+v", got[0])
	}
	if got[1].TargetPort != 9090 || got[1].PublishedPort != 19090 || got[1].URL != "0.0.0.0" || got[1].Protocol != "udp" {
		t.Fatalf("second publisher = %+v", got[1])
	}
}

func TestParsePodmanPublishedPortsRejectsInvalidInspect(t *testing.T) {
	for _, input := range []string{"", "{}", "[]", "[{},{}]"} {
		if _, err := parsePodmanPublishedPorts(input); err == nil {
			t.Fatalf("parsePodmanPublishedPorts(%q) unexpectedly succeeded", input)
		}
	}
}
