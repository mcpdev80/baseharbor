package repositoryinspect

import (
	"strings"
	"testing"
)

func TestInspectWorkloadSourcesComposePositive(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml": []byte("services:\n  api:\n    image: ghcr.io/acme/api:1\n    ports: [\"8080:8080\"]\n    healthcheck:\n      test: [\"CMD\", \"true\"]\n  db:\n    image: postgres:17\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != WorkloadSourceCompose {
		t.Fatalf("candidates = %#v", candidates)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 2 {
		t.Fatalf("components = %#v", evidence.Components)
	}
	if evidence.Fingerprint == "" {
		t.Fatal("missing fingerprint")
	}
}

func TestComposeNormalizedEvidenceIncludesBuildConfigDependenciesAndStorage(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml": []byte(`services:
  api:
    build:
      context: .
    image: ghcr.io/acme/api:1
    env_file:
      - .env.example
    configs:
      - app-config
    depends_on:
      db:
        condition: service_healthy
    volumes:
      - app-data:/var/lib/app
    ports:
      - "8080:8080"
`),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 1 {
		t.Fatalf("components = %#v", evidence.Components)
	}
	got := evidence.Components[0]
	if got.Image != "ghcr.io/acme/api:1" || got.Build != "." {
		t.Fatalf("image/build evidence = %#v", got)
	}
	if len(got.EnvironmentRefs) != 1 || got.EnvironmentRefs[0] != ".env.example" {
		t.Fatalf("environment refs = %#v", got.EnvironmentRefs)
	}
	if len(got.ConfigRefs) != 1 || got.ConfigRefs[0] != "app-config" {
		t.Fatalf("config refs = %#v", got.ConfigRefs)
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0] != "db" {
		t.Fatalf("dependencies = %#v", got.Dependencies)
	}
	if len(got.PersistentStorage) != 1 || got.PersistentStorage[0] != "app-data:/var/lib/app" {
		t.Fatalf("storage = %#v", got.PersistentStorage)
	}
}

func TestComposeClassifiesMySQLAndMariaDBAsSQLInfrastructure(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml": []byte("services:\n  mysql-db:\n    image: mysql:8\n  maria-db:\n    image: mariadb:11\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 2 {
		t.Fatalf("components = %#v", evidence.Components)
	}
	for _, component := range evidence.Components {
		if component.InfrastructureClass != "database.sql" {
			t.Fatalf("component %s classification = %q", component.ID, component.InfrastructureClass)
		}
	}
}

func TestInspectWorkloadSourcesQuadletPositive(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/quadlet/api.container": []byte("[Container]\nImage=ghcr.io/acme/api:1\nPublishPort=8080:8080\nHealthCmd=true\n"),
		"deploy/quadlet/db.container":  []byte("[Container]\nImage=postgres:17\nVolume=db:/var/lib/postgresql/data\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != WorkloadSourceQuadlet {
		t.Fatalf("candidates = %#v", candidates)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 2 {
		t.Fatalf("components = %#v", evidence.Components)
	}
	if evidence.Components[1].InfrastructureClass != "database.sql" && evidence.Components[0].InfrastructureClass != "database.sql" {
		t.Fatalf("postgres classification missing: %#v", evidence.Components)
	}
}

func TestQuadletKubeEmbedsKubernetesSourcePositive(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/app.kube": []byte("[Kube]\nYaml=app.yaml\n"),
		"deploy/app.yaml": []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: example/api:1
          ports:
            - containerPort: 8080
`),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != WorkloadSourceQuadlet {
		t.Fatalf("embedded Kubernetes YAML must not become a second source candidate: %#v", candidates)
	}
	if len(candidates[0].Evidence) != 2 {
		t.Fatalf("quadlet candidate should retain unit and embedded manifest evidence: %#v", candidates[0].Evidence)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 1 || evidence.Components[0].ID != "api" {
		t.Fatalf("components = %#v", evidence.Components)
	}
	if evidence.Components[0].Image != "example/api:1" {
		t.Fatalf("embedded Kubernetes image was not normalized: %#v", evidence.Components[0])
	}
	if evidence.Components[0].Image == "app.yaml" {
		t.Fatal("Quadlet Kube.Yaml path leaked into image evidence")
	}
}

func TestQuadletKubeMissingManifestDoesNotInventKubernetesCandidateNegative(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/app.kube": []byte("[Kube]\nYaml=missing.yaml\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != WorkloadSourceQuadlet {
		t.Fatalf("candidates = %#v", candidates)
	}
	if len(candidates[0].Unresolved) != 1 || candidates[0].Unresolved[0] != "deploy/missing.yaml" {
		t.Fatalf("unresolved = %#v", candidates[0].Unresolved)
	}
	if _, err := NormalizeWorkloadSource(snapshot, candidates[0]); err == nil || !strings.Contains(err.Error(), "unresolved inputs") {
		t.Fatalf("missing embedded YAML must fail closed, err=%v", err)
	}
}

func TestInspectWorkloadSourcesKubernetesPositive(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/k8s/app.yaml": []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/acme/api:1
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: api
spec:
  ports:
    - port: 80
`),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != WorkloadSourceKubernetes {
		t.Fatalf("candidates = %#v", candidates)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 1 || evidence.Components[0].ID != "api" || !evidence.Components[0].Health {
		t.Fatalf("components = %#v", evidence.Components)
	}
	if len(evidence.Opaque) != 1 || evidence.Opaque[0].Resource != "Service/api" {
		t.Fatalf("opaque/supporting = %#v", evidence.Opaque)
	}
}

func TestKubernetesNormalizedEvidenceIncludesReferencesAndExposure(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/k8s/app.yaml": []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: example/api:1
          env:
            - name: DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: api-secret
                  key: password
      volumes:
        - name: cfg
          configMap:
            name: api-config
---
apiVersion: v1
kind: Service
metadata:
  name: api-http
spec:
  selector:
    app: api
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: public
spec:
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: api
                port:
                  number: 80
`),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Components) != 1 {
		t.Fatalf("components = %#v", evidence.Components)
	}
	got := evidence.Components[0]
	if len(got.EnvironmentRefs) != 1 || got.EnvironmentRefs[0] != "DATABASE_PASSWORD" {
		t.Fatalf("environment refs = %#v", got.EnvironmentRefs)
	}
	if len(got.ConfigRefs) != 2 {
		t.Fatalf("config refs = %#v", got.ConfigRefs)
	}
	if len(got.Exposure) != 2 {
		t.Fatalf("exposure = %#v", got.Exposure)
	}
}

