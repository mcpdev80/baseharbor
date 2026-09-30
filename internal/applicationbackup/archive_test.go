package applicationbackup

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildOpenRoundTrip(t *testing.T) {
	password := []byte("correct horse battery staple")
	created := time.Date(2026, 9, 9, 0, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	entries := []PayloadEntry{
		{Name: "metadata/application.json", Data: []byte(`{"name":"mailflow"}`)},
		{Name: "postgres/main.dump", Data: []byte("postgres-dump")},
		{Name: "secrets/openbao.json", Data: []byte(`{"dyn-abc":"provider-key"}`)},
	}

	archive, err := Build("mailflow", "dev", created, entries, password)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if strings.Contains(string(archive), "provider-key") || strings.Contains(string(archive), "postgres-dump") {
		t.Fatal("archive contains plaintext payload")
	}

	payload, err := Open(archive, password)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if payload.Manifest.Application != "mailflow" || payload.Manifest.Environment != "dev" {
		t.Fatalf("unexpected identity: %#v", payload.Manifest)
	}
	if !payload.Manifest.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt = %v, want %v", payload.Manifest.CreatedAt, created)
	}
	if got := string(payload.Entries[2].Data); got != `{"dyn-abc":"provider-key"}` {
		t.Fatalf("secret payload = %q", got)
	}
}

func TestOpenRejectsWrongPassword(t *testing.T) {
	archive, err := Build("mailflow", "dev", time.Now(), []PayloadEntry{{Name: "metadata/application.json", Data: []byte("x")}}, []byte("right"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(archive, []byte("wrong")); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Open() error = %v, want authentication failure", err)
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	archive, err := Build("mailflow", "dev", time.Now(), []PayloadEntry{{Name: "metadata/application.json", Data: []byte("x")}}, []byte("password"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(archive, &envelope); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
	archive, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(archive, []byte("password")); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Open() error = %v, want authentication failure", err)
	}
}

func TestBuildRejectsTraversalName(t *testing.T) {
	badNames := []string{
		"../secrets/openbao.json",
		"postgres/../../etc/passwd",
		"/postgres/main.dump",
		"postgres\\main.dump",
		"postgres//main.dump",
	}
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			_, err := Build("mailflow", "dev", time.Now(), []PayloadEntry{{Name: name, Data: []byte("x")}}, []byte("password"))
			if err == nil || !strings.Contains(err.Error(), "invalid backup entry name") {
				t.Fatalf("Build() error = %v", err)
			}
		})
	}
}

func TestOpenRejectsUnsafeKDFBeforeDerivation(t *testing.T) {
	envelope := Envelope{
		Magic:   FormatMagic,
		Version: SchemaVersion,
		KDF: KDFParams{
			Algorithm: "argon2id",
			Time:      1,
			MemoryKiB: 1 << 30,
			Threads:   1,
			KeyLength: argonKeyLength,
			Salt:      base64.RawStdEncoding.EncodeToString(make([]byte, 16)),
		},
		Nonce:      "AA",
		Ciphertext: "AA",
	}
	archive, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(archive, []byte("password")); err == nil || !strings.Contains(err.Error(), "memory cost outside allowed range") {
		t.Fatalf("Open() error = %v, want KDF resource-limit rejection", err)
	}
}

func TestPayloadValidateRejectsChecksumMismatch(t *testing.T) {
	archive, err := Build("mailflow", "dev", time.Now(), []PayloadEntry{{Name: "secrets/openbao.json", Data: []byte("secret")}}, []byte("password"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Open(archive, []byte("password"))
	if err != nil {
		t.Fatal(err)
	}
	payload.Entries[0].Data[0] ^= 0xff
	if err := payload.Validate(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Validate() error = %v, want checksum mismatch", err)
	}
}
