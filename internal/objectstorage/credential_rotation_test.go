package objectstorage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type rotationTestRealization struct {
	instance SeaweedFSInstance
	mu       sync.Mutex
	commands []string
	retired  bool
	oldUser  string
}

func (r *rotationTestRealization) Apply(context.Context) (SeaweedFSInstance, error) {
	return r.instance, nil
}
func (r *rotationTestRealization) Existing(context.Context) (SeaweedFSInstance, error) {
	return r.instance, nil
}
func (r *rotationTestRealization) Admin(_ context.Context, command string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
	if r.oldUser != "" && strings.Contains(command, "-user="+r.oldUser) && strings.Contains(command, "-delete") {
		r.retired = true
	}
	return "", nil
}
func (r *rotationTestRealization) Destroy(context.Context) error { return nil }

func TestRotateBucketCredentialsUsesOverlapVerifyThenRetire(t *testing.T) {
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "rotation-demo",
		Environment:   "dev",
		Services: application.Services{
			ObjectStorage: true,
			ObjectStorageBuckets: map[string]application.ServiceInstance{
				"default": {},
			},
		},
	}
	store := application.Store{Root: t.TempDir()}
	files, err := application.EnsureRuntime(context.Background(), nil, store, m)
	if err != nil {
		t.Fatal(err)
	}
	old, err := application.LoadObjectStorageCredentials(files, "default")
	if err != nil {
		t.Fatal(err)
	}
	physical := PhysicalBucketName(m, "default")

	var mu sync.Mutex
	objects := map[string][]byte{}
	var realization *rotationTestRealization
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		usesOld := strings.Contains(auth, "Credential="+old.AccessKeyID+"/")
		realization.mu.Lock()
		retired := realization.retired
		realization.mu.Unlock()
		if usesOld && retired {
			http.Error(w, "retired", http.StatusForbidden)
			return
		}
		if !usesOld && !strings.Contains(auth, "Credential=BH") {
			http.Error(w, "unknown credential", http.StatusForbidden)
			return
		}
		path := req.URL.EscapedPath()
		switch req.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			objects[path] = append([]byte(nil), body...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if req.URL.Path == "/"+physical || req.URL.Path == "/"+physical+"/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			mu.Lock()
			body, ok := objects[path]
			mu.Unlock()
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			mu.Lock()
			delete(objects, path)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	realization = &rotationTestRealization{
		instance: SeaweedFSInstance{
			Endpoint:         server.URL,
			WorkloadEndpoint: server.URL,
			TrustBundle:      []byte("test-ca"),
			HTTPClient:       server.Client(),
		},
		oldUser: physical,
	}
	driver := NewDriverWithRealization(realization, m, files)

	if err := driver.RotateBucketCredentials(context.Background(), "default"); err != nil {
		t.Fatal(err)
	}

	current, err := application.LoadObjectStorageCredentials(files, "default")
	if err != nil {
		t.Fatal(err)
	}
	if current == old {
		t.Fatal("credential rotation did not replace application credential pair")
	}
	if !strings.HasPrefix(current.AccessKeyID, "BH") {
		t.Fatalf("replacement access key %q does not use BaseHarbor prefix", current.AccessKeyID)
	}

	bindingSecret, err := os.ReadFile(filepath.Join(files.Bindings, "object-storage-s3", "secret_access_key"))
	if err != nil {
		// Multi-instance bindings place the logical instance below the capability.
		bindingSecret, err = os.ReadFile(filepath.Join(files.Bindings, "object-storage-s3", "default", "secret_access_key"))
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(bindingSecret)) != current.SecretAccessKey {
		t.Fatal("workload binding was not reconciled to replacement S3 credentials")
	}

	realization.mu.Lock()
	commands := append([]string(nil), realization.commands...)
	retired := realization.retired
	realization.mu.Unlock()
	if !retired {
		t.Fatal("old SeaweedFS IAM identity was not retired")
	}
	if len(commands) < 2 || !strings.Contains(commands[0], "-access_key=") || !strings.Contains(commands[len(commands)-1], "-delete") {
		t.Fatalf("unexpected rotation command sequence: %#v", commands)
	}
	if _, err := os.Stat(bucketCredentialRotationStatePath(files, "default")); !os.IsNotExist(err) {
		t.Fatalf("completed rotation state was not removed: %v", err)
	}
	user, err := currentBucketIAMUser(files, "default", physical)
	if err != nil {
		t.Fatal(err)
	}
	if user == physical {
		t.Fatal("active IAM identity still points to retired user")
	}
}
