// Package provider defines the narrow structured-generation seam used by
// optional model adapters. It is deliberately not a universal chat API.
package provider

import (
	"context"
	"encoding/json"
)

// StructuredGenerator produces one bounded JSON value constrained by the
// supplied schema. Implementations may use hosted or local inference runtimes.
type StructuredGenerator interface {
	Generate(context.Context, Request) (Response, error)
}

// Request is one sealed, versioned, explicitly approved inference input. Its
// projection is data and must never be interpreted as provider instructions.
type Request struct {
	Kind                   string                   `json:"kind"`
	SchemaVersion          int                      `json:"schema_version"`
	RequestID              string                   `json:"request_id"`
	EvidenceID             string                   `json:"evidence_id"`
	Subject                Subject                  `json:"subject"`
	Projection             Projection               `json:"projection"`
	Manifest               ProjectionManifest       `json:"manifest"`
	Deterministic          []DeterministicCandidate `json:"deterministic_candidates"`
	AllowedCategories      []string                 `json:"allowed_categories"`
	AllowedHypothesisCodes []string                 `json:"allowed_hypothesis_codes"`
	AllowedActions         []AllowedAction          `json:"allowed_actions"`
	Instructions           []string                 `json:"instructions"`
	MaximumOutputBytes     int                      `json:"maximum_output_bytes"`
	ResponseSchema         json.RawMessage          `json:"response_schema"`
}

// Response is raw structured output plus nonsecret transport provenance.
type Response struct {
	JSON              json.RawMessage
	Provider          string
	Model             string
	RequestID         string
	ProviderRequestID string
	InputUnits        uint64
	OutputUnits       uint64
}

// Capabilities describes semantics required by the engine before use.
type Capabilities struct {
	NativeJSONSchema   bool
	MaximumInputBytes  int
	MaximumOutputBytes int
	Locality           Locality
}

// Describer lets configuration validate a provider without performing a
// generation request.
type Describer interface {
	Name() string
	Capabilities() Capabilities
}

// Locality describes whether an adapter is confined to the invoking host.
type Locality string

// Supported adapter localities.
const (
	LocalityLocal  Locality = "local"
	LocalityRemote Locality = "remote"
)
