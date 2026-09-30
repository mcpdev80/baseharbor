package resourcepreflight

import "testing"

func TestParseMeminfoAndPressure(t *testing.T) {
	host, err := ParseMeminfo([]byte("MemTotal: 8192000 kB\nMemAvailable: 4096000 kB\nSwapTotal: 1024000 kB\nSwapFree: 102400 kB\n"))
	if err != nil { t.Fatal(err) }
	if host.AvailableBytes != 4096000*1024 { t.Fatalf("available=%d", host.AvailableBytes) }
	p, err := ParseMemoryPressure([]byte("some avg10=2.00 avg60=1.50 avg300=0.50 total=1\nfull avg10=0.20 avg60=0.10 avg300=0.05 total=1\n"))
	if err != nil { t.Fatal(err) }
	if p.SomeAvg60 != 1.50 || p.FullAvg60 != 0.10 { t.Fatalf("pressure=%#v", p) }
}

func TestAssessHardFailureUsesReliableMinimumOnly(t *testing.T) {
	host := HostMemory{TotalBytes: 8*GiB, AvailableBytes: 1*GiB}
	a := Assess(host, []Requirement{
		{ID:"known", MinimumBytes: 1200*MiB, EstimatedBytes: 1500*MiB, Confidence: ConfidenceKnownBaseline},
		{ID:"guess", EstimatedBytes: 4*GiB, Confidence: ConfidenceEstimated},
	})
	if a.Decision != DecisionUnsafe || a.ProblemCode != "host_memory_insufficient" { t.Fatalf("%#v", a) }

	b := Assess(host, []Requirement{{ID:"guess", EstimatedBytes: 4*GiB, Confidence: ConfidenceEstimated}})
	if b.Decision == DecisionUnsafe { t.Fatalf("estimate alone hard-failed: %#v", b) }
}

func TestAssessDoesNotCountAlreadyRunning(t *testing.T) {
	host := HostMemory{TotalBytes: 4*GiB, AvailableBytes: 2*GiB}
	a := Assess(host, []Requirement{
		{ID:"running", MinimumBytes: 3*GiB, EstimatedBytes: 3*GiB, Confidence: ConfidenceKnownBaseline, AlreadyRunning:true},
	})
	if a.RequiredMinimumBytes != 0 || a.EstimatedBytes != 0 || a.Decision != DecisionSafe { t.Fatalf("%#v", a) }
}

func TestComposeMemoryDeclarationsAndUnknown(t *testing.T) {
	data := []byte(`services:
  api:
    mem_limit: 512m
  worker:
    deploy:
      resources:
        limits:
          memory: 1GiB
  unknown:
    image: alpine
`)
	reqs, err := ParseComposeWorkloadMemory(data, []string{"api","worker","unknown"})
	if err != nil { t.Fatal(err) }
	if reqs[0].Confidence != ConfidenceExplicit || reqs[0].MinimumBytes != 512000000 { t.Fatalf("api=%#v", reqs[0]) }
	if reqs[1].MinimumBytes != GiB { t.Fatalf("worker=%#v", reqs[1]) }
	if reqs[2].Confidence != ConfidenceUnknown { t.Fatalf("unknown=%#v", reqs[2]) }
}

func TestTightFromPressureAndUnknownDoesNotHardFail(t *testing.T) {
	host := HostMemory{TotalBytes: 16*GiB, AvailableBytes: 8*GiB, Pressure:&MemoryPressure{SomeAvg60:1.2}}
	a := Assess(host, []Requirement{{ID:"unknown", Confidence:ConfidenceUnknown}})
	if a.Decision != DecisionTight || a.UnknownCount != 1 { t.Fatalf("%#v", a) }
}
