package repositoryinspect

import (
    "context"
    "testing"
)

func TestImportEvidenceRealSyntaxOnly(t *testing.T) {
    cases := []struct{name, file, source, capability string; want bool}{
        {"python import", "app.py", "import pymongo\n", "database.document", true},
        {"python from", "app.py", "from pymongo import MongoClient\n", "database.document", true},
        {"python prompt", "drive.py", "Rule(r\"MongoDB-compatible document database instances\", callback)\n", "database.document", false},
        {"python comment", "app.py", "# import pymongo\nvalue = 'import pymongo'\n", "database.document", false},
        {"python triple", "app.py", "text = '''\nimport pymongo\n'''\n", "database.document", false},
        {"go mongo", "app.go", "package example\nimport \"go.mongodb.org/mongo-driver/mongo\"\n", "database.document", true},
        {"go pgx", "app.go", "package example\nimport (\n \"github.com/jackc/pgx/v5\"\n)\n", "database.sql", true},
        {"go prose", "app.go", "package example\n// import \"go.mongodb.org/mongo-driver/mongo\"\nvar prompt = \"mongo-driver\"\n", "database.document", false},
        {"js import", "app.js", "import client from 'mongodb';\n", "database.document", true},
        {"ts export", "app.ts", "export { MongoClient } from 'mongodb';\n", "database.document", true},
        {"js require", "app.cjs", "const client = require('mongoose');\n", "database.document", true},
        {"js comments", "app.js", "// require('mongodb')\n/* import x from 'mongodb' */\n", "database.document", false},
        {"ts strings", "app.tsx", "const prompt = \"require('mongodb')\";\nconst re = /mongodb/;\n", "database.document", false},
        {"python redis", "app.py", "from redis import Redis\n", "cache.key-value", true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            signals := documentDatabaseSignals()
            if tc.capability == "database.sql" { signals = sqlSignals() }
            if tc.capability == "cache.key-value" { signals = keyValueSignals() }
            actual := sourceImportsCompatibleClient(tc.file, []byte(tc.source), signals.imports)
            if actual != tc.want { t.Fatalf("syntax matched %t, expected %t", actual, tc.want) }
            result, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{tc.file: []byte(tc.source)}}, tc.capability, signals)
            if err != nil { t.Fatal(err) }
            var evidence bool
            for _, finding := range result {
                for _, item := range finding.Evidence {
                    if item.Kind == EvidenceImport { evidence = true }
                }
            }
            if evidence != tc.want { t.Fatalf("import evidence=%v, want=%v; findings=%+v", evidence, tc.want, result) }
        })
    }
}
