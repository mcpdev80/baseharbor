package credential

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNoopBrokerFailsClosed(t *testing.T) {
	broker := NoopBroker{}
	_, err := broker.Resolve(context.Background(), "secret-ref", "tenant-a")
	if !errors.Is(err, ErrBrokerNotConfigured) {
		t.Fatalf("expected ErrBrokerNotConfigured, got %v", err)
	}
}

func TestNoopBrokerRejectsEmptyRef(t *testing.T) {
	broker := NoopBroker{}
	_, err := broker.Resolve(context.Background(), "", "tenant-a")
	if !errors.Is(err, ErrEmptyRef) {
		t.Fatalf("expected ErrEmptyRef, got %v", err)
	}
}

func TestDataDoesNotSerializePayload(t *testing.T) {
	data := Data{Payload: map[string][]byte{"token": []byte("super-secret")}}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-secret") || strings.Contains(string(raw), "token") {
		t.Fatalf("credential payload leaked into JSON: %s", raw)
	}
}
