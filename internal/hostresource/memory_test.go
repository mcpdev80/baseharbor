package hostresource

import (
	"os"
	"path/filepath"
	"testing"
)

func mib(v uint64) uint64 { return v << 20 }

func TestReadLinuxUsesMemAvailableAndPressure(t *testing.T) {
	root := t.TempDir()
	meminfo := filepath.Join(root, "meminfo")
	pressure := filepath.Join(root, "pressure")
	if err := os.WriteFile(meminfo, []byte("MemTotal: 8192 kB\nMemFree: 500 kB\nMemAvailable: 4096 kB\nSwapTotal: 2048 kB\nSwapFree: 1024 kB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pressure, []byte("some avg10=0.00 avg60=0.20 avg300=1.25 total=1\nfull avg10=0.00 avg60=0.10 avg300=0.75 total=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readLinuxFiles(meminfo, pressure)
	if err != nil {
		t.Fatal(err)
	}
	if got.AvailableBytes != 4096*1024 || got.TotalBytes != 8192*1024 {
		t.Fatalf("unexpected memory evidence: %#v", got)
	}
	if !got.Pressure.Available || got.Pressure.SomeAvg300 != 1.25 || got.Pressure.FullAvg300 != 0.75 {
		t.Fatalf("unexpected pressure: %#v", got.Pressure)
	}
}

func TestEvaluateUnsafeRequiresReliableMinimum(t *testing.T) {
	e := MemoryEvidence{AvailableBytes: mib(512)}
	estimate := MemoryEstimate{MinimumBytes: mib(768), EstimatedBytes: mib(1024), Confidence: ConfidenceKnownBaseline}
	got := Evaluate(e, estimate, DefaultPolicy())
	if got.Decision != DecisionUnsafe {
		t.Fatalf("decision = %s", got.Decision)
	}
}

func TestEvaluateEstimatedPressureIsTightNotUnsafe(t *testing.T) {
	e := MemoryEvidence{AvailableBytes: mib(700), SwapTotalBytes: mib(512), SwapFreeBytes: 0}
	estimate := MemoryEstimate{MinimumBytes: 0, EstimatedBytes: mib(600), Confidence: ConfidenceEstimated}
	got := Evaluate(e, estimate, DefaultPolicy())
	if got.Decision != DecisionTight {
		t.Fatalf("decision = %s, reasons=%v", got.Decision, got.Reasons)
	}
}

func TestEvaluateSafeWithHeadroom(t *testing.T) {
	e := MemoryEvidence{AvailableBytes: mib(4096)}
	estimate := MemoryEstimate{MinimumBytes: mib(512), EstimatedBytes: mib(1024), Confidence: ConfidenceKnownBaseline}
	got := Evaluate(e, estimate, DefaultPolicy())
	if got.Decision != DecisionSafe {
		t.Fatalf("decision = %s, reasons=%v", got.Decision, got.Reasons)
	}
}
