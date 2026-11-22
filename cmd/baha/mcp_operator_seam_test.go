package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPOperatorSeamReachesGateBothDirections is the 0428-218 mutation proof
// for the MCP operator-auth seam (cmd/baha/mcp.go:authorizeMCPOperation).
//
// It drives one and the same managed-environment MCP mutation (target.create in
// a "prod" environment) through the same MCP transport twice, once with the
// in-test operator fixture stamped onto the context and once with the context
// explicitly marked anonymous via withoutOperatorAuthTestContext. The two calls
// must land on opposite sides of the gate:
//
//   - BEFORE marking (wrongly-green protocol): the stamped context carries a
//     verified operator principal, so the managed mutation succeeds. This is
//     exactly the wrongly-green trap the card targets: an MCP test that receives
//     its verified principal from the fixture can never reach the denial it
//     claims. A case that stays green here is not evidence the gate works.
//   - AFTER marking (red protocol): the identical call on an anonymous context
//     is refused with the exact production denial code authentication_failed.
//
// Only a case that turns RED when the fixture stops stamping proves the
// production denial path is reachable in-test without editing any
// internal/operatorauth/** code or changing denial semantics.
func TestMCPOperatorSeamReachesGateBothDirections(t *testing.T) {
	configureTestTarget(t)
	manifest := workspaceMutationFixture(t, "prod")
	t.Chdir(filepath.Dir(manifest))

	input := machineTargetCreateInput{Name: "seam-probe", RuntimeProvider: "podman", Access: "local-podman", Reference: "local"}

	// BEFORE (wrongly-green): the fixture stamps a verified operator, mirroring
	// the production runWithIO entrypoint, so the managed mutation succeeds. This
	// is the unmarked state the card warns about.
	stamped := testOperatorContext(context.Background())
	stampedSession := workspaceMutationClientWithContext(t, stamped)
	stampedResult, err := stampedSession.CallTool(stamped, &mcp.CallToolParams{
		Name: "baseharbor.target.create", Arguments: input,
	})
	if err != nil || stampedResult == nil || stampedResult.IsError {
		t.Fatalf("stamped managed target.create must reach the gate and be authorized: %#v %v", stampedResult, err)
	}

	// AFTER (red): the identical call on an anonymous, fixture-opted-out context
	// is refused with the exact production denial code.
	anonymous := operatorauth.WithEnforcement(context.Background())
	anonymousSession := workspaceMutationClientWithContext(t, anonymous)
	anonymousResult, err := anonymousSession.CallTool(anonymous, &mcp.CallToolParams{
		Name: "baseharbor.target.create", Arguments: input,
	})
	if err != nil || anonymousResult == nil || !anonymousResult.IsError {
		t.Fatal("managed target.create must be denied when the test context is marked anonymous")
	}

	stampedBody := formatMCPAcceptanceContent(stampedResult)
	anonymousBody := formatMCPAcceptanceContent(anonymousResult)
	if !strings.Contains(anonymousBody, `"code":"authentication_failed"`) {
		t.Fatalf("anonymous managed mutation is not denied with a typed authentication_failed: %s", anonymousBody)
	}
	if strings.Contains(stampedBody, `"code":"authentication_failed"`) {
		t.Fatalf("stamped managed mutation unexpectedly denied: %s", stampedBody)
	}
	if strings.TrimSpace(stampedBody) == strings.TrimSpace(anonymousBody) {
		t.Fatalf("both states produced identical output; the gate is not reachable: %s", stampedBody)
	}

	// The denial body must be structurally a machine error carrying the exact
	// production denial code, not a transport-level failure.
	var payload struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(anonymousBody), &payload); err != nil {
		t.Fatalf("anonymous denial is not structured JSON: %v: %s", err, anonymousBody)
	}
	if payload.Error == nil || payload.Error.Code != "authentication_failed" {
		t.Fatalf("anonymous denial error is not a typed authentication_failed machine error: %s", anonymousBody)
	}
	// The denial body must carry the exact production gate message verbatim.
	// source internal/operatorauth/authorization.go:286 and cmd/baha/mcp.go:301.
	// This pins the literal text so the case goes RED if the production denial
	// message changes.
	if payload.Error.Message != "Managed-environment machine operations require an authenticated BaseHarbor operator." {
		t.Fatalf("anonymous denial does not carry the exact production gate message: %q", payload.Error.Message)
	}
	// The anonymous context really did reach the seam and was refused there for a
	// missing principal, not short-circuited by a precondition: no verified
	// operator principal was present on the opt-out context.
	if _, ok := operatorauth.PrincipalFromContext(anonymous); ok {
		t.Fatal("anonymous context unexpectedly carried a verified principal")
	}
}
