package domain

import (
	"encoding/json"
	"fmt"
)

// ManifestEnvelope wraps a managed resource spec with identity fields.
// Addons unwrap the envelope to extract the identity fields and inner spec.
type ManifestEnvelope struct {
	Name ResourceName         `json:"name"`
	UID  ExtensionResourceUID `json:"uid"`
	Spec json.RawMessage      `json:"spec"`
}

// WrapManifestEnvelope marshals a ManifestEnvelope into JSON suitable
// for use as Manifest.Raw.
func WrapManifestEnvelope(name ResourceName, uid ExtensionResourceUID, spec json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(ManifestEnvelope{
		Name: name,
		UID:  uid,
		Spec: spec,
	})
}

// UnwrapManifestEnvelope extracts identity and the inner spec from a
// manifest payload produced by WrapManifestEnvelope.
func UnwrapManifestEnvelope(raw json.RawMessage) (*ManifestEnvelope, error) {
	var env ManifestEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unwrap manifest envelope: %w", err)
	}
	if env.Name == "" {
		return nil, fmt.Errorf("unwrap manifest envelope: name is required")
	}
	if env.Spec == nil {
		return nil, fmt.Errorf("unwrap manifest envelope: spec is required")
	}
	return &env, nil
}
