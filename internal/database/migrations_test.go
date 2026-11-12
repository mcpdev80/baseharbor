package database

import (
	"reflect"
	"testing"
)

func TestExtractDroppedTablesRequiresDropTableClause(t *testing.T) {
	got := extractDroppedTables(`
ALTER TABLE workstation_enrollment_grants DROP CONSTRAINT grant_ttl_check;
DROP TABLE IF EXISTS obsolete_grants;
DROP TABLE old_grants;
`)
	want := []string{"obsolete_grants", "old_grants"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractDroppedTables() = %#v, want %#v", got, want)
	}
}
