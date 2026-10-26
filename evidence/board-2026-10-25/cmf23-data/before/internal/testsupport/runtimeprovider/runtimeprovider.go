package runtimeprovider

import (
	"context"
	"os"
	"strings"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
	runtimeresolver "github.com/mcpdev80/baseharbor/internal/runtime/resolver"
)

func Resolve(ctx context.Context) (runtimecontract.RuntimeProvider, error) {
	kind, err := runtimecontract.ParseProviderKind(strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME")))
	if err != nil {
		return nil, err
	}
	return runtimeresolver.RuntimeProvider(ctx, kind)
}
