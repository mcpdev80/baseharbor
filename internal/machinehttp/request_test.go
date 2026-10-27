package machinehttp

import (
	"strings"
	"testing"
)

func TestRequestBoundsApplyToCompleteBodyAndNoCredentialsReachErrors(t *testing.T) {
	for _, body := range []string{
		`{"operation_id":"runtime.info"} {"operation_id":"runtime.destroy"}`,
		`{"credential-secret":"sensitive-value"}`,
		`{"operation_id":"status","operation_id":"destroy","context":{"environment":"dev"}}`,
		`{"operation_id":"status","context":{"environment":"dev","environment":"prod"}}`,
		`{"operation_id":"status","input":{"credential-secret":"sensitive-value","credential-secret":"other"}}`,
		`null`,
		`[]`,
		`{}` + strings.Repeat(" ", maxRequestBytes),
	} {
		var destination ExecuteRequest
		err := decodeRequest(strings.NewReader(body), &destination)
		if err == nil || strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "credential") {
			t.Fatal("ambiguous/oversized request accepted or parser leaked credential data")
		}
	}
	var request ExecuteRequest
	if err := decodeRequest(strings.NewReader(`{"operation_id":"runtime.info","context":{"environment":"dev"}}`), &request); err != nil {
		t.Fatal(err)
	}
}
