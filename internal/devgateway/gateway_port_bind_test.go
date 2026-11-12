package devgateway

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type boundPortRuntime struct {
	port    int
	running bool
}

func (r boundPortRuntime) PreferredLocalHTTPSPort() int                               { return r.port }
func (boundPortRuntime) ConfigProject(context.Context, string, string, string) error  { return nil }
func (boundPortRuntime) UpProject(context.Context, string, string, string) error      { return nil }
func (boundPortRuntime) DestroyProject(context.Context, string, string, string) error { return nil }
func (r boundPortRuntime) RunningServicesProject(context.Context, string, string, string) ([]string, error) {
	if r.running {
		return []string{"dev-gateway"}, nil
	}
	return nil, nil
}

func TestGatewayBindMigrationDoesNotClaimCorePort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	files := Files{Compose: filepath.Join(t.TempDir(), "compose.yaml")}
	if err := os.WriteFile(files.Compose, []byte("existing config"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, previousBind string
		running, retain    bool
	}{
		{"loopback-to-other-address", "127.0.0.1", true, false},
		{"failed-gateway-does-not-own-core-listener", "127.0.0.2", false, false},
		{"running-owned-listener-stays-stable", "127.0.0.2", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := resolveGatewayHostPortBound(context.Background(), files, state{HostPort: port, BindAddress: tc.previousBind}, boundPortRuntime{port: port, running: tc.running}, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			if (selected == port) != tc.retain {
				t.Fatalf("selected %d, occupied %d retain=%t", selected, port, tc.retain)
			}
		})
	}
}

func TestGatewayEarlyCoreAddressIsBoundToSelectedTarget(t *testing.T) {
	ctx := WithCoreAccessAddress(context.Background(), "own", "192.0.2.4")
	if got := requestedCoreAccessAddress(ctx, "own", nil); got == nil || *got != "192.0.2.4" {
		t.Fatal("early Core address absent")
	}
	if requestedCoreAccessAddress(ctx, "other", nil) != nil {
		t.Fatal("Core address crossed Target boundary")
	}
	explicit := "192.0.2.5"
	if got := requestedCoreAccessAddress(ctx, "own", &explicit); got == nil || *got != explicit {
		t.Fatal("explicit address lost")
	}
}
