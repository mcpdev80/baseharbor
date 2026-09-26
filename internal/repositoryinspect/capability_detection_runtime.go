package repositoryinspect

import (
	"bufio"
	"context"
	"path/filepath"
	"strings"
	"unicode"
)

type sqlDetector struct{}

func (sqlDetector) Name() string { return "database.sql" }

func (sqlDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "database.sql", sqlSignals()), nil
}

type keyValueDetector struct{}

func (keyValueDetector) Name() string { return "cache.key-value" }

func (keyValueDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "cache.key-value", keyValueSignals()), nil
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

func detectCapability(ctx context.Context, snapshot Snapshot, capability string, signals signalSet) []Finding {
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
		if isSourceFile(base) && containsAny(lower, signals.imports) {
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
		return findings
	}
	if len(suggested) > 0 {
		return []Finding{{Capability: capability, Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(suggested)}}
	}
	if len(possible) > 0 {
		return []Finding{{Capability: capability, Confidence: ConfidencePossible, Evidence: uniqueEvidence(possible)}}
	}
	return nil
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
