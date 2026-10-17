package repositoryinspect

import (
	"bufio"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"path/filepath"
	"strings"
	"unicode"
)

type sqlDetector struct{}

func (sqlDetector) Name() string { return "database.sql" }

func (sqlDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "database.sql", sqlSignals())
}

type keyValueDetector struct{}

func (keyValueDetector) Name() string { return "cache.key-value" }

func (keyValueDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "cache.key-value", keyValueSignals())
}

type documentDatabaseDetector struct{}

func (documentDatabaseDetector) Name() string { return "database.document" }

func (documentDatabaseDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "database.document", documentDatabaseSignals())
}

type signalSet struct {
	env        []string
	compose    []string
	dependency []string
	imports    []string
	config     []string
}

func sqlSignals() signalSet {
	return signalSet{
		env:     []string{"DATABASE_URL", "POSTGRES_URL", "POSTGRESQL_URL"},
		compose: []string{"postgres", "postgresql"},
		dependency: []string{
			"pg", "postgres", "postgresql", "pgx", "psycopg", "asyncpg", "sqlalchemy",
		},
		imports: []string{"pgx", "psycopg", "asyncpg", "sequelize", "typeorm", "prisma", "postgres"},
		config:  []string{"jdbc:postgresql:", "postgresql://", "postgres://"},
	}
}

func keyValueSignals() signalSet {
	return signalSet{
		env:     []string{"REDIS_URL", "VALKEY_URL"},
		compose: []string{"redis", "valkey"},
		dependency: []string{
			"ioredis", "redis", "go-redis", "valkey",
		},
		imports: []string{"ioredis", "redis", "go-redis", "valkey"},
		config:  []string{"redis://", "rediss://"},
	}
}

func documentDatabaseSignals() signalSet {
	return signalSet{
		env:     []string{"MONGODB_URL", "MONGO_URL"},
		compose: []string{"mongodb", "mongo"},
		dependency: []string{
			"mongodb", "mongo-driver", "mongoose", "pymongo", "motor",
		},
		imports: []string{"mongodb", "mongo-driver", "mongoose", "pymongo", "motor"},
		config:  []string{"mongodb://", "mongodb+srv://"},
	}
}

func detectCapability(ctx context.Context, snapshot Snapshot, capability string, signals signalSet) ([]Finding, error) {
	var detected, suggested, possible []Evidence
	namedDetected := map[string][]Evidence{}
	for path, data := range snapshot.Files {
		if ctx.Err() != nil {
			break
		}
		lower := strings.ToLower(string(data))
		base := strings.ToLower(filepath.Base(path))
		if isComposeFile(base) {
			services, detectErr := detectComposeServices(data)
			if detectErr != nil {
				return nil, fmt.Errorf("inspect Compose file %s: %w", path, detectErr)
			}
			for _, service := range services {
				matches := false
				instanceKind := ""
				switch capability {
				case "database.sql":
					matches = service.Postgres
					instanceKind = "postgres"
				case "cache.key-value":
					matches = service.Redis
					instanceKind = "redis"
				case "database.document":
					matches = service.MongoDB
					instanceKind = "mongodb"
				default:
					matches = containsAny(strings.ToLower(service.Name), signals.compose)
				}
				if matches {
					evidence := Evidence{
						Kind: EvidenceCompose, Path: path,
						Detail: "compose service " + service.Name + " matches " + capability,
					}
					if instanceKind != "" {
						name := detectedLogicalInstanceName(service.Name, instanceKind)
						namedDetected[name] = append(namedDetected[name], evidence)
					} else {
						detected = append(detected, evidence)
					}
				}
			}
		}
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				if containsExactFold(signals.env, name) {
					detected = append(detected, Evidence{
						Kind: EvidenceEnv, Path: path, Detail: "variable " + name,
					})
				}
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, signals.dependency) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references a compatible client/provider",
			})
		}
		if isSourceFile(base) && sourceImportsCompatibleClient(base, data, signals.imports) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceImport, Path: path,
				Detail: "source imports/references a compatible client",
			})
		}
		if !isEnvFile(base) && containsAny(lower, signals.config) {
			possible = append(possible, Evidence{
				Kind: EvidenceConfig, Path: path,
				Detail: "configuration contains a compatible endpoint pattern",
			})
		}
	}

	var findings []Finding
	for name, evidence := range namedDetected {
		findings = append(findings, Finding{
			Capability: capability,
			Name:       name,
			Confidence: ConfidenceDetected,
			Evidence:   uniqueEvidence(evidence),
		})
	}
	if len(detected) > 0 {
		findings = append(findings, Finding{
			Capability: capability,
			Confidence: ConfidenceDetected,
			Evidence:   uniqueEvidence(detected),
		})
	}
	if len(findings) > 0 {
		return findings, nil
	}
	if len(suggested) > 0 {
		return []Finding{{Capability: capability, Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(suggested)}}, nil
	}
	if len(possible) > 0 {
		return []Finding{{Capability: capability, Confidence: ConfidencePossible, Evidence: uniqueEvidence(possible)}}, nil
	}
	return nil, nil
}

func composeAmbiguousInfrastructureMarker(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "db", "database", "cache", "storage", "object-storage", "s3":
		return true
	default:
		return false
	}
}

func composeObjectStorageMarker(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"minio/minio", "seaweedfs", "chrislusf/seaweedfs", "radosgw", "ceph-rgw"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return value == "minio" || value == "seaweedfs" || value == "radosgw"
}

