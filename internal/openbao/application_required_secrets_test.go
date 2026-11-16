package openbao

import "testing"

func TestRequireApplicationSecrets(t *testing.T) {
	if err := RequireApplicationSecrets([]RequiredSecretStatus{{Name: "A", Present: true, Usable: true}}); err != nil {
		t.Fatalf("ready secret rejected: %v", err)
	}
	for _, statuses := range [][]RequiredSecretStatus{
		{{Name: "A", Present: false, Usable: false}},
		{{Name: "A", Present: true, Usable: false}},
		{{Name: "A", Present: true, Usable: true}, {Name: "B", Present: false, Usable: false}},
	} {
		if err := RequireApplicationSecrets(statuses); err == nil {
			t.Fatalf("expected required secret failure for %#v", statuses)
		}
	}
}
