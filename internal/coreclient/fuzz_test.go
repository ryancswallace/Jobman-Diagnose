package coreclient

import (
	"bytes"
	"testing"
)

func FuzzDecodeEvidence(f *testing.F) {
	f.Add([]byte(`{"kind":"jobman.diagnostic_evidence","schema_version":1}`))
	f.Add([]byte(`{"schema_version":1,"data":{"kind":"jobman.diagnostic_evidence"}}`))
	f.Fuzz(func(_ *testing.T, encoded []byte) {
		if _, err := DecodeEvidence(bytes.NewReader(encoded)); err != nil {
			return
		}
	})
}
