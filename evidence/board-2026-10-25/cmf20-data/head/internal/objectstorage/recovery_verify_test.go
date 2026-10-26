package objectstorage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRestoreVerificationWaitsForKeysAndObjectBytes(t *testing.T) {
	lists, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "" {
			lists++
			if lists == 1 {
				w.Write([]byte(`<ListBucketResult/>`))
				return
			}
			w.Write([]byte(`<ListBucketResult><Contents><Key>restored</Key></Contents></ListBucketResult>`))
			return
		}
		reads++
		if reads == 1 {
			w.Write([]byte("old"))
			return
		}
		w.Write([]byte("new"))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	backup := BucketBackup{LogicalBucket: "uploads", Objects: []BackupObject{{Key: "restored", Data: []byte("new")}}}
	if err := waitRestoredBucket(ctx, server.Client(), server.URL, "bucket", application.ObjectStorageCredentials{}, backup); err != nil {
		t.Fatal(err)
	}
	if lists != 3 || reads != 2 {
		t.Fatalf("lists=%d reads=%d", lists, reads)
	}
}

func TestRestoreVerificationFailsClosedOnPersistentObjectMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<ListBucketResult><Contents><Key>unexpected</Key></Contents></ListBucketResult>`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := waitRestoredBucket(ctx, server.Client(), server.URL, "bucket", application.ObjectStorageCredentials{}, BucketBackup{LogicalBucket: "uploads"})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "object set differs") {
		t.Fatalf("err=%v", err)
	}
}
