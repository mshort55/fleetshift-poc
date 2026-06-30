package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func TestManifestEnvelope_RoundTrip(t *testing.T) {
	name := domain.ResourceName("clusters/prod")
	uid := domain.NewExtensionResourceUID()
	spec := json.RawMessage(`{"endpointAccess":"PublicAndPrivate"}`)

	raw, err := domain.WrapManifestEnvelope(name, uid, spec)
	if err != nil {
		t.Fatalf("WrapManifestEnvelope() error = %v", err)
	}

	got, err := domain.UnwrapManifestEnvelope(raw)
	if err != nil {
		t.Fatalf("UnwrapManifestEnvelope() error = %v", err)
	}

	if got.Name != name {
		t.Errorf("Name = %q, want %q", got.Name, name)
	}
	if got.UID != uid {
		t.Errorf("UID = %q, want %q", got.UID, uid)
	}
	if string(got.Spec) != string(spec) {
		t.Errorf("Spec = %s, want %s", got.Spec, spec)
	}
}

func TestUnwrapManifestEnvelope_InvalidJSON(t *testing.T) {
	_, err := domain.UnwrapManifestEnvelope(json.RawMessage(`{{{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestUnwrapManifestEnvelope_MissingName(t *testing.T) {
	raw := json.RawMessage(`{"uid":"550e8400-e29b-41d4-a716-446655440000","spec":{}}`)
	_, err := domain.UnwrapManifestEnvelope(raw)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestUnwrapManifestEnvelope_MissingSpec(t *testing.T) {
	raw := json.RawMessage(`{"name":"clusters/prod","uid":"550e8400-e29b-41d4-a716-446655440000"}`)
	_, err := domain.UnwrapManifestEnvelope(raw)
	if err == nil {
		t.Fatal("expected error for missing spec")
	}
}
