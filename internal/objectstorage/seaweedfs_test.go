package objectstorage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestPhysicalBucketNameIsStableAndBounded(t *testing.T) {
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "very-long-application-name-with-many-segments",
		Environment: "production",
	}
	first := PhysicalBucketName(m, "large-object-storage-bucket-name")
	second := PhysicalBucketName(m, "large-object-storage-bucket-name")
	if first != second {
		t.Fatalf("bucket name is not deterministic: %q vs %q", first, second)
	}
	if len(first) > 63 {
		t.Fatalf("bucket name length = %d, want <= 63: %q", len(first), first)
	}
	if !strings.HasPrefix(first, "bh-") {
		t.Fatalf("bucket name = %q, want BaseHarbor prefix", first)
	}
}

func TestEnsureProviderFilesUsesIAMWithoutGlobalS3Credentials(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)

	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t))
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	envText := string(env)
	if !strings.Contains(envText, "BASEHARBOR_SEAWEEDFS_PORT=") {
		t.Fatalf("provider environment missing selected port: %s", envText)
	}
	for _, forbidden := range []string{"ACCESS_KEY", "SECRET_KEY", "AWS_"} {
		if strings.Contains(envText, forbidden) {
			t.Fatalf("provider environment contains global S3 credential material %q: %s", forbidden, envText)
		}
	}
	info, err := os.Stat(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("provider environment mode = %o, want 600", info.Mode().Perm())
	}

	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	if !strings.Contains(composeText, "server -s3 -iam=true") {
		t.Fatalf("SeaweedFS IAM is not explicitly enabled:\n%s", composeText)
	}
	if strings.Contains(composeText, "AWS_ACCESS_KEY_ID") || strings.Contains(composeText, "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("provider compose contains global S3 credentials:\n%s", composeText)
	}
}

func TestSignedS3RequestUsesSigV4AndPathStyle(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotHash, gotDate, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		gotHash = r.Header.Get("X-Amz-Content-Sha256")
		gotDate = r.Header.Get("X-Amz-Date")
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	credentials := application.ObjectStorageCredentials{
		AccessKeyID:     "BASEHARBORTESTKEY",
		SecretAccessKey: "baseharbor-test-secret",
	}
	status, _, err := signedS3Request(
		context.Background(),
		server.Client(),
		server.URL,
		http.MethodPut,
		"assets",
		"folder/probe.txt",
		credentials,
		[]byte("probe"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if gotMethod != http.MethodPut || gotPath != "/assets/folder/probe.txt" || gotBody != "probe" {
		t.Fatalf("request = method %s path %s body %q", gotMethod, gotPath, gotBody)
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 Credential=BASEHARBORTESTKEY/") {
		t.Fatalf("authorization header = %q", gotAuth)
	}
	if gotHash == "" || gotDate == "" {
		t.Fatalf("SigV4 headers missing: hash=%q date=%q", gotHash, gotDate)
	}
}

func TestSeaweedFSProviderRunsUnprivileged(t *testing.T) {
	text := providerComposeYAML()
	for _, want := range []string{
		"user: \"seaweed\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"no-new-privileges:true",
		"/tmp:rw,noexec,nosuid,nodev",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("SeaweedFS provider missing %q:\n%s", want, text)
		}
	}
}

func TestSeaweedFSProviderUsesQualifiedImageReference(t *testing.T) {
	text := providerComposeYAML()
	if !strings.Contains(text, "image: "+ProviderImage) {
		t.Fatalf("SeaweedFS provider compose does not use qualified ProviderImage %q:\n%s", ProviderImage, text)
	}
	if strings.Contains(text, "image: chrislusf/seaweedfs:") {
		t.Fatalf("SeaweedFS provider compose contains unqualified image reference:\n%s", text)
	}
}
