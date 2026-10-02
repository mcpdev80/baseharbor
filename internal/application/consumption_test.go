package application

import "testing"

func TestConsumptionIdentityIsLogicalAndStableAcrossRuntimeTopology(t *testing.T) {
	producer := LogicalProducerReference{
		ApplicationID: "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75",
		Component: "api",
		Interface: "http",
	}
	first := ConsumptionBindingID(producer)
	second := ConsumptionBindingID(producer)
	if first != second {
		t.Fatalf("binding identity changed: %q != %q", first, second)
	}
}

func TestSameAndCrossApplicationConsumptionUseSameContract(t *testing.T) {
	current := "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	same := ConsumptionRequirement{Name: "local-api", Component: "api", Interface: "http", Protocol: "https"}
	cross := ConsumptionRequirement{Name: "remote-api", ApplicationID: "b6e60b14-e4d3-4d27-bb91-26589f9dced5", Component: "api", Interface: "http", Protocol: "https"}
	for _, req := range []ConsumptionRequirement{same, cross} {
		if err := req.Validate(current); err != nil {
			t.Fatalf("%s: %v", req.Name, err)
		}
	}
	if got, _ := same.Producer(current); got.ApplicationID != current {
		t.Fatalf("same-app producer id = %q", got.ApplicationID)
	}
}

func TestConsumptionIntentRejectsAddressLeakage(t *testing.T) {
	req := ConsumptionRequirement{
		Name: "api",
		ApplicationID: "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75",
		Component: "api",
		Interface: "http",
		Protocol: "https://pod.namespace.svc",
	}
	if err := req.Validate("7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"); err == nil {
		t.Fatal("runtime/provider address leaked into portable consumption protocol")
	}
}


func TestConsumptionIntentRoundTrip(t *testing.T) {
	m := New("consumer", "prod", false, false, false)
	m.ApplicationID = "7a9dc6a7-9cab-4c62-a0dd-e55d5bf7ff75"
	m.Consumes = []ConsumptionRequirement{
		{Name: "producer-api", ApplicationID: "b6e60b14-e4d3-4d27-bb91-26589f9dced5", Component: "api", Interface: "http", Protocol: "https"},
	}
	data := m.YAML()
	got, err := ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Consumes) != 1 || got.Consumes[0] != m.Consumes[0] {
		t.Fatalf("round-trip consumes = %#v, want %#v", got.Consumes, m.Consumes)
	}
}