func TestInspectWorkloadSourcesRejectsFalsePositiveYaml(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		".github/workflows/ci.yml": []byte("name: ci\non:\n  push:\n"),
		"config/app.yaml":          []byte("server:\n  port: 8080\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("false positives: %#v", candidates)
	}
}

func TestInspectWorkloadSourcesAmbiguousFamiliesNegative(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml":        []byte("services:\n  api:\n    image: ghcr.io/acme/api:1\n"),
		"deploy/k8s/app.yaml": []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: ghcr.io/acme/api:1\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v", candidates)
	}
	if selected := selectWorkloadSource(candidates); selected != nil {
		t.Fatalf("different workload source families require explicit selection, selected=%#v", selected)
	}
}

func TestWorkloadSourceSelectionPrefersProductionComposeOverTests(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "docker/docker-compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "e2e/docker-compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "packages/db/test/docker-compose.yml"},
	}
	selected := selectWorkloadSource(candidates)
	if selected == nil || selected.Path != "docker/docker-compose.yml" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestWorkloadSourceSelectionKeepsSameFamilyAmbiguousWithoutStrongSignal(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "lifecycle/container/compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "packages/client-go/compose.yml"},
		{Kind: WorkloadSourceCompose, Path: "scripts/compose.yml"},
	}
	if selected := selectWorkloadSource(candidates); selected != nil {
		t.Fatalf("weak candidates must remain ambiguous: %#v", selected)
	}
}

func TestWorkloadSourceSelectionIgnoresCrossFamilyTestNoise(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "docker/docker-compose.yml"},
		{Kind: WorkloadSourceKubernetes, Path: "tests/kubernetes"},
		{Kind: WorkloadSourceQuadlet, Path: "examples/quadlet"},
	}
	selected := selectWorkloadSource(candidates)
	if selected == nil || selected.Kind != WorkloadSourceCompose || selected.Path != "docker/docker-compose.yml" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestWorkloadSourceSelectionDoesNotChooseBetweenProductionFamilies(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "compose.yaml"},
		{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"},
	}
	if selected := selectWorkloadSource(candidates); selected != nil {
		t.Fatalf("production source families must remain explicit: %#v", selected)
	}
}

func TestWorkloadSourceFingerprintDeterministic(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"deploy/quadlet/api.container": []byte("[Container]\nImage=example/api:1\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("fingerprint not deterministic: %q != %q", a.Fingerprint, b.Fingerprint)
	}
	snapshot.Files["deploy/quadlet/api.container"] = []byte("[Container]\nImage=example/api:2\n")
	c, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if c.Fingerprint == a.Fingerprint {
		t.Fatalf("fingerprint did not change: %q", c.Fingerprint)
	}
}

func TestComposeFingerprintIncludesMaterialBuildInputs(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml": []byte("services:\n  api:\n    build: .\n"),
		"Dockerfile":   []byte("FROM scratch\nCOPY main.go /app\n"),
		"main.go":      []byte("package main\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files["main.go"] = []byte("package main\n// changed\n")
	after, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == after.Fingerprint {
		t.Fatal("material build input change did not change fingerprint")
	}
}

func TestComposeFingerprintIgnoresUnrelatedSourceForImageOnlyService(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yaml": []byte("services:\n  api:\n    image: example/api:1\n"),
		"main.go":      []byte("package main\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files["main.go"] = []byte("package main\n// unrelated\n")
	after, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint != after.Fingerprint {
		t.Fatalf("image-only fingerprint changed for unrelated source: %q -> %q", before.Fingerprint, after.Fingerprint)
	}
}

func TestKubernetesSecretValuesAreNotNormalized(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"k8s/secret.yaml": []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: credentials\nstringData:\n  password: super-secret\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range evidence.Opaque {
		if ref.Resource == "super-secret" {
			t.Fatal("secret value leaked into evidence")
		}
	}
}
