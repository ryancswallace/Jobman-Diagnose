package generation

import (
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

//nolint:cyclop // The assertions jointly verify selection, provenance, and rebased enrichment.
func TestPrepareSelectsBuriedCausalContextWithinProfileBudget(t *testing.T) {
	t.Parallel()

	log := strings.Repeat("startup detail with no causal signal\n", 180) +
		"synchronize inventory: GET https://inventory.internal/snapshot: context deadline exceeded\n" +
		strings.Repeat("shutdown cleanup detail with no causal signal\n", 300)
	core, err := testevidence.Failed("nonzero_exit", []byte(log))
	if err != nil {
		t.Fatal(err)
	}
	core.Source.Capabilities = append(core.Source.Capabilities, "configured_value_redaction_v1")
	core.EvidenceID = ""
	core, err = diagnostic.Seal(core)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	report, err := deterministic(t).Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, true)
	profile.Disclosure["log_content"] = configLogLimit(1, 2048)
	prepared, err := Prepare(evidence, report, "test", profile, []string{"metadata", "log_content"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Request.Projection.Artifacts) != 1 || len(prepared.Request.Projection.Enrichment) != 1 {
		t.Fatalf("causal projection = %#v", prepared.Request.Projection)
	}
	artifact := prepared.Request.Projection.Artifacts[0]
	item := prepared.Request.Projection.Enrichment[0]
	if artifact.Selection != "causal_context" || artifact.AnchorReason != "causal_diagnostic" ||
		artifact.ContentBytes > 2048 || !artifact.Truncated ||
		!strings.Contains(artifact.Content, "context deadline exceeded") ||
		strings.Contains(artifact.Content, strings.Repeat("shutdown cleanup", 20)) ||
		artifact.ContentDigest != logContentDigest([]byte(artifact.Content)) {
		t.Fatalf("selected artifact = %#v", artifact)
	}
	if item.ByteStart >= item.ByteEnd || item.ByteEnd > artifact.ContentBytes ||
		!strings.Contains(artifact.Content[item.ByteStart:item.ByteEnd], "context deadline exceeded") ||
		prepared.Request.Manifest.ArtifactBytes != artifact.ContentBytes {
		t.Fatalf("rebased enrichment/manifest = %#v / %#v", item, prepared.Request.Manifest)
	}
}

func TestCausalContextOutranksGenericStderrTail(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte(strings.Repeat("generic stderr noise\n", 200)))
	if err != nil {
		t.Fatal(err)
	}
	stdout := core.Artifacts[0]
	stdout.ID = "artifact:run:00000000000000000001:stdout"
	stdout.Stream = "stdout"
	stdout.Data = []byte("connect inventory: dial tcp 127.0.0.1:4319: connect: connection refused\n")
	stdout.OriginalBytes = uint64(len(stdout.Data))
	stdout.ByteStart = 0
	stdout.ByteEnd = uint64(len(stdout.Data))
	stdout.SelectedBytes = uint64(len(stdout.Data))
	stdout.ContentBytes = uint64(len(stdout.Data))
	stdout.Digest = ""
	core.Artifacts = append(core.Artifacts, stdout)
	core.Source.Capabilities = append(core.Source.Capabilities, "configured_value_redaction_v1")
	core.EvidenceID = ""
	core, err = diagnostic.Seal(core)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	report, err := deterministic(t).Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, true)
	profile.Disclosure["log_content"] = configLogLimit(2, 4096)
	prepared, err := Prepare(evidence, report, "test", profile, []string{"metadata", "log_content"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(prepared.Request.Manifest.ArtifactIDs, stdout.ID) ||
		len(prepared.Request.Projection.Artifacts) != 2 {
		t.Fatalf("ranked projection = %#v", prepared.Request.Projection.Artifacts)
	}
	var causal *provider.ProjectedArtifact
	for index := range prepared.Request.Projection.Artifacts {
		if prepared.Request.Projection.Artifacts[index].ID == stdout.ID {
			causal = &prepared.Request.Projection.Artifacts[index]
		}
	}
	if causal == nil || causal.Selection != "causal_context" ||
		!strings.Contains(causal.Content, "connection refused") {
		t.Fatalf("causal stdout projection = %#v", causal)
	}
}

func TestCausalContextLineBoundExcludesRepeatedNoiseBelowByteCeiling(t *testing.T) {
	t.Parallel()

	log := strings.Repeat("heartbeat accepted queue_depth=0\n", 180) +
		"checkpoint writer: write /srv/checkpoints/partition-17.json: no space left on device\n"
	core, err := testevidence.Failed("nonzero_exit", []byte(log))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	contexts := selectLogContexts(core.Artifacts, evidence.Enrichment, configLogLimit(1, 64*1024))
	if len(contexts) != 1 {
		t.Fatalf("causal contexts = %#v", contexts)
	}
	artifact := contexts[0].artifact
	if artifact.ByteStart == 0 || strings.Count(artifact.Content, "heartbeat accepted") > causalContextLinesBefore ||
		!strings.Contains(artifact.Content, "no space left on device") {
		t.Fatalf("line-bounded causal context = %#v", artifact)
	}
}

func TestPreparePreservesColoredLogAndProjectsPlainDiagnosticLine(t *testing.T) {
	t.Parallel()

	message := "2026-08-12T04:18:31Z ERROR exporter: x509: certificate signed by unknown authority"
	colored := "\x1b[31m" + message + "\x1b[0m"
	controlled := "HTTP 503 service unavailable\x00, retrying"
	log := controlled + "\n" + colored + "\n"
	core, err := testevidence.Failed("nonzero_exit", []byte(log))
	if err != nil {
		t.Fatal(err)
	}
	core.Source.Capabilities = append(core.Source.Capabilities, "configured_value_redaction_v1")
	core.EvidenceID = ""
	core, err = diagnostic.Seal(core)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	report, err := deterministic(t).Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(evidence, report, "test", testProfile(t, true), []string{"metadata", "log_content"})
	if err != nil {
		t.Fatal(err)
	}

	projection := prepared.Request.Projection
	if len(projection.Artifacts) != 1 || projection.Artifacts[0].Content != log || len(projection.Enrichment) != 2 {
		t.Fatalf("projection changed the log or enrichment set: %#v", projection)
	}
	expected := map[string]struct {
		excerpt string
		lines   []string
	}{
		"tls_verification_failed": {excerpt: colored, lines: []string{message}},
		"service_unavailable":     {excerpt: controlled},
	}
	for _, item := range projection.Enrichment {
		want := expected[item.Format]
		excerpt := strings.TrimSuffix(log[item.ByteStart:item.ByteEnd], "\n")
		if excerpt != want.excerpt || !slices.Equal(item.DiagnosticLines, want.lines) {
			t.Fatalf("enrichment %s projected range %q and lines %#v, want %q and %#v",
				item.Format, excerpt, item.DiagnosticLines, want.excerpt, want.lines)
		}
	}
}

func TestOversizedStructuredRangeKeepsAnchorInsideSelectedContext(t *testing.T) {
	t.Parallel()

	log := "Traceback (most recent call last):\n" + strings.Repeat("  File \"worker.py\", line 12, in run\n", 300) +
		"ValueError: rejected inventory record\n"
	core, err := testevidence.Failed("nonzero_exit", []byte(log))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	contexts := selectLogContexts(core.Artifacts, evidence.Enrichment, configLogLimit(1, 2048))
	if len(contexts) != 1 || contexts[0].artifact.Selection != "causal_context" ||
		contexts[0].artifact.AnchorLine < contexts[0].artifact.StartLine ||
		contexts[0].artifact.AnchorLine > contexts[0].artifact.EndLine {
		t.Fatalf("structured context = %#v", contexts)
	}
}

func TestLogContextFallsBackToBoundedTerminalOutput(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte(strings.Repeat("ordinary output\n", 400)+"terminal marker\n"))
	if err != nil {
		t.Fatal(err)
	}
	core.Source.Capabilities = append(core.Source.Capabilities, "configured_value_redaction_v1")
	core.EvidenceID = ""
	core, err = diagnostic.Seal(core)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	report, err := deterministic(t).Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, true)
	profile.Disclosure["log_content"] = configLogLimit(1, 1024)
	prepared, err := Prepare(evidence, report, "test", profile, []string{"metadata", "log_content"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Request.Projection.Artifacts) != 1 {
		t.Fatalf("terminal projection = %#v", prepared.Request.Projection)
	}
	artifact := prepared.Request.Projection.Artifacts[0]
	if artifact.Selection != "tail" || artifact.AnchorReason != "terminal_output" ||
		artifact.ByteStart == 0 || artifact.ByteEnd != artifact.FileBytes || artifact.EndLine != artifact.TotalLines ||
		!strings.Contains(artifact.Content, "terminal marker") || !artifact.Truncated {
		t.Fatalf("terminal context = %#v", artifact)
	}
}

