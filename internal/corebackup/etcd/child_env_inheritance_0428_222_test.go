package etcd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEtcdEnvInheritsParentEnv proves that a child of the baha CLI inherits the
// parent environment at every etcdctl/etcdutl call site. The stub binary writes
// the env it receives to a file; the test sets a card-scoped sentinel before the
// call and asserts the child side sees it. This is criterion 3 of card 0428-222.
func TestEtcdEnvInheritsParentEnv(t *testing.T) {
	const sentinelKey = "BASEHARBOR_0428_222_SENTINEL"
	const sentinelVal = "1"
	root := t.TempDir()
	out := filepath.Join(root, "child_env.txt")

	// env-capturing stub for both "etcdctl" and "etcdutl". It always writes the
	// inherited environment (minus PATH) to a file. For the "endpoint status"
	// action it also emits the JSON the attestation parser requires; for
	// "snapshot save" it creates a non-empty archive; other actions no-op.
	// argv layout: $0=binary, $1=subcommand, $2=action, $3=archive path (save).
	// The JSON is assembled from single-quoted shell pieces to avoid nested-quote
	// escaping in the Go source.
	statusLit := `{"revision":42,"totalKey":10,"totalSize":4096,"hash":1}`
	attestLit := `[{"Endpoint":"https://etcd-1.internal:2379","Status":{"header":{"cluster_id":12345,"revision":42},"version":"3.6.0"}}]`
	writeEnv := "#!/bin/sh\n" +
		"act=\"$2\"\n" + // action: save / status / restore
		"archive=\"$3\"\n" + // archive path (save)
		"shift\n" + // drop binary $0
		"shift\n" + // drop subcommand $1
		"env | grep -v '^PATH=' > " + quoteShell(out) + "\n" +
		"if [ \"$act\" = \"save\" ]; then printf 'archived' > \"$archive\"; fi\n" +
		"if [ \"$act\" = \"status\" ]; then printf '%s' '" + statusLit + "'; fi\n" +
		"if [ \"$act\" = \"endpoint\" ]; then printf '%s' '" + attestLit + "'; fi\n" +
		"exit 0\n"
	ctl := filepath.Join(root, "etcdctl")
	utl := filepath.Join(root, "etcdutl")
	for _, p := range []string{ctl, utl} {
		if err := os.WriteFile(p, []byte(writeEnv), 0700); err != nil {
			t.Fatal(err)
		}
	}

	prev, had := os.LookupEnv(sentinelKey)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(sentinelKey, prev)
		} else {
			_ = os.Unsetenv(sentinelKey)
		}
	})
	if err := os.Setenv(sentinelKey, sentinelVal); err != nil {
		t.Fatal(err)
	}

	ca, cert, key := tlsForTest(t)

	// --- site 1 & 2: Snapshot -> etcdctl snapshot save, etcdutl snapshot status ---
	t.Run("snapshot-save", func(t *testing.T) {
		if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		// Snapshot needs an owner-only (0700) scratch dir. t.TempDir() is 0755,
		// which fails the owner-only check, so create it explicitly.
		scratch := filepath.Join(t.TempDir(), "scratch")
		if err := os.Mkdir(scratch, 0700); err != nil {
			t.Fatal(err)
		}
		source := EtcdctlSource{
			Etcdctl: ctl, Etcdutl: utl,
			Endpoints:  []string{"https://etcd-1.internal:2379"},
			Identity:   Identity{Core: "core-1", Target: "target-1", Cluster: "cluster-1"},
			TLS:        TLSFiles{CA: ca, Cert: cert, Key: key},
			ScratchDir: scratch,
			Attest: func(context.Context) (SnapshotInfo, error) {
				return SnapshotInfo{ClusterID: "cluster-1", Version: "3.6.0", Revision: 1}, nil
			},
		}
		dest := filepath.Join(scratch, "snap.db")
		f, err := os.Create(dest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.Snapshot(context.Background(), f); err != nil {
			t.Fatalf("snapshot failed: %v", err)
		}
		assertSentinelInEnvFile(t, out, sentinelKey, sentinelVal)
	})

	// --- site 3: attestEndpointStatus -> etcdctl endpoint status ---
	t.Run("endpoint-status", func(t *testing.T) {
		if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		source := EtcdctlSource{
			Etcdctl:   ctl,
			Endpoints: []string{"https://etcd-1.internal:2379"},
			Identity:  Identity{Core: "core-1", Target: "target-1", Cluster: "12345"},
			TLS:       TLSFiles{CA: ca, Cert: cert, Key: key},
		}
		if _, err := source.attestEndpointStatus(context.Background()); err != nil {
			t.Fatalf("attestation failed: %v", err)
		}
		assertSentinelInEnvFile(t, out, sentinelKey, sentinelVal)
	})

	// --- site 4: IsolatedEtcdutl.RestoreIsolated -> etcdutl snapshot restore ---
	t.Run("isolated-restore", func(t *testing.T) {
		if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		// RestoreIsolated makes the parent of the destination owner-only, so put
		// the destination under a freshly created 0700 dir.
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(root, "snapshot")
		if err := os.WriteFile(archive, []byte("archive"), 0600); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, "restored")
		e := IsolatedEtcdutl{
			Binary:                   utl,
			MemberName:               "node1",
			ExpectedMemberNames:      []string{"node1"},
			InitialCluster:           "node1=https://n1:2380",
			InitialAdvertisePeerURLs: "https://n1:2380",
		}
		id := Identity{Core: "core1", Target: "target1", Cluster: "cluster-1"}
		info := SnapshotInfo{ClusterID: "cluster-1", Version: "3.6.0", Revision: 1}
		if err := e.RestoreIsolated(context.Background(), archive, dest, id, info); err != nil {
			t.Fatalf("isolated restore failed: %v", err)
		}
		assertSentinelInEnvFile(t, out, sentinelKey, sentinelVal)
	})
}

