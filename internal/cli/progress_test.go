package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPlainProgressReportsDistinctPhases(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	reporter := newProgressReporter(
		progressPlain, true, true, false, &output,
		progressDescriptor{profile: "local-vllm", model: "jobman-llama", timeout: 2 * time.Minute},
		func() progressTiming { t.Fatal("plain progress requested timing"); return progressTiming{} },
	)
	for _, phase := range []progressPhase{
		progressCollecting, progressPreparing, progressPreparing,
		progressWaiting, progressValidating, progressFallback,
	} {
		reporter.Phase(phase)
	}
	if err := reporter.Close(); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"AI analysis: collecting Jobman evidence",
		"AI analysis: preparing bounded, redacted evidence",
		"AI analysis: waiting for profile local-vllm (model jobman-llama)",
		"AI analysis: validating the model response",
		"AI analysis: using the deterministic fallback",
		"",
	}, "\n")
	if output.String() != want {
		t.Fatalf("plain progress = %q, want %q", output.String(), want)
	}
}

func TestProgressIsSilentWhenDisabledOrAutomaticOutputIsNoninteractive(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		mode        progressMode
		enabled     bool
		jsonOutput  bool
		interactive bool
	}{
		{name: "JSON", mode: progressAuto, enabled: true, jsonOutput: true, interactive: true},
		{name: "redirected", mode: progressAuto, enabled: true, jsonOutput: false, interactive: false},
		{name: "explicitly off", mode: progressOff, enabled: true, interactive: true},
		{name: "deterministic", mode: progressPlain, enabled: false, interactive: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			reporter := newProgressReporter(
				test.mode, test.enabled, test.jsonOutput, test.interactive, &output, progressDescriptor{},
				func() progressTiming { t.Fatal("silent progress requested timing"); return progressTiming{} },
			)
			reporter.Phase(progressWaiting)
			if err := reporter.Close(); err != nil {
				t.Fatal(err)
			}
			if output.Len() != 0 {
				t.Fatalf("progress = %q, want empty", output.String())
			}
		})
	}
}

func TestSpinnerProgressIsDelayedTimedAndCleared(t *testing.T) {
	t.Parallel()

	current := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	delay := make(chan time.Time)
	ticks := make(chan time.Time)
	stopped := false
	timing := progressTiming{
		now: func() time.Time { return current }, delay: delay, ticks: ticks,
		stop: func() { stopped = true },
	}
	var output bytes.Buffer
	reporter := newProgressReporter(
		progressAuto, true, false, true, &output,
		progressDescriptor{profile: "local-vllm", model: "jobman-llama", timeout: 2 * time.Minute},
		func() progressTiming { return timing },
	)
	reporter.Phase(progressWaiting)
	if output.Len() != 0 {
		t.Fatalf("progress before delay = %q", output.String())
	}
	delay <- current
	reporter.Phase(progressWaiting) // Synchronize after the delay draw.
	current = current.Add(18 * time.Second)
	ticks <- current
	reporter.Phase(progressWaiting) // Synchronize after the timed redraw.
	if err := reporter.Close(); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("progress timing was not stopped")
	}
	rendered := output.String()
	for _, expected := range []string{
		"AI analysis: waiting for profile local-vllm (model jobman-llama)",
		"18s", "timeout 2m", "Ctrl-C to cancel", "\r\x1b[2K",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("spinner progress = %q, want %q", rendered, expected)
		}
	}
	if !strings.HasSuffix(rendered, "\r\x1b[2K") {
		t.Fatalf("spinner progress was not cleared: %q", rendered)
	}
}

func TestProgressSanitizesConfiguredLabels(t *testing.T) {
	t.Parallel()

	text := progressPhaseText(progressWaiting, progressDescriptor{
		profile: "local\nprofile", model: "model\x1b[31m",
	})
	if strings.ContainsAny(text, "\n\r\x1b") || !strings.Contains(text, "local�profile") ||
		!strings.Contains(text, "model�[31m") {
		t.Fatalf("progress text = %q", text)
	}
}
