package repositoryinspect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectionBudgetRejectsTooManyRelevantFiles(t *testing.T) {
	budget := inspectionBudget{files: maxInspectionFiles}
	if err := budget.add(1); err == nil || !strings.Contains(err.Error(), "file limit exceeded") {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectionBudgetRejectsTooManyBytes(t *testing.T) {
	budget := inspectionBudget{bytes: maxInspectionTotalBytes}
	if err := budget.add(1); err == nil || !strings.Contains(err.Error(), "size limit exceeded") {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectionSkipsSourcesBeyondTraversalDepth(t *testing.T) {
	root := t.TempDir()
	parts := make([]string, maxInspectionDepth+1)
	for i := range parts {
		parts[i] = fmt.Sprintf("d%d", i)
	}
	deep := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkloadSourceResolution.State != WorkloadSourceResolutionNotDetected {
		t.Fatalf("resolution = %#v", result.WorkloadSourceResolution)
	}
}

func TestKubernetesDocumentLimitFailsExplicitly(t *testing.T) {
	var content strings.Builder
	for i := 0; i <= maxKubernetesDocumentsPerFile; i++ {
		if i > 0 {
			content.WriteString("---\n")
		}
		fmt.Fprintf(&content, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config-%d\n", i)
	}
	snapshot := Snapshot{Files: map[string][]byte{"k8s/many.yaml": []byte(content.String())}}
	candidate := WorkloadSourceCandidate{
		Kind:       WorkloadSourceKubernetes,
		Path:       "k8s",
		Confidence: ConfidenceDetected,
		Evidence:   []string{"k8s/many.yaml"},
	}
	if _, err := NormalizeWorkloadSource(snapshot, candidate); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}
