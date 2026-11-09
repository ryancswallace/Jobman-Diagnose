package generation

import (
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

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

func configLogLimit(artifacts, bytes uint64) config.ClassLimits {
	return config.ClassLimits{MaximumArtifacts: artifacts, MaximumBytes: bytes}
}
