package repositoryinspect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectSnapshotSkipsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	target := filepath.Join(outside, "compose.yaml")
	if err := os.WriteFile(target, []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "compose.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	snapshot, _, err := collectSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot.Files["compose.yaml"]; ok {
		t.Fatal("symlinked file outside repository root was followed")
	}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("unexpected candidates from symlink escape: %#v", candidates)
	}
}

func TestCollectSnapshotSkipsOversizedRelevantFile(t *testing.T) {
	root := t.TempDir()
	data := strings.Repeat("x", maxInspectionFileSize+1)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, _, err := collectSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot.Files["compose.yaml"]; ok {
		t.Fatal("oversized inspection file was read")
	}
}

func TestInspectionBudgetFileLimitFailsClosed(t *testing.T) {
	var budget inspectionBudget
	for i := 0; i < maxInspectionFiles; i++ {
		if err := budget.add(1); err != nil {
			t.Fatalf("unexpected early budget failure at %d: %v", i, err)
		}
	}
	if err := budget.add(1); err == nil || !strings.Contains(err.Error(), "file limit exceeded") {
		t.Fatalf("expected file-limit error, got %v", err)
	}
}

func TestInspectionBudgetByteLimitFailsClosed(t *testing.T) {
	var budget inspectionBudget
	if err := budget.add(maxInspectionTotalBytes); err != nil {
		t.Fatal(err)
	}
	if err := budget.add(1); err == nil || !strings.Contains(err.Error(), "size limit exceeded") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}

func TestKubernetesDocumentLimitFailsClosed(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= maxKubernetesDocumentsPerFile; i++ {
		if i > 0 {
			b.WriteString("---\n")
		}
		b.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n")
	}
	snapshot := Snapshot{Files: map[string][]byte{
		"k8s/many.yaml": []byte(b.String()),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v", candidates)
	}
	if _, err := NormalizeWorkloadSource(snapshot, candidates[0]); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected document-limit failure, got %v", err)
	}
}