func assertSentinelInEnvFile(t *testing.T, path, key, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("did not capture child env: %v", err)
	}
	for _, line := range splitLines(string(data)) {
		if line == key+"="+want {
			return
		}
	}
	t.Fatalf("sentinel %s=%s not inherited by child; captured env:\n%s", key, want, string(data))
}

// pinWant is a single controlled key and the value the pin must carry.
type pinWant struct {
	key, pin string
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// quoteShell single-quotes a shell string safely.
func quoteShell(s string) string {
	return "'" + replaceSingleQuote(s) + "'"
}

func replaceSingleQuote(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b = append(b, '\'', '\\', '\'')
		} else {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// tlsForTest builds an in-memory CA and a client key pair so validate() and the
// mTLS Config() succeed without real PKI on disk.
func tlsForTest(t *testing.T) (caPEM, certPEM, keyPEM string) {
	t.Helper()
	dir := t.TempDir()

	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "baseharbor-test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Build the CA as its own parent using the parsed certificate, matching the
	// x509 contract that a self-signed certificate needs a parsed parent.
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "baseharbor-test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	caCert, err := x509.CreateCertificate(rand.Reader, clientTemplate, parent, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	caBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, caBytes, 0600); err != nil {
		t.Fatal(err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert})
	certPath := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(certPath, certBytes, 0600); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, keyBytes, 0600); err != nil {
		t.Fatal(err)
	}
	return caPath, certPath, keyPath
}

// TestPinnedEnvDropsInheritedKeys proves the security property of pinnedEnv:
// for every controlled key, an inherited value that conflicts with the pin is
// dropped so the pin wins. One case per key is required by card 0428-222 rc2.
func TestPinnedEnvDropsInheritedKeys(t *testing.T) {
	keys := []string{"PATH", "ETCDCTL_API", "ETCDCTL_ENDPOINTS", "ETCDCTL_CACERT", "ETCDCTL_CERT", "ETCDCTL_KEY"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			// Opposite inherited value for this key.
			inheritVal := "inherited-" + key + "-opposite"
			pinVal := "PIN-" + key

			prev, had := os.LookupEnv(key)
			t.Cleanup(func() {
				if had {
					_ = os.Setenv(key, prev)
				} else {
					_ = os.Unsetenv(key)
				}
			})
			if err := os.Setenv(key, inheritVal); err != nil {
				t.Fatal(err)
			}

			got := pinnedEnv("PATH=/usr/bin:/bin", "ETCDCTL_API=3",
				"ETCDCTL_ENDPOINTS=https://etcd-1.internal:2379",
				"ETCDCTL_CACERT=/pins/ca.pem", "ETCDCTL_CERT=/pins/cert.pem",
				"ETCDCTL_KEY=/pins/key.pem")

			// The key must appear exactly once and its value must be the pin's.
			count := 0
			var finalVal string
			for _, kv := range got {
				if i := indexByte(kv, '='); i >= 0 && kv[:i] == key {
					count++
					finalVal = kv[i+1:]
				}
			}
			if count != 1 {
				t.Fatalf("key %s appears %d times in pinned env; want exactly 1", key, count)
			}
			want := pinVal
			if key == "PATH" {
				want = "/usr/bin:/bin"
			} else if key == "ETCDCTL_API" {
				want = "3"
			} else if key == "ETCDCTL_ENDPOINTS" {
				want = "https://etcd-1.internal:2379"
			} else if key == "ETCDCTL_CACERT" {
				want = "/pins/ca.pem"
			} else if key == "ETCDCTL_CERT" {
				want = "/pins/cert.pem"
			} else if key == "ETCDCTL_KEY" {
				want = "/pins/key.pem"
			}
			if finalVal != want {
				t.Fatalf("key %s final value = %q; want pin %q (inherited %q must be dropped)", key, finalVal, want, inheritVal)
			}
		})
	}
}

