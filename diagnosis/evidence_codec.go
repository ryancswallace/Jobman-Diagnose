package diagnosis

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

const maximumFailureEvidenceBytes = 4 * 1024 * 1024

// DecodeFailureEvidence reloads one immutable analysis wrapper for report and
// citation verification. It bounds input before decoding, rejects duplicate
// fields/trailing documents, and verifies both the wrapper and its core seal.
// Zero limits select 4 MiB and depth 32; callers can require tighter limits but
// cannot raise these bounds. Errors never contain source bytes or item IDs.
// This function does not acquire evidence, enrich it or invoke any provider.
func DecodeFailureEvidence(source io.Reader, limits DecodeLimits) (FailureEvidence, error) {
	if source == nil {
		return FailureEvidence{}, errors.New("decode failure evidence: source is nil")
	}
	maximumBytes, maximumDepth := limits.MaxBytes, limits.MaxDepth
	if maximumBytes == 0 {
		maximumBytes = maximumFailureEvidenceBytes
	}
	if maximumDepth == 0 {
		maximumDepth = defaultMaximumReportDepth
	}
	if maximumBytes < 1 || maximumBytes > maximumFailureEvidenceBytes || maximumDepth < 1 || maximumDepth > defaultMaximumReportDepth {
		return FailureEvidence{}, errors.New("decode failure evidence: limits exceed supported bounds")
	}
	encoded, err := io.ReadAll(io.LimitReader(source, maximumBytes+1))
	if err != nil {
		return FailureEvidence{}, errors.New("decode failure evidence: read failed")
	}
	if int64(len(encoded)) > maximumBytes {
		return FailureEvidence{}, errors.New("decode failure evidence: input exceeds byte limit")
	}
	if err := validateJSONObject(encoded, maximumDepth); err != nil {
		return FailureEvidence{}, errors.New("decode failure evidence: invalid or excessive JSON structure")
	}
	// encoding/json matches struct fields without regard to case. Check exact
	// contract names first so aliases cannot overwrite another sealed value.
	var document any
	shape := json.NewDecoder(bytes.NewReader(encoded))
	shape.UseNumber()
	if shape.Decode(&document) != nil || !exactEvidenceFields(document, reflect.TypeFor[FailureEvidence]()) {
		return FailureEvidence{}, errors.New("decode failure evidence: invalid document fields")
	}
	var value FailureEvidence
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return FailureEvidence{}, errors.New("decode failure evidence: invalid document fields")
	}
	if err := VerifyFailureEvidence(value); err != nil {
		return FailureEvidence{}, errors.New("decode failure evidence: invalid sealed evidence")
	}
	return value, nil
}

// Raw fact values deliberately retain their own versioned schema. Custom JSON
// scalars such as timestamps are validated by their decoder and seal checks.
func exactEvidenceFields(value any, schema reflect.Type) bool {
	if reflect.PointerTo(schema).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return true
	}
	if schema.Kind() == reflect.Pointer {
		return exactEvidenceFields(value, schema.Elem())
	}
	if schema.Kind() == reflect.Struct {
		return exactEvidenceObject(value, schema)
	}
	if schema.Kind() == reflect.Slice || schema.Kind() == reflect.Array {
		if items, ok := value.([]any); ok {
			for _, item := range items {
				if !exactEvidenceFields(item, schema.Elem()) {
					return false
				}
			}
		}
	}
	return true
}

func exactEvidenceObject(value any, schema reflect.Type) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return value == nil
	}
	fields := make(map[string]reflect.Type, schema.NumField())
	for field := range schema.NumField() {
		definition := schema.Field(field)
		name, _, _ := strings.Cut(definition.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = definition.Type
		}
	}
	for name, child := range object {
		field, found := fields[name]
		if !found || !exactEvidenceFields(child, field) {
			return false
		}
	}
	return true
}
