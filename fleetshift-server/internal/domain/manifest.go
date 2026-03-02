package domain

import "encoding/json"

// ResourceType identifies the kind of declarative resource in a [Manifest].
// The platform treats manifest content as opaque; ResourceType provides
// metadata that delivery agents use for dispatch and validation.
type ResourceType string

// Manifest is an opaque declarative payload delivered to a target.
// ResourceType identifies what the payload represents (e.g.
// "api.kind.cluster"); Raw holds the actual content as opaque JSON.
type Manifest struct {
	ResourceType ResourceType
	Raw          json.RawMessage
}
