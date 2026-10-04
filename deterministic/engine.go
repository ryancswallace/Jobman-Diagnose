// Package deterministic exposes the supported, network-free diagnosis engine.
// It performs no evidence acquisition, process execution, source-file access,
// provider calls, credential loading or job changes.
package deterministic

import (
	"context"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
)

// New constructs a diagnostician with an explicit version and clock. The
// returned implementation validates sealed evidence and every report citation.
func New(companionVersion string, now func() time.Time) (diagnosis.Diagnostician, error) {
	return engine.New(companionVersion, now)
}

// Prepare verifies a core bundle and derives bounded, attributed structures
// solely from its already-sealed artifacts. Persist this exact wrapper with
// the report: enrichment citations refer to its immutable artifact ranges.
// Use diagnosis.CoreFailureEvidence instead when core-only analysis is wanted.
func Prepare(ctx context.Context, core diagnostic.Evidence) (diagnosis.FailureEvidence, error) {
	return enrichment.Collect(ctx, core)
}