// TestChildProcessEnvShowsPinNotInherited proves the end-to-end effect: the child
// process receives the pin value for every controlled key, even when the parent
// process exports an opposite value. It runs each of the four call sites with a
// stub that writes the received env to a file and asserts the pin value wins.
func TestChildProcessEnvShowsPinNotInherited(t *testing.T) {
	// keys pinned by sites 1 and 3 (snapshot save / endpoint status).
	pinnedKeys := []pinWant{{"PATH", "/usr/bin:/bin"}, {"ETCDCTL_API", "3"},
		{"ETCDCTL_ENDPOINTS", "https://etcd-1.internal:2379"},
		{"ETCDCTL_CACERT", "/pins/ca.pem"}, {"ETCDCTL_CERT", "/pins/cert.pem"},
		{"ETCDCTL_KEY", "/pins/key.pem"}}

	root := t.TempDir()
	out := filepath.Join(root, "child_env.txt")

	statusLit := `{"revision":42,"totalKey":10,"totalSize":4096,"hash":1}`
	attestLit := `[{"Endpoint":"https://etcd-1.internal:2379","Status":{"header":{"cluster_id":12345,"revision":42},"version":"3.6.0"}}]`

	// writeEnv writes the received env for each action to a distinct file
	// (child_env_<action>.txt) so the test can assert the exact env every call
	// site saw, independent of the order Snapshot() runs them in.
	writeEnv := "#!/bin/sh\n" +
		"act=\"$2\"\n" +
		"archive=\"$3\"\n" +
		"shift\n" +
		"shift\n" +
		"env > " + quoteShell(out) + ".${act}\n" +
		"if [ \"$act\" = \"save\" ]; then printf 'archived' > \"$archive\"; fi\n" +
		"if [ \"$act\" = \"status\" ]; then printf '%s' '" + statusLit + "'; fi\n" +
		"if [ \"$act\" = \"endpoint\" ]; then printf '%s' '" + attestLit + "'; fi\n" +
		"exit 0\n"

	ctl := filepath.Join(root, "etcdctl")
	utl := filepath.Join(root, "etcdutl")
	for _, p := range []string{ctl, utl} {
		if err := os.WriteFile(p, []byte(writeEnv), 0700); err != nil {
			t.Fatal(err)
		}
	}

	ca, cert, key := tlsForTest(t)

	// Set an opposite inherited value for every controlled key.
	prev := map[string]string{}
	had := map[string]bool{}
	for _, w := range pinnedKeys {
		p, h := os.LookupEnv(w.key)
		prev[w.key], had[w.key] = p, h
		if err := os.Setenv(w.key, "INHERITED-"+w.key); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, w := range pinnedKeys {
			if had[w.key] {
				_ = os.Setenv(w.key, prev[w.key])
			} else {
				_ = os.Unsetenv(w.key)
			}
		}
	})

	// --- sites 1 & 2: Snapshot save (site 1, all pins) + etcdutl status (site 2, PATH only) ---
	t.Run("snapshot-save-pins", func(t *testing.T) {
		if err := os.Remove(out + ".save"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		scratch := filepath.Join(t.TempDir(), "scratch")
		if err := os.Mkdir(scratch, 0700); err != nil {
			t.Fatal(err)
		}
		source := EtcdctlSource{
			Etcdctl: ctl, Etcdutl: utl,
			Endpoints:  []string{"https://etcd-1.internal:2379"},
			Identity:   Identity{Core: "core-1", Target: "target-1", Cluster: "cluster-1"},
			TLS:        TLSFiles{CA: ca, Cert: cert, Key: key},
			ScratchDir: scratch,
			Attest: func(context.Context) (SnapshotInfo, error) {
				return SnapshotInfo{ClusterID: "cluster-1", Version: "3.6.0", Revision: 1}, nil
			},
		}
		dest := filepath.Join(scratch, "snap.db")
		f, err := os.Create(dest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.Snapshot(context.Background(), f); err != nil {
			t.Fatalf("snapshot failed: %v", err)
		}
		// Site 1 (snapshot save) pins PATH, ETCDCTL_API, ETCDCTL_ENDPOINTS,
		// ETCDCTL_CACERT, ETCDCTL_CERT, ETCDCTL_KEY; the pin values come from the
		// source's own TLS/endpoints fields, so assert those exact values win
		// over the inherited opposite values.
		assertPinsInChildEnv(t, out+".save", []pinWant{
			{"PATH", "/usr/bin:/bin"},
			{"ETCDCTL_API", "3"},
			{"ETCDCTL_ENDPOINTS", "https://etcd-1.internal:2379"},
			{"ETCDCTL_CACERT", ca},
			{"ETCDCTL_CERT", cert},
			{"ETCDCTL_KEY", key},
		})
	})

	// --- site 3: attestEndpointStatus (all pins) ---
	t.Run("endpoint-status-pins", func(t *testing.T) {
		if err := os.Remove(out + ".endpoint"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		source := EtcdctlSource{
			Etcdctl:   ctl,
			Endpoints: []string{"https://etcd-1.internal:2379"},
			Identity:  Identity{Core: "core-1", Target: "target-1", Cluster: "12345"},
			TLS:       TLSFiles{CA: ca, Cert: cert, Key: key},
		}
		if _, err := source.attestEndpointStatus(context.Background()); err != nil {
			t.Fatalf("attestation failed: %v", err)
		}
		assertPinsInChildEnv(t, out+".endpoint", []pinWant{
			{"PATH", "/usr/bin:/bin"},
			{"ETCDCTL_API", "3"},
			{"ETCDCTL_CACERT", ca},
			{"ETCDCTL_CERT", cert},
			{"ETCDCTL_KEY", key},
		})
	})

	// --- site 4: IsolatedEtcdutl.RestoreIsolated (PATH pin only) ---
	t.Run("isolated-restore-pins", func(t *testing.T) {
		if err := os.Remove(out + ".restore"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		// RestoreIsolated makes the parent of the destination owner-only, so put
		// the destination under a freshly created 0700 dir.
		base := t.TempDir()
		if err := os.Chmod(base, 0700); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(base, "snapshot")
		if err := os.WriteFile(archive, []byte("archive"), 0600); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(base, "restored")
		e := IsolatedEtcdutl{
			Binary:                   utl,
			MemberName:               "node1",
			ExpectedMemberNames:      []string{"node1"},
			InitialCluster:           "node1=https://n1:2380",
			InitialAdvertisePeerURLs: "https://n1:2380",
		}
		id := Identity{Core: "core1", Target: "target1", Cluster: "cluster-1"}
		info := SnapshotInfo{ClusterID: "cluster-1", Version: "3.6.0", Revision: 1}
		if err := e.RestoreIsolated(context.Background(), archive, dest, id, info); err != nil {
			t.Fatalf("isolated restore failed: %v", err)
		}
		assertPinsInChildEnv(t, out+".restore", []pinWant{{"PATH", "/usr/bin:/bin"}})
	})
}

// assertPinsInChildEnv reads the file the child wrote and asserts every pinned
// key appears exactly once with the pin value (not the inherited opposite).
func assertPinsInChildEnv(t *testing.T, path string, pins []pinWant) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("did not capture child env: %v", err)
	}
	captured := map[string]string{}
	counts := map[string]int{}
	for _, line := range splitLines(string(data)) {
		if i := indexByte(line, '='); i >= 0 {
			k, v := line[:i], line[i+1:]
			counts[k]++
			captured[k] = v
		}
	}
	for _, p := range pins {
		if counts[p.key] != 1 {
			t.Fatalf("child saw key %s %d times; want exactly 1 (captured:\n%s)", p.key, counts[p.key], string(data))
		}
		if captured[p.key] != p.pin {
			t.Fatalf("child saw key %s = %q; want pin %q (parent exported INHERITED-%s)", p.key, captured[p.key], p.pin, p.key)
		}
	}
}
