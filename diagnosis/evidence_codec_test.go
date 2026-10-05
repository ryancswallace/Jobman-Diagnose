package diagnosis

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestFailureEvidenceEncodeByteCeiling(t *testing.T) {
	t.Parallel()
	core, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("x\n")
	source := SourceContext{
		ID: "context:source:001", Role: "source.context", Path: "/synthetic-private-canary",
		Language: "go", MediaType: "text/x-go", Mode: SourceContextFull,
		AnchorReason: "full_file", StartLine: 1, EndLine: 1, TotalLines: 1,
		ByteEnd: uint64(len(data)), FileBytes: uint64(len(data)),
		ContentBytes: uint64(len(data)), Data: data,
		Digest: contentDigest(data), ContentDigest: contentDigest(data),
		CapturedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Collector:  AnalyzerDescriptor{Name: "source", Version: "1"},
		Quality:    diagnostic.QualityPointInTime, Disclosure: DisclosureSourceContent,
	}
	seal := func(path string) FailureEvidence {
		t.Helper()
		source.Path = path
		value, sealErr := SealFailureEvidenceWithContext(core, nil, []SourceContext{source})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return value
	}
	var baseline bytes.Buffer
	if err := EncodeFailureEvidence(&baseline, seal(source.Path)); err != nil {
		t.Fatal(err)
	}
	basePath := source.Path
	for _, delta := range []int{-1, 0, 1} {
		value := seal(basePath + strings.Repeat("a", maximumFailureEvidenceBytes-baseline.Len()+delta))
		var encoded bytes.Buffer
		err := EncodeFailureEvidence(&encoded, value)
		if delta > 0 {
			assertOversizedFailureEvidence(t, value, &encoded, err)
			continue
		}
		if err != nil || encoded.Len() != maximumFailureEvidenceBytes+delta {
			t.Fatalf("encoding within byte ceiling failed: %v", err)
		}
		decoded, err := DecodeFailureEvidence(&encoded, DecodeLimits{})
		if err != nil || decoded.AnalysisEvidenceID != value.AnalysisEvidenceID {
			t.Fatalf("encoding within byte ceiling did not round trip: %v", err)
		}
	}
}

func assertOversizedFailureEvidence(t *testing.T, value FailureEvidence, encoded *bytes.Buffer, encodeErr error) {
	t.Helper()
	if encodeErr == nil || encoded.Len() != 0 || strings.Contains(encodeErr.Error(), "synthetic-private-canary") {
		t.Fatal("oversized encoding was written or exposed source content")
	}
	// Confirm this is a valid sealed wrapper whose wire encoding exceeds
	// the decoder ceiling by exactly one byte, including its newline.
	encoder := json.NewEncoder(encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatal(err)
	}
	if encoded.Len() != maximumFailureEvidenceBytes+1 {
		t.Fatal("oversized fixture does not straddle the byte ceiling")
	}
	if _, err := DecodeFailureEvidence(encoded, DecodeLimits{}); err == nil {
		t.Fatal("decoder accepted oversized encoding")
	}
}

func TestFailureEvidenceDecodePreservesLocalAndSharedSeals(t *testing.T) {
	t.Parallel()
	local, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../testdata/jobman-v2/shared-control-failure-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := diagnostic.Decode(bytes.NewReader(data), diagnostic.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, core := range []diagnostic.Evidence{local, shared} {
		wrapper, err := CoreFailureEvidence(core)
		if err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		if err = EncodeFailureEvidence(&encoded, wrapper); err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeFailureEvidence(bytes.NewReader(encoded.Bytes()), DecodeLimits{MaxBytes: int64(encoded.Len())})
		if err != nil || decoded.AnalysisEvidenceID != wrapper.AnalysisEvidenceID || decoded.Core.EvidenceID != core.EvidenceID {
			t.Fatalf("round trip changed sealed identity: %v", err)
		}
		if _, err := DecodeFailureEvidence(bytes.NewReader(encoded.Bytes()), DecodeLimits{MaxBytes: int64(encoded.Len() - 1)}); err == nil {
			t.Fatal("accepted input exceeding exact byte budget")
		}
	}
}

func TestFailureEvidenceDecodeRejectsCorruptionAndKeepsErrorsContentFree(t *testing.T) {
	t.Parallel()
	_, wrapper := validReportAndEvidence(t)
	var buffer bytes.Buffer
	if err := EncodeFailureEvidence(&buffer, wrapper); err != nil {
		t.Fatal(err)
	}
	valid := buffer.String()
	const canary = "synthetic-private-canary"
	tests := []struct {
		name   string
		input  io.Reader
		limits DecodeLimits
	}{
		{"nil", nil, DecodeLimits{}},
		{"reader failure", iotest.ErrReader(errors.New(canary)), DecodeLimits{}},
		{"negative bytes", strings.NewReader(valid), DecodeLimits{MaxBytes: -1}},
		{"overflow bytes", strings.NewReader(valid), DecodeLimits{MaxBytes: math.MaxInt64}},
		{"negative depth", strings.NewReader(valid), DecodeLimits{MaxDepth: -1}},
		{"excessive depth limit", strings.NewReader(valid), DecodeLimits{MaxDepth: 33}},
		{"nesting", strings.NewReader(valid), DecodeLimits{MaxDepth: 1}},
		{"root", strings.NewReader("[]"), DecodeLimits{}},
		{"null", strings.NewReader("null"), DecodeLimits{}},
		{"trailing document", strings.NewReader(valid + valid), DecodeLimits{}},
		{"duplicate", strings.NewReader(strings.Replace(valid, `"kind":`, `"kind":"`+canary+`","kind":`, 1)), DecodeLimits{}},
		{"case alias", strings.NewReader(strings.Replace(valid, `"kind":`, `"Kind":`, 1)), DecodeLimits{}},
		{"nested alias", strings.NewReader(strings.Replace(valid, `"job_id":`, `"Job_id":`, 1)), DecodeLimits{}},
		{"duplicate case alias", strings.NewReader(strings.Replace(valid, `"kind":`, `"Kind":"`+canary+`","kind":`, 1)), DecodeLimits{}},
		{"unknown field", strings.NewReader(strings.Replace(valid, `"kind":`, `"`+canary+`":true,"kind":`, 1)), DecodeLimits{}},
		{"bad type", strings.NewReader(strings.Replace(valid, `"schema_version":2`, `"schema_version":"`+canary+`"`, 1)), DecodeLimits{}},
		{"wrong wrapper seal", strings.NewReader(strings.Replace(valid, wrapper.AnalysisEvidenceID, "sha256:"+strings.Repeat("0", 64), 1)), DecodeLimits{}},
		{"wrong core seal", strings.NewReader(strings.Replace(valid, wrapper.Core.EvidenceID, "sha256:"+strings.Repeat("0", 64), 1)), DecodeLimits{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := DecodeFailureEvidence(test.input, test.limits)
			if err == nil || value.AnalysisEvidenceID != "" {
				t.Fatal("invalid stored evidence returned a usable wrapper")
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatal("decoder error exposed source content")
			}
		})
	}
}
