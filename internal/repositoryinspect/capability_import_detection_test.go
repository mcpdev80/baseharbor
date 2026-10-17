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
		{"python comma import", "app.py", "import os, pymongo\n", []string{"pymongo"}, true},
		{"python aliases", "app.py", "import os as operating_system, pymongo as mongo\n", []string{"pymongo"}, true},
		{"python comma absent", "app.py", "import os, sys as system\n", []string{"pymongo"}, false},
		{"python commented list", "app.py", "import os # , pymongo\n", []string{"pymongo"}, false},
		{"js regex return", "app.ts", "function probe() { return /import 'mongodb'/; }\n", []string{"mongodb"}, false},
		{"js regex throw", "app.ts", "function probe() { throw /require('mongodb')/; }\n", []string{"mongodb"}, false},
		{"js regex case", "app.ts", "switch (x) { case /import 'mongodb'/: break; }\n", []string{"mongodb"}, false},
		{"js regex yield", "app.ts", "function* probe() { yield /import 'mongodb'/; }\n", []string{"mongodb"}, false},
		{"js regex grouped", "app.js", "const value = (/import 'mongodb'/).test(text);\n", []string{"mongodb"}, false},
		{"js method require", "app.js", "const obj = { require() { return 1 } }; obj.require('mongodb');\n", []string{"mongodb"}, false},
		{"js optional method require", "app.js", "obj?.require('mongodb');\n", []string{"mongodb"}, false},
		{"js commonjs", "app.js", "const db = require('mongodb');\n", []string{"mongodb"}, true},
		{"js dynamic import", "app.ts", "const db = import('mongodb');\n", []string{"mongodb"}, true},
		{"js if regex statement", "app.js", "if (ready) /import 'mongodb'/.test(text);\n", []string{"mongodb"}, false},
		{"js while regex statement", "app.js", "while (ready) /import 'mongodb'/.test(text);\n", []string{"mongodb"}, false},
		{"js for regex statement", "app.js", "for (; ready; ) /import 'mongodb'/.test(text);\n", []string{"mongodb"}, false},
		{"js nested control regex", "app.ts", "if ((ready)) /import 'mongodb'/.test(text);\n", []string{"mongodb"}, false},
		{"js call result division", "app.ts", "const n = calculate(ready) / divisor;\n", []string{"mongodb"}, false},
		{"js control then real import", "app.ts", "if (ready) /import 'mongodb'/.test(text);\nimport { MongoClient } from 'mongodb';\n", []string{"mongodb"}, true},
		{"js division then real import", "app.js", "const n = calculate(ready) / divisor;\nconst db = require('mongodb');\n", []string{"mongodb"}, true},
		{"js else regex", "app.js", "if (ready) done(); else /import 'mongodb'/.test(text);", []string{"mongodb"}, false},
		{"js else newline regex", "app.ts", "if (ready) done(); else\n /import 'mongodb'/.test(text);", []string{"mongodb"}, false},
		{"js do regex", "app.js", "do /import 'mongodb'/.test(text); while (ready);", []string{"mongodb"}, false},
		{"js multiline import", "app.ts", "import {\n MongoClient\n} from\n 'mongodb';", []string{"mongodb"}, true},
		{"js multiline side effect", "app.js", "import\n 'mongodb';", []string{"mongodb"}, true},
		{"js multiline reexport", "app.ts", "export {\n MongoClient\n} from\n 'mongodb';", []string{"mongodb"}, true},
		{"js multiline unrelated import", "app.ts", "import {\n Something\n} from\n 'unrelated';", []string{"mongodb"}, false},
		{"js multiline comment import", "app.js", "// import {\n// X\n// } from 'mongodb';", []string{"mongodb"}, false},
		{"js newline division", "app.js", "const n = compute(ready)\n / divisor;", []string{"mongodb"}, false},
		{"js import after division", "app.ts", "const n = compute(ready)\n / divisor;\nimport {\n MongoClient\n} from 'mongodb';", []string{"mongodb"}, true},
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

