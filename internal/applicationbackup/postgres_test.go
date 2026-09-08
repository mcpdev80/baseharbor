package applicationbackup

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestPostgresPayloadEntriesRoundTrip(t *testing.T) {
	m := application.WithPostgresInstances(application.New("mailflow", "dev", true, false, false), "analytics", "primary")
	backups := []application.PostgresBackup{
		{Instance: "primary", SQL: []byte("CREATE TABLE primary_probe(id integer);\n")},
		{Instance: "analytics", SQL: []byte("CREATE TABLE analytics_probe(id integer);\n")},
	}
	entries, err := PostgresPayloadEntries(backups)
	if err != nil {
		t.Fatalf("PostgresPayloadEntries() error = %v", err)
	}
	if got, want := []string{entries[0].Name, entries[1].Name}, []string{"postgres/analytics.sql", "postgres/primary.sql"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry names = %v, want %v", got, want)
	}

	archive, err := Build("mailflow", "dev", testCreatedAt, entries, []byte("correct horse battery staple"))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	payload, err := Open(archive, []byte("correct horse battery staple"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	restored, err := PostgresBackupsFromPayload(m, payload)
	if err != nil {
		t.Fatalf("PostgresBackupsFromPayload() error = %v", err)
	}
	if got, want := []string{restored[0].Instance, restored[1].Instance}, []string{"analytics", "primary"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored instances = %v, want %v", got, want)
	}
	if string(restored[0].SQL) != "CREATE TABLE analytics_probe(id integer);\n" {
		t.Fatalf("analytics SQL = %q", restored[0].SQL)
	}
	if string(restored[1].SQL) != "CREATE TABLE primary_probe(id integer);\n" {
		t.Fatalf("primary SQL = %q", restored[1].SQL)
	}
}

func TestPostgresBackupsFromPayloadRejectsCrossInstanceSet(t *testing.T) {
	m := application.WithPostgresInstances(application.New("mailflow", "dev", true, false, false), "analytics", "primary")
	payload := Payload{Entries: []PayloadEntry{
		{Name: "postgres/primary.sql", Data: []byte("SELECT 1;\n")},
		{Name: "postgres/other.sql", Data: []byte("SELECT 1;\n")},
	}}
	_, err := PostgresBackupsFromPayload(m, payload)
	if err == nil || !strings.Contains(err.Error(), "unexpected instance") {
		t.Fatalf("PostgresBackupsFromPayload() error = %v", err)
	}
}

func TestPostgresPayloadEntriesRejectsUnsafeInstance(t *testing.T) {
	_, err := PostgresPayloadEntries([]application.PostgresBackup{{Instance: "../escape", SQL: []byte("SELECT 1;\n")}})
	if err == nil || !strings.Contains(err.Error(), "invalid postgres backup instance") {
		t.Fatalf("PostgresPayloadEntries() error = %v", err)
	}
}
