package sourcecontext

import (
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func TestAssessDetectsSourceContextMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		data       string
		log        string
		wantStatus AssessmentStatus
		wantReason string
	}{
		{
			name: "runtime path mismatch", path: "/checkout/current.go", data: "package current\n",
			log:        "2026-08-12T04:44:02Z worker.go:8: context deadline exceeded\n",
			wantStatus: AssessmentMismatch, wantReason: ReasonRuntimePathMismatch,
		},
		{
			name: "runtime line outside current file", path: "/checkout/worker.go", data: "package worker\n",
			log:        "worker.go:8: context deadline exceeded\n",
			wantStatus: AssessmentMismatch, wantReason: ReasonRuntimeLineOutOfRange,
		},
		{
			name: "recorded Python source differs", path: "/checkout/worker.py",
			data:       "def run():\n    return current()\n",
			log:        "Traceback (most recent call last):\n  File \"worker.py\", line 2, in run\n    return executed()\nRuntimeError: failed\n",
			wantStatus: AssessmentMismatch, wantReason: ReasonRuntimeContentMismatch,
		},
		{
			name: "recorded Python source matches", path: "/checkout/worker.py",
			data:       "def run():\n    return executed()\n",
			log:        "Traceback (most recent call last):\n  File \"worker.py\", line 2, in run\n    return executed()\nRuntimeError: failed\n",
			wantStatus: AssessmentConsistent, wantReason: ReasonRuntimeContentMatch,
		},
		{
			name: "cross-platform path matches", path: "/checkout/worker.go", data: "package worker\n",
			log:        `C:\service\worker.go:1: initialization failed` + "\n",
			wantStatus: AssessmentConsistent, wantReason: ReasonRuntimeLocationMatch,
		},
		{
			name: "no comparable location", path: "/checkout/worker.py", data: "raise RuntimeError()\n",
			log:        "RuntimeError: failed without a source location\n",
			wantStatus: AssessmentUnverified, wantReason: ReasonRuntimeLocationMissing,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := fullSourceContext(test.path, test.data)
			assessment := Assess([]diagnostic.Artifact{{
				ID: "artifact:stderr", Data: []byte(test.log), Disclosure: diagnostic.DisclosureLogContent,
			}}, source)
			if assessment.Status != test.wantStatus || assessment.Reason != test.wantReason {
				t.Fatalf("assessment = %#v", assessment)
			}
			if assessment.Status == AssessmentMismatch && !strings.Contains(MismatchMessage(assessment), "withheld") {
				t.Fatalf("mismatch message = %q", MismatchMessage(assessment))
			}
		})
	}
}

func TestAssessIgnoresNetworkEndpointsAndNonLogArtifacts(t *testing.T) {
	t.Parallel()

	source := fullSourceContext("/checkout/worker.go", "package worker\n")
	artifacts := []diagnostic.Artifact{
		{ID: "metadata", Data: []byte("other.go:9"), Disclosure: diagnostic.DisclosureMetadata},
		{ID: "stderr", Data: []byte("dial inventory.internal:443: connection refused"), Disclosure: diagnostic.DisclosureLogContent},
	}
	assessment := Assess(artifacts, source)
	if assessment.Status != AssessmentUnverified || assessment.Reason != ReasonRuntimeLocationMissing {
		t.Fatalf("assessment = %#v", assessment)
	}
}

func fullSourceContext(path, data string) diagnosis.SourceContext {
	// #nosec G115 -- the bounded test fixture line count is representable as uint64.
	lines := uint64(strings.Count(data, "\n"))
	if !strings.HasSuffix(data, "\n") {
		lines++
	}

	return diagnosis.SourceContext{
		ID: "context:source:001", Path: path, Mode: diagnosis.SourceContextFull,
		StartLine: 1, EndLine: lines, TotalLines: lines, Data: []byte(data),
	}
}
