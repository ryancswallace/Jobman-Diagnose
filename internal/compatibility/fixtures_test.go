package compatibility_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/presentation"
)

const forbiddenFixtureCanary = "JOBMAN_DIAG_SECRET_CANARY_7f84d1"

type manifest struct {
	JobmanRelease  string  `json:"jobman_release"`
	EvidenceSchema int     `json:"evidence_schema"`
	Fixtures       []entry `json:"fixtures"`
}

type entry struct {
	File       string `json:"file"`
	EvidenceID string `json:"evidence_id"`
	SHA256     string `json:"sha256"`
}

func TestCopiedCoreFixtures(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "testdata", "jobman-v1")
	origin := readManifest(t, filepath.Join(root, "manifest.json"))
	if origin.EvidenceSchema != diagnostic.SchemaVersion || origin.JobmanRelease == "" {
		t.Fatalf("fixture origin = %#v", origin)
	}
	diagnostician, err := engine.New("compatibility-test", func() time.Time {
		return time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range origin.Fixtures {
		t.Run(fixture.File, func(t *testing.T) {
			t.Parallel()

			testCopiedFixture(t, root, fixture, diagnostician)
		})
	}
}

func testCopiedFixture(t *testing.T, root string, fixture entry, diagnostician diagnosis.Diagnostician) {
	t.Helper()

	encoded := readFile(t, filepath.Join(root, fixture.File))
	digest := sha256.Sum256(encoded)
	if got := hex.EncodeToString(digest[:]); got != fixture.SHA256 {
		t.Fatalf("file SHA-256 = %s, want %s", got, fixture.SHA256)
	}
	if bytes.Contains(encoded, []byte(forbiddenFixtureCanary)) {
		t.Fatal("copied evidence contains the secret canary")
	}
	evidence, err := diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.EvidenceID != fixture.EvidenceID {
		t.Fatalf("evidence ID = %s, want %s", evidence.EvidenceID, fixture.EvidenceID)
	}
	failureEvidence, err := diagnosis.CoreFailureEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), failureEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnosis.ValidateAgainstEvidence(report, failureEvidence); err != nil {
		t.Fatal(err)
	}
	var human bytes.Buffer
	if err := presentation.Human(&human, report); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(human.Bytes(), []byte(forbiddenFixtureCanary)) {
		t.Fatal("rendered diagnosis contains the secret canary")
	}
}

func TestRejectsCopiedUnsupportedSchema(t *testing.T) {
	t.Parallel()

	encoded := readFile(t, filepath.Join("..", "..", "testdata", "jobman-v1", "unsupported-schema-v2.json"))
	if _, err := diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{}); err == nil {
		t.Fatal("Decode(unsupported schema) error = nil")
	}
}

func readManifest(t *testing.T, path string) manifest {
	t.Helper()

	var value manifest
	if err := json.Unmarshal(readFile(t, path), &value); err != nil {
		t.Fatal(err)
	}

	return value
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	// #nosec G304 -- every path is rooted in repository-owned compatibility testdata.
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return value
}