func TestLogContextCandidateRanking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		left, right logContextCandidate
		want        int
	}{
		{
			name:  "anchor score",
			left:  logContextCandidate{anchorScore: 300},
			right: logContextCandidate{anchorScore: 200},
			want:  -100,
		},
		{
			name:  "newer run",
			left:  logContextCandidate{artifact: diagnostic.Artifact{Run: 2}},
			right: logContextCandidate{artifact: diagnostic.Artifact{Run: 1}},
			want:  -1,
		},
		{
			name:  "older run",
			left:  logContextCandidate{artifact: diagnostic.Artifact{Run: 1}},
			right: logContextCandidate{artifact: diagnostic.Artifact{Run: 2}},
			want:  1,
		},
		{
			name:  "stderr first",
			left:  logContextCandidate{artifact: diagnostic.Artifact{Stream: "stderr"}},
			right: logContextCandidate{artifact: diagnostic.Artifact{Stream: "stdout"}},
			want:  -1,
		},
		{
			name:  "stderr second",
			left:  logContextCandidate{artifact: diagnostic.Artifact{Stream: "stdout"}},
			right: logContextCandidate{artifact: diagnostic.Artifact{Stream: "stderr"}},
			want:  1,
		},
		{
			name:  "artifact ID",
			left:  logContextCandidate{artifact: diagnostic.Artifact{ID: "a", Stream: "stdout"}},
			right: logContextCandidate{artifact: diagnostic.Artifact{ID: "b", Stream: "stdout"}},
			want:  -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := compareLogContextCandidates(test.left, test.right); got != test.want {
				t.Fatalf("compareLogContextCandidates() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestDiagnosticAnchorScoringAndSpan(t *testing.T) {
	t.Parallel()

	for format, want := range map[string]int{
		"connection_refused": 300,
		"python_traceback":   250,
		"unknown":            200,
	} {
		if got := diagnosticAnchorScore(diagnosis.EnrichmentItem{Format: format}); got != want {
			t.Fatalf("diagnosticAnchorScore(%q) = %d, want %d", format, got, want)
		}
	}

	anchor := diagnosis.EnrichmentItem{ByteStart: 5, ByteEnd: 13}
	if got := contextAnchor(logContextCandidate{}, 20); got != nil {
		t.Fatalf("contextAnchor(no anchor) = %#v", got)
	}
	single := logContextCandidate{anchor: &anchor, enrichment: []diagnosis.EnrichmentItem{anchor}}
	if got := contextAnchor(single, 20); got != &anchor {
		t.Fatalf("contextAnchor(single) = %#v, want original anchor", got)
	}
	multiple := logContextCandidate{
		anchor: &anchor,
		enrichment: []diagnosis.EnrichmentItem{
			anchor,
			{ByteStart: 1, ByteEnd: 3},
		},
	}
	if got := contextAnchor(multiple, 10); got != &anchor {
		t.Fatalf("contextAnchor(oversized span) = %#v, want original anchor", got)
	}
	if got := contextAnchor(multiple, 20); got == nil || got.ByteStart != 1 || got.ByteEnd != 13 {
		t.Fatalf("contextAnchor(bounded span) = %#v", got)
	}
}

func TestSelectedLogByteRanges(t *testing.T) {
	t.Parallel()

	data := []byte("zero\none\ntwo\nthree\n")
	if start, end := selectedLogByteRange(data, nil, 0); start != 0 || end != 0 {
		t.Fatalf("selectedLogByteRange(zero budget) = %d:%d", start, end)
	}
	if start, end := selectedLogByteRange(data, nil, 100); start != 0 || end != len(data) {
		t.Fatalf("selectedLogByteRange(full) = %d:%d", start, end)
	}
	anchor := diagnosis.EnrichmentItem{ByteStart: 5, ByteEnd: 13}
	if start, end := selectedLogByteRange(data, &anchor, 8); start != 9 || end != 13 {
		t.Fatalf("selectedLogByteRange(wide anchor) = %d:%d", start, end)
	}
	narrow := diagnosis.EnrichmentItem{ByteStart: 9, ByteEnd: 12}
	if start, end := selectedLogByteRange(data, &narrow, 10); start != 5 || end != 13 {
		t.Fatalf("selectedLogByteRange(context) = %d:%d", start, end)
	}
	plain := []byte("abcdefghijklmnop")
	late := diagnosis.EnrichmentItem{ByteStart: 12, ByteEnd: 15}
	if start, end := selectedLogByteRange(plain, &late, 6); start != 9 || end != 15 {
		t.Fatalf("selectedLogByteRange(late anchor) = %d:%d", start, end)
	}
	fallback := diagnosis.EnrichmentItem{ByteStart: 6, ByteEnd: 8}
	if start, end := selectedLogByteRange(data, &fallback, 5); start != 3 || end != 8 {
		t.Fatalf("selectedLogByteRange(line fallback) = %d:%d", start, end)
	}
}

func TestLogLineBoundaries(t *testing.T) {
	t.Parallel()

	data := []byte("zero\none\ntwo\nthree\n")
	if got := nextLineStart(data, 0); got != 0 {
		t.Fatalf("nextLineStart(0) = %d", got)
	}
	if got := nextLineStart(data, 1); got != 5 {
		t.Fatalf("nextLineStart(1) = %d", got)
	}
	if got := nextLineStart(data, 17); got != 17 {
		t.Fatalf("nextLineStart(last line) = %d", got)
	}
	if got := previousLineEnd(data, len(data)); got != len(data) {
		t.Fatalf("previousLineEnd(end) = %d", got)
	}
	if got := previousLineEnd(data, 8); got != 5 {
		t.Fatalf("previousLineEnd(8) = %d", got)
	}
	plain := []byte("abcdefghijklmnop")
	if got := previousLineEnd(plain, 8); got != 8 {
		t.Fatalf("previousLineEnd(no newline) = %d", got)
	}
	if got := logLineCount(nil); got != 0 {
		t.Fatalf("logLineCount(nil) = %d", got)
	}
	if got := logLineCount([]byte("one\n")); got != 1 {
		t.Fatalf("logLineCount(trailing newline) = %d", got)
	}
	if got := logLineCount([]byte("one\ntwo")); got != 2 {
		t.Fatalf("logLineCount(no trailing newline) = %d", got)
	}
	if got := logLineAt(data, -1); got != 1 {
		t.Fatalf("logLineAt(negative) = %d", got)
	}
	if got := logLineAt(data, len(data)+1); got != 5 {
		t.Fatalf("logLineAt(after end) = %d", got)
	}
}

func configLogLimit(artifacts, bytes uint64) config.ClassLimits {
	return config.ClassLimits{MaximumArtifacts: artifacts, MaximumBytes: bytes}
}