func TestReview984DirectAndEvidenceImport(t *testing.T) {
	cases := []struct {
		name, path, source string
		want               bool
	}{
		{"block regex", "app.js", "if (ready) {} /import 'mongodb'/.test(text);", false},
		{"multiline control", "app.js", "if\n(ready) /import 'mongodb'/.test(text);", false},
		{"completed import", "app.js", "import 'other'\nfrom\n'mongodb'", false},
		{"division require", "app.js", "const ratio = 10 / require('mongodb').value;", true},
		{"template require", "app.js", "const message = `result: ${require('mongodb').value}`;", true},
		{"template prose", "app.js", "const message = `require('mongodb')`;", false},
		{"python semicolon", "app.py", "import os; import pymongo", true},
		{"python suite", "app.py", "if enabled: import pymongo", true},
		{"python continuation", "app.py", "import os, \\\n pymongo", true},
		{"java import", "app.java", "import com.mongodb.client.MongoClient;", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.source)
			if got := sourceImportsCompatibleClient(tc.path, data, documentDatabaseSignals().imports); got != tc.want {
				t.Errorf("direct got %t, want %t", got, tc.want)
			}
			findings, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{tc.path: data}}, "database.document", documentDatabaseSignals())
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, finding := range findings {
				for _, e := range finding.Evidence {
					got = got || e.Kind == EvidenceImport
				}
			}
			if got != tc.want {
				t.Errorf("EvidenceImport got %t, want %t: %+v", got, tc.want, findings)
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
		{"app.ts", "function probe() { return /import 'mongodb'/; }\n", false},
		{"app.ts", "obj.require('mongodb');\n", false},
		{"app.ts", "if (ready) /import 'mongodb'/.test(text);\n", false},
		{"app.ts", "while (ready) /import 'mongodb'/.test(text);\n", false},
		{"app.js", "for (; ready; ) /import 'mongodb'/.test(text);\n", false},
		{"app.ts", "const n = calculate(ready) / divisor;\nconst db = require('mongodb');\n", true},
		{"app.ts", "if (ready) done(); else /import 'mongodb'/.test(text);", false},
		{"app.js", "if (ready) done(); else\n/import 'mongodb'/.test(text);", false},
		{"app.js", "import {\nMongoClient\n} from\n'mongodb';", true},
		{"app.ts", "export {\nMongoClient\n} from\n'mongodb';", true},
		{"app.ts", "import {\nOther\n} from\n'other';", false},
		{"app.ts", "const n = compute(ready)\n / divisor;", false},
		{"app.py", "import os, pymongo as mongo\n", true},
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

// Operator and computed-member syntax must survive tokenization.
func TestJSOperatorAndMemberMatrix(t *testing.T) {
	cases := []struct {
		name, source string
		want         bool
	}{
		{"and regex", "const rx = ready && /import 'mongodb'/;", false},
		{"or regex", "const rx = ready || /require('mongodb')/;", false},
		{"ternary regex", "const rx = ready ? /import 'mongodb'/ : /require('mongodb')/;", false},
		{"computed require", "obj[require]('mongodb');", false},
		{"computed import", "obj[import]('mongodb');", false},
		{"optional computed require", "obj?.[require]('mongodb');", false},
		{"direct import", "const db = import('mongodb');", true},
		{"direct require", "const db = require('mongodb');", true},
		{"multiline import", "const db = import\n(\n'mongodb'\n);", true},
		{"static declaration", "import {DB} from\n'mongodb';", true},
		{"re-export declaration", "export {DB} from\n'mongodb';", true},
		{"division and import", "const n = calculate(x) / other;\nconst db = require('mongodb');", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.source)
			if got := sourceImportsCompatibleClient("app.ts", data, []string{"mongodb"}); got != tc.want {
				t.Fatalf("direct: got %t, want %t", got, tc.want)
			}
			findings, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{"app.ts": data}}, "database.document", documentDatabaseSignals())
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, finding := range findings {
				for _, evidence := range finding.Evidence {
					got = got || evidence.Kind == EvidenceImport
				}
			}
			if got != tc.want {
				t.Fatalf("EvidenceImport: got %t, want %t; findings=%+v", got, tc.want, findings)
			}
		})
	}
}

// The same cases must pass both syntax detection and actual EvidenceImport.
func TestJSModuleReferenceMatrix(t *testing.T) {
	cases := []struct {
		name, source string
		want         bool
	}{
		{"static normal", "import client from 'mongodb';", true},
		{"static multiline", "import {\nClient\n} from\n'mongodb';", true},
		{"static comments", "import {Client} /* note */ from /* note */ 'mongodb';", true},
		{"static member", "obj.import('mongodb');", false},
		{"dynamic normal", "const db = import('mongodb');", true},
		{"dynamic multiline", "const db = import\n(\n'mongodb'\n);", true},
		{"dynamic comments", "const db = import/* note */( /* note */ 'mongodb' /* note */ );", true},
		{"dynamic member", "obj.import('mongodb');", false},
		{"require normal", "const db = require('mongodb');", true},
		{"require multiline", "const db = require\n(\n'mongodb'\n);", true},
		{"require comments", "const db = require/* note */( /* note */ 'mongodb' /* note */ );", true},
		{"require member", "obj.require('mongodb');", false},
		{"member optional import", "obj?.import('mongodb');", false},
		{"member optional require", "obj?.require('mongodb');", false},
		{"reexport multiline comments", "export {Client}\nfrom /* note */ 'mongodb';", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.source)
			if got := sourceImportsCompatibleClient("app.ts", data, []string{"mongodb"}); got != tc.want {
				t.Fatalf("direct syntax: got %t, want %t", got, tc.want)
			}
			findings, err := detectCapability(context.Background(), Snapshot{Files: map[string][]byte{"app.ts": data}}, "database.document", documentDatabaseSignals())
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, finding := range findings {
				for _, evidence := range finding.Evidence {
					if evidence.Kind == EvidenceImport {
						found = true
					}
				}
			}
			if found != tc.want {
				t.Fatalf("EvidenceImport: got %t, want %t; findings=%+v", found, tc.want, findings)
			}
		})
	}
}
