package targetaccess

import "testing"

func TestBuiltInProvidersExposeIndependentCapabilities(t *testing.T) {
	local, ok := BuiltInDescriptor(ProviderLocal)
	if !ok {
		t.Fatal("local access provider descriptor missing")
	}
	if local.Remote || local.Capabilities.PeerIdentity {
		t.Fatalf("local provider unexpectedly remote: %#v", local)
	}
	if err := RequireCapabilities(local, CapabilityConnect, CapabilityTypedRealization); err != nil {
		t.Fatal(err)
	}

	connector, ok := BuiltInDescriptor(ProviderNodeConnector)
	if !ok {
		t.Fatal("node connector descriptor missing")
	}
	if !connector.Remote || !connector.Capabilities.PeerIdentity || !connector.Capabilities.Stream {
		t.Fatalf("node connector capabilities incomplete: %#v", connector)
	}

	native, ok := BuiltInDescriptor(ProviderNativeAPI)
	if !ok {
		t.Fatal("native-api descriptor missing")
	}
	if !native.Capabilities.NativeContext {
		t.Fatalf("native-api lacks native-context capability: %#v", native)
	}
}

func TestParseProviderKindAllowsFutureProviderNames(t *testing.T) {
	kind, err := ParseProviderKind("cloud/vendor-api")
	if err != nil {
		t.Fatal(err)
	}
	if kind != "cloud/vendor-api" {
		t.Fatalf("kind = %q", kind)
	}
	if _, known := BuiltInDescriptor(kind); known {
		t.Fatal("future provider unexpectedly treated as built-in")
	}
}