func readEnvNames(data []byte) []string {
	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		trim := strings.TrimSpace(scanner.Text())
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		trim = strings.TrimPrefix(trim, "export ")
		name, _, ok := strings.Cut(trim, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if validEnvName(name) {
			names = append(names, name)
		}
	}
	return uniqueSorted(names)
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

func likelySecretName(name string) bool {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "PUBLIC") || strings.HasSuffix(upper, "_URL") ||
		strings.HasSuffix(upper, "_HOST") || strings.HasSuffix(upper, "_PORT") {
		return false
	}
	return upper == "SECRET_KEY" || strings.Contains(upper, "PASSWORD") ||
		strings.HasSuffix(upper, "_SECRET") || strings.HasSuffix(upper, "_TOKEN") ||
		strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_PRIVATE_KEY")
}

var pythonImportLine = regexp.MustCompile(`^\s*(?:import\s+([A-Za-z_][\w.]*)(?:\s*,|\s|$)|from\s+([A-Za-z_][\w.]*)\s+import\b)`)

// Source import evidence requires syntax, not a matching comment or literal.
func sourceImportsCompatibleClient(base string, data []byte, signals []string) bool {
	matches := func(module string) bool {
		module = strings.ToLower(strings.TrimSpace(module))
		for _, signal := range signals {
			signal = strings.ToLower(signal)
			if module == signal || strings.HasPrefix(module, signal+".") || strings.HasPrefix(module, signal+"/") || strings.HasSuffix(module, "/"+signal) || strings.Contains(module, "/"+signal+"/") {
				return true
			}
		}
		return false
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go":
		file, err := parser.ParseFile(token.NewFileSet(), base, data, parser.ImportsOnly)
		if err != nil {
			return false
		}
		for _, imp := range file.Imports {
			module, err := strconv.Unquote(imp.Path.Value)
			if err == nil && matches(module) {
				return true
			}
		}
	case ".py":
		var quote byte
		triple := false
		var line strings.Builder
		flush := func() bool {
			m := pythonImportLine.FindStringSubmatch(line.String())
			line.Reset()
			return m != nil && (matches(m[1]) || matches(m[2]))
		}
		for i := 0; i < len(data); i++ {
			c := data[i]
			if c == '\n' {
				if flush() {
					return true
				}
				continue
			}
			if quote != 0 {
				if c == '\\' && !triple && i+1 < len(data) {
					i++
					continue
				}
				if c == quote {
					if triple {
						if i+2 < len(data) && data[i+1] == quote && data[i+2] == quote {
							quote = 0
							triple = false
							i += 2
						}
					} else {
						quote = 0
					}
				}
				line.WriteByte(' ')
				continue
			}
			if c == '#' {
				for i+1 < len(data) && data[i+1] != '\n' {
					i++
				}
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
				triple = i+2 < len(data) && data[i+1] == c && data[i+2] == c
				if triple {
					i += 2
				}
				line.WriteByte(' ')
				continue
			}
			line.WriteByte(c)
		}
		return flush()
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		tokens := jsModuleTokens(data)
		importing := false
		for i, tok := range tokens {
			if tok == ";" || tok == "\n" {
				importing = false
				continue
			}
			if tok == "import" || tok == "export" {
				importing = true
			}
			if tok == "require" && i+3 < len(tokens) && tokens[i+1] == "(" && strings.HasPrefix(tokens[i+2], "STRING:") && tokens[i+3] == ")" && matches(strings.TrimPrefix(tokens[i+2], "STRING:")) {
				return true
			}
			if tok == "import" && i+3 < len(tokens) && tokens[i+1] == "(" && strings.HasPrefix(tokens[i+2], "STRING:") && tokens[i+3] == ")" && matches(strings.TrimPrefix(tokens[i+2], "STRING:")) {
				return true
			}
			if importing && (tok == "import" || tok == "from") && i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "STRING:") && matches(strings.TrimPrefix(tokens[i+1], "STRING:")) {
				return true
			}
		}
	}
	return false
}

func jsModuleTokens(data []byte) []string {
	var tokens []string
	for i := 0; i < len(data); {
		c := data[i]
		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			if i+1 < len(data) {
				i += 2
			}
			continue
		}
		if c == '/' && (len(tokens) == 0 || tokens[len(tokens)-1] == "=" || tokens[len(tokens)-1] == "(" || tokens[len(tokens)-1] == ":" || tokens[len(tokens)-1] == ",") {
			i++
			inClass := false
			for i < len(data) && data[i] != '\n' {
				if data[i] == '\\' && i+1 < len(data) {
					i += 2
					continue
				}
				if data[i] == '[' {
					inClass = true
				}
				if data[i] == ']' {
					inClass = false
				}
				if data[i] == '/' && !inClass {
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '\'' || c == '"' || c == 96 {
			start, delimiter := i+1, c
			i++
			for i < len(data) {
				if data[i] == '\\' && i+1 < len(data) {
					i += 2
					continue
				}
				if data[i] == delimiter {
					break
				}
				i++
			}
			if delimiter != 96 && i < len(data) {
				tokens = append(tokens, "STRING:"+string(data[start:i]))
			} else {
				tokens = append(tokens, "LITERAL")
			}
			if i < len(data) {
				i++
			}
			continue
		}
		if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			start := i
			for i < len(data) {
				x := data[i]
				if !(x == '_' || x == '$' || x >= 'a' && x <= 'z' || x >= 'A' && x <= 'Z' || x >= '0' && x <= '9') {
					break
				}
				i++
			}
			tokens = append(tokens, string(data[start:i]))
			continue
		}
		if c == ';' || c == '(' || c == ')' || c == '=' || c == ':' || c == ',' || c == '\n' {
			tokens = append(tokens, string(c))
		}
		i++
	}
	return tokens
}
