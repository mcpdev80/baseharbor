package repositoryinspect

import (
	"bytes"
	"sort"
	"strings"
)

// DeclaredIntentEvidenceProfile describes provider-neutral repository evidence
// that can satisfy an already explicit portable capability intent. The intent
// disambiguates service semantics; the evidence proves that the repository
// actually wires a compatible client/binding path.
type DeclaredIntentEvidenceProfile struct {
	Environment []string
	Tokens      []string
}

var declaredIntentEvidenceProfiles = map[string]DeclaredIntentEvidenceProfile{
	"database.key-value": {
		Environment: []string{"VALKEY_URL", "VALKEY_CA_FILE"},
		Tokens: []string{
			"github.com/redis/go-redis", "quarkus-redis-client", "import redis", "\"redis\"",
		},
	},
	"database.document": {
		Environment: []string{"MONGODB_URL", "MONGO_URL", "MONGODB_CA_FILE"},
		Tokens: []string{
			"go.mongodb.org/mongo-driver", "mongodb", "pymongo", "quarkus-mongodb-client",
		},
	},
	"messaging.queue": {
		Environment: []string{"AMQP_URL", "RABBITMQ_URL", "RABBITMQ_CA_FILE"},
		Tokens: []string{
			"amqp091-go", "amqplib", "pika", "quarkus-messaging-rabbitmq",
		},
	},
	"messaging.pubsub": {
		Environment: []string{"AMQP_URL", "RABBITMQ_URL", "RABBITMQ_CA_FILE"},
		Tokens: []string{
			"amqp091-go", "amqplib", "pika", "quarkus-messaging-rabbitmq",
		},
	},
	"messaging.stream": {
		Environment: []string{"AMQP_URL", "RABBITMQ_URL", "RABBITMQ_CA_FILE"},
		Tokens: []string{
			"amqp091-go", "amqplib", "pika", "quarkus-messaging-rabbitmq",
		},
	},
}

// DeclaredIntentEvidenceProfiles returns the capability IDs for which repository
// inspection can prove developer integration from standard bindings/dependencies.
// It is intentionally exported for development conformance tests.
func DeclaredIntentEvidenceProfiles() []string {
	result := make([]string, 0, len(declaredIntentEvidenceProfiles))
	for capability := range declaredIntentEvidenceProfiles {
		result = append(result, capability)
	}
	sort.Strings(result)
	return result
}

func enrichDeclaredIntentEvidence(snapshot Snapshot, declared []CapabilityIntent, findings []Finding) []Finding {
	for _, intent := range declared {
		profile, ok := declaredIntentEvidenceProfiles[intent.Capability]
		if !ok {
			continue
		}
		evidence := declaredIntentEvidence(snapshot, profile)
		if len(evidence) == 0 {
			continue
		}
		findings = mergeFindings(findings, []Finding{{
			Capability: intent.Capability,
			Name:       intent.Name,
			Direction:  intent.Direction,
			Confidence: ConfidenceDetected,
			Evidence:   evidence,
		}})
	}
	return findings
}

func declaredIntentEvidence(snapshot Snapshot, profile DeclaredIntentEvidenceProfile) []Evidence {
	var evidence []Evidence
	envSet := map[string]struct{}{}
	for _, name := range profile.Environment {
		envSet[strings.ToUpper(strings.TrimSpace(name))] = struct{}{}
	}
	tokens := make([]string, 0, len(profile.Tokens))
	for _, token := range profile.Tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	for path, data := range snapshot.Files {
		lower := bytes.ToLower(data)
		if strings.HasPrefix(filepathBaseLower(path), ".env") {
			for _, name := range readEnvNames(data) {
				if _, ok := envSet[strings.ToUpper(strings.TrimSpace(name))]; ok {
					evidence = append(evidence, Evidence{Kind: EvidenceEnv, Path: path, Detail: "standard capability binding "+name})
				}
			}
		}
		for _, token := range tokens {
			if bytes.Contains(lower, []byte(token)) {
				evidence = append(evidence, Evidence{Kind: EvidenceDependency, Path: path, Detail: "compatible capability client/dependency "+token})
			}
		}
	}
	return uniqueEvidence(evidence)
}

func filepathBaseLower(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	return strings.ToLower(path)
}
