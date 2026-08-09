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
