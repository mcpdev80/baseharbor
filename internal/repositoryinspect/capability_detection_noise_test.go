package repositoryinspect

import (
	"context"
	"testing"
)

func TestSourceImportsAnyIgnoresPromptAndCommentText(t *testing.T) {
	signals := documentDatabaseSignals().imports

	cases := []struct {
		name string
		base string
		data string
		want bool
	}{
		{
			name: "python prompt string",
			base: "drive.py",
			data: "Rule(r\"MongoDB-compatible document database instances\", optional=True)\n",
			want: false,
		},
		{
			name: "python comment",
			base: "app.py",
			data: "# pymongo would be used here\nprint(\"hello\")\n",
			want: false,
		},
		{
			name: "python import",
			base: "app.py",
			data: "import pymongo\n",
			want: true,
		},
		{
			name: "python from import",
			base: "app.py",
			data: "from pymongo import MongoClient\n",
			want: true,
		},
		{
			name: "go import block",
			base: "main.go",
			data: "package main\nimport (\n  \"go.mongodb.org/mongo-driver/v2/mongo\"\n)\n",
			want: true,
		},
		{
			name: "javascript require",
			base: "app.js",
			data: "const mongodb = require(\"mongodb\")\n",
			want: true,
		},
		{
			name: "typescript import",
			base: "app.ts",
			data: "import { MongoClient } from \"mongodb\"\n",
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceImportsAny([]byte(tc.data), tc.base, signals); got != tc.want {
				t.Fatalf("sourceImportsAny() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDocumentDatabaseDetectorIgnoresGuidedPromptHarness(t *testing.T) {
	snapshot := Snapshot{
		Root: "/repo",
		Files: map[string][]byte{
			"tests/guided/drive.py": []byte(
				"Rule(r\"MongoDB-compatible document database instances (comma-separated)\", optional=True)\n",
			),
			"app/main.py": []byte("print(\"MongoDB-compatible is documentation text only\")\n"),
		},
	}

	findings, err := documentDatabaseDetector{}.Detect(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected document database findings from prompt/string noise: %#v", findings)
	}
}

func TestDocumentDatabaseDetectorKeepsRealImportEvidence(t *testing.T) {
	snapshot := Snapshot{
		Root: "/repo",
		Files: map[string][]byte{
			"app/main.py": []byte("from pymongo import MongoClient\n"),
		},
	}

	findings, err := documentDatabaseDetector{}.Detect(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v", findings)
	}
	if findings[0].Capability != "database.document" || findings[0].Confidence != ConfidenceSuggested {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
	if len(findings[0].Evidence) != 1 || findings[0].Evidence[0].Kind != EvidenceImport {
		t.Fatalf("unexpected evidence: %#v", findings[0].Evidence)
	}
}
