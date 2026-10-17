package repositoryinspect

import (
	"context"
	"testing"
)

func TestSyntaxImportDetection(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		source  string
		signals []string
		want    bool
	}{
		{"python import", "app.py", "import pymongo\n", []string{"pymongo"}, true},
		{"python from", "app.py", "from pymongo import MongoClient\n", []string{"pymongo"}, true},
		{"python prompt", "drive.py", "Rule(r\"MongoDB-compatible document database instances\", callback)\n", []string{"mongodb"}, false},
		{"python comment", "app.py", "# import pymongo\nvalue = 'import pymongo'\n", []string{"pymongo"}, false},
		{"python triple", "app.py", "text = '''\nimport pymongo\n'''\n", []string{"pymongo"}, false},
		{"go mongo", "app.go", "package example\nimport \"go.mongodb.org/mongo-driver/mongo\"\n", []string{"mongo-driver"}, true},
		{"go pgx", "app.go", "package example\nimport (\n \"github.com/jackc/pgx/v5\"\n)\n", []string{"pgx"}, true},
		{"go prose", "app.go", "package example\n// import \"go.mongodb.org/mongo-driver/mongo\"\nvar prompt = \"mongo-driver\"\n", []string{"mongo-driver"}, false},
		{"js import", "app.js", "import client from 'mongodb';\n", []string{"mongodb"}, true},
		{"js import side effect", "app.js", "import 'mongodb';\n", []string{"mongodb"}, true},
		{"ts export", "app.ts", "export { MongoClient } from 'mongodb';\n", []string{"mongodb"}, true},
		{"js require", "app.cjs", "const client = require('mongoose');\n", []string{"mongoose"}, true},
		{"js dynamic", "app.mjs", "const c = import('mongodb');\n", []string{"mongodb"}, true},
		{"js comments", "app.js", "// require('mongodb')\n/* import x from 'mongodb' */\n", []string{"mongodb"}, false},
		{"js strings", "app.tsx", "const prompt = \"require('mongodb')\";\n", []string{"mongodb"}, false},
		{"js regex", "app.tsx", "const rx = /require('mongodb')/;\n", []string{"mongodb"}, false},
		{"python redis", "app.py", "from redis import Redis\n", []string{"redis"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := sourceImportsCompatibleClient(tc.file, []byte(tc.source), tc.signals)
			if actual != tc.want {
				t.Fatalf("got %v, expected %v", actual, tc.want)
			}
		})
	}
}

func TestImportEvidenceUsesOnlySyntax(t *testing.T) {
	for _, tc := range []struct {
		path, source string
		want         bool
	}{
		{"app.py", "from pymongo import MongoClient\n", true},
		{"tests/guided/drive.py", "Rule(r\"MongoDB-compatible document database instances\", callback)\n", false},
		{"app.go", "package demo\n// import \"go.mongodb.org/mongo-driver/mongo\"\n", false},
		{"app.ts", "const prompt = /require('mongodb')/;\n", false},
		{"app.ts", "const db = require('mongodb');\n", true},
	} {
		t.Run(tc.path+tc.source, func(t *testing.T) {
			findings, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{tc.path: []byte(tc.source)}}, "database.document", documentDatabaseSignals())
			if err != nil {
				t.Fatal(err)
			}
			evidence := false
			for _, finding := range findings {
				for _, item := range finding.Evidence {
					if item.Kind == EvidenceImport {
						evidence = true
					}
				}
			}
			if evidence != tc.want {
				t.Fatalf("import evidence=%v, want=%v, findings=%+v", evidence, tc.want, findings)
			}
		})
	}
}

func TestImportDetectorRetainsDependencyMetadata(t *testing.T) {
	findings, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{"package.json": []byte(`{"dependencies":{"mongodb":"6.0.0"}}`)}}, "database.document", documentDatabaseSignals())
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		for _, item := range finding.Evidence {
			if item.Kind == EvidenceDependency {
				return
			}
		}
	}
	t.Fatalf("dependency evidence was lost: %+v", findings)
}
