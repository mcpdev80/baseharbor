package machinehttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/sourcetransfer"
)

func TestSourceTransferAuthenticatedHTTPSDeltaAndHashGate(t *testing.T) {
	h, err := New(executorFunc(nil), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, url string, data []byte, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, url, bytes.NewReader(data))
		if authenticated {
			req = withTestPrincipal(req)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	raw := []byte("name: hello\n")
	sum := sha256.Sum256(raw)
	m, err := sourcetransfer.CreateManifest("remote-docker", "hello", "dev", "", []sourcetransfer.Entry{{Path: "baseharbor.yaml", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw)), Mode: 0644}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(m)
	denied := send(http.MethodPost, "/api/v1/machine/source-transfers", manifest, false)
	if denied.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext upload %d", denied.Code)
	}
	begin := send(http.MethodPost, "/api/v1/machine/source-transfers", manifest, true)
	if begin.Code != http.StatusCreated {
		t.Fatalf("authenticated begin %d: %s", begin.Code, begin.Body.String())
	}
	var admitted struct {
		ID      string   `json:"id"`
		Missing []string `json:"missing"`
	}
	if err := json.Unmarshal(begin.Body.Bytes(), &admitted); err != nil || len(admitted.Missing) != 1 {
		t.Fatalf("begin receipt %+v / %v", admitted, err)
	}
	payload, _ := json.Marshal(struct {
		Path   string `json:"path"`
		Offset int64  `json:"offset"`
		Data   []byte `json:"data"`
	}{"baseharbor.yaml", 0, raw})
	put := send(http.MethodPut, "/api/v1/machine/source-transfers/"+admitted.ID, payload, true)
	if put.Code != http.StatusOK {
		t.Fatalf("upload %d: %s", put.Code, put.Body.String())
	}
	done := send(http.MethodPost, "/api/v1/machine/source-transfers/"+admitted.ID+"/commit", nil, true)
	if done.Code != http.StatusOK {
		t.Fatalf("commit %d: %s", done.Code, done.Body.String())
	}
	current, root, err := h.sourceTransfers.Current(m.Target, m.Application, m.Environment)
	if err != nil || current.Digest != m.Digest {
		t.Fatalf("committed source not retained: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "baseharbor.yaml"))
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("committed bytes differ")
	}
}
