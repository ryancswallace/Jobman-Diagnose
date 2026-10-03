package diagnosis

import (
	"bytes"
	"testing"
)

func FuzzDecodeReport(f *testing.F) {
	f.Add([]byte(`{"kind":"jobman.diagnosis_report","schema_version":1}`))
	f.Add([]byte(`{"kind":"jobman.diagnosis_report","kind":"duplicate"}`))
	f.Fuzz(func(_ *testing.T, encoded []byte) {
		if _, err := Decode(bytes.NewReader(encoded), DecodeLimits{MaxBytes: 64 * 1024, MaxDepth: 16}); err != nil {
			return
		}
	})
}

func FuzzDecodeFailureEvidence(f *testing.F) {
	f.Add([]byte(`{"kind":"jobman.failure_evidence","schema_version":2}`))
	f.Add([]byte(`{"kind":"jobman.failure_evidence","Kind":"duplicate"}`))
	f.Fuzz(func(t *testing.T, encoded []byte) {
		value, err := DecodeFailureEvidence(bytes.NewReader(encoded), DecodeLimits{MaxBytes: 64 * 1024, MaxDepth: 16})
		if err == nil && VerifyFailureEvidence(value) != nil {
			t.Fatal("decoder returned unverified evidence")
		}
	})
}
