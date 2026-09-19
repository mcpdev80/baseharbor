package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type fakeRuntimeS3IAM struct {
	mu              sync.Mutex
	buckets         map[string]bool
	users           map[string]bool
	keys            map[string][]IAMAccessKey
	createKeyCalls  int
}

func newFakeRuntimeS3IAM() *fakeRuntimeS3IAM {
	return &fakeRuntimeS3IAM{
		buckets: map[string]bool{},
		users:   map[string]bool{},
		keys:    map[string][]IAMAccessKey{},
	}
}

func (f *fakeRuntimeS3IAM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodPost && r.URL.Path == "/" {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		action := values.Get("Action")
		user := values.Get("UserName")
		switch action {
		case "CreateUser":
			if f.users[user] {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `<ErrorResponse><Error><Code>EntityAlreadyExists</Code></Error></ErrorResponse>`)
				return
			}
			f.users[user] = true
			_, _ = io.WriteString(w, `<CreateUserResponse/>`)
		case "ListAccessKeys":
			var b strings.Builder
			b.WriteString("<ListAccessKeysResponse><ListAccessKeysResult><AccessKeyMetadata>")
			for _, key := range f.keys[user] {
				fmt.Fprintf(&b, "<member><AccessKeyId>%s</AccessKeyId></member>", key.AccessKeyID)
			}
			b.WriteString("</AccessKeyMetadata></ListAccessKeysResult></ListAccessKeysResponse>")
			_, _ = io.WriteString(w, b.String())
		case "DeleteAccessKey":
			target := values.Get("AccessKeyId")
			var kept []IAMAccessKey
			for _, key := range f.keys[user] {
				if key.AccessKeyID != target {
					kept = append(kept, key)
				}
			}
			f.keys[user] = kept
			_, _ = io.WriteString(w, `<DeleteAccessKeyResponse/>`)
		case "PutUserPolicy":
			if !f.users[user] {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<ErrorResponse><Error><Code>NoSuchEntity</Code></Error></ErrorResponse>`)
				return
			}
			_, _ = io.WriteString(w, `<PutUserPolicyResponse/>`)
		case "CreateAccessKey":
			f.createKeyCalls++
			key := IAMAccessKey{
				AccessKeyID:     fmt.Sprintf("AKIA%08d", f.createKeyCalls),
				SecretAccessKey: fmt.Sprintf("secret-%08d", f.createKeyCalls),
			}
			f.keys[user] = append(f.keys[user], key)
			fmt.Fprintf(w, "<CreateAccessKeyResponse><CreateAccessKeyResult><AccessKey><AccessKeyId>%s</AccessKeyId><SecretAccessKey>%s</SecretAccessKey></AccessKey></CreateAccessKeyResult></CreateAccessKeyResponse>", key.AccessKeyID, key.SecretAccessKey)
		case "DeleteUserPolicy":
			_, _ = io.WriteString(w, `<DeleteUserPolicyResponse/>`)
		case "DeleteUser":
			delete(f.users, user)
			delete(f.keys, user)
			_, _ = io.WriteString(w, `<DeleteUserResponse/>`)
		default:
			http.Error(w, "unsupported IAM action", http.StatusBadRequest)
		}
		return
	}

	bucket := strings.TrimPrefix(r.URL.Path, "/")
	switch r.Method {
	case http.MethodHead:
		if f.buckets[bucket] {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case http.MethodPut:
		f.buckets[bucket] = true
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if !f.buckets[bucket] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.buckets, bucket)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unsupported S3 request", http.StatusMethodNotAllowed)
	}
}

func TestRuntimeResourceManagerCreateGetDeleteIsIdempotent(t *testing.T) {
	fake := newFakeRuntimeS3IAM()
	server := httptest.NewServer(fake)
	defer server.Close()

	manager, err := NewRuntimeResourceManager(t.TempDir(), server.URL, server.Client(), AdminCredentials{
		AccessKeyID: "ADMINKEY", SecretAccessKey: "ADMINSECRET",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := manager.Create(context.Background(), "mailflow", "dev", "user-4711")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create(context.Background(), "mailflow", "dev", "user-4711")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("idempotent create changed binding: first=%#v second=%#v", first, second)
	}
	if fake.createKeyCalls != 1 {
		t.Fatalf("CreateAccessKey calls = %d, want 1", fake.createKeyCalls)
	}

	got, err := manager.Get(context.Background(), "mailflow", "dev", "user-4711")
	if err != nil {
		t.Fatal(err)
	}
	if got.ResourceID != first.ResourceID || got.AccessKeyID != first.AccessKeyID || got.SecretAccessKey != first.SecretAccessKey {
		t.Fatalf("get binding = %#v, want %#v", got, first)
	}

	if _, err := manager.Get(context.Background(), "other-app", "dev", "user-4711"); err == nil {
		t.Fatal("cross-application get unexpectedly succeeded")
	}

	if err := manager.Delete(context.Background(), "mailflow", "dev", "user-4711"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(context.Background(), "mailflow", "dev", "user-4711"); err == nil {
		t.Fatal("deleted runtime resource still resolves")
	}
	if err := manager.Delete(context.Background(), "mailflow", "dev", "user-4711"); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}
