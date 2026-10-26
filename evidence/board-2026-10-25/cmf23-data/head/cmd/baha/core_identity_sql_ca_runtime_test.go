package main

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestCoreSharedIdentitySQLCARotationRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_IDENTITY_SQL_CA_ACCEPTANCE") != "1" {
		t.Skip("isolated native Core SQL/Identity trust rotation acceptance is opt-in")
	}
	for _, ha := range []bool{false, true} {
		if !t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) { runCoreOnlyBootstrapRuntimeWithHA(t, coreinstallation.Development, ha) }) {
			return
		}
	}
}

func verifyInstalledCoreIdentitySQLCARotation(t *testing.T, ctx context.Context, target deployment.ResolvedTarget, state coreinstallation.State, recovery string) {
	t.Helper()
	if recovery == "" {
		var err error
		recovery, _, err = resolveTargetRecoveryFile(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	rt, files, err := openBaoRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(bhruntime.CorePostgresCA(files))
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
	if err != nil || identity.SharedSQL == nil {
		t.Fatalf("native Identity did not use Core SQL: %v", err)
	}
	if err := rotateManagedOpenBao(ctx, recovery); err != nil {
		logCoreBootstrapFailure(t, rt, target.Name, target.RuntimeProvider)
		t.Fatalf("native Core trust rotation: %v", err)
	}
	after, err := os.ReadFile(bhruntime.CorePostgresCA(files))
	if err != nil || sha256.Sum256(before) == sha256.Sum256(after) {
		t.Fatal("Core PostgreSQL CA did not rotate")
	}
	if err := identityprovider.VerifyCoreIdentity(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer); err != nil {
		t.Fatalf("post-retirement database-backed Identity verification failed: %v", err)
	}
	// Reproduce the failing continuation using actual SQL-only apply/doctor/destroy.
	runManagedProviderOnlyReadinessRegression(t, ctx)
	t.Logf("actual native Core SQL CA overlap/retirement preserved shared Identity admin authentication, owned realm and discovery; HA=%t, runtime=%s; fresh application continuation and owned cleanup verified", state.Spec.HA, target.RuntimeProvider)
}
