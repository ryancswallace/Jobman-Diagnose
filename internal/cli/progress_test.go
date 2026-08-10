package cli

import (
	"bytes"
	"errors"
	"os"
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

//nolint:cyclop // One table-free test keeps the small progress boundary helpers readable together.
func TestProgressBoundaryHelpers(t *testing.T) {
	t.Parallel()

	environment := defaultRuntimeEnvironment()
	if environment.interactive(&bytes.Buffer{}) {
		t.Fatal("ordinary writer reported as interactive")
	}
	if environment.lookupEnv == nil {
		t.Fatal("default environment lookup is nil")
	}
	timing := environment.newProgressTiming()
	if timing.now == nil || timing.delay == nil || timing.ticks == nil || timing.stop == nil {
		t.Fatalf("default timing is incomplete: %#v", timing)
	}
	timing.stop()

	file, err := os.CreateTemp(t.TempDir(), "progress-terminal")
	if err != nil {
		t.Fatal(err)
	}
	if terminalWriter(file) {
		t.Fatal("regular file reported as terminal")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if terminalWriter(file) {
		t.Fatal("closed file reported as terminal")
	}

	if formatProgressElapsed(-time.Second) != "0s" || formatProgressElapsed(1500*time.Millisecond) != "1s" {
		t.Fatal("elapsed time formatting changed")
	}
	if formatProgressDuration(2*time.Minute) != "2m" || formatProgressDuration(1500*time.Millisecond) != "2s" {
		t.Fatal("duration formatting changed")
	}
	if progressPhaseText("unknown", progressDescriptor{}) != "working" ||
		progressPhaseText(progressWaiting, progressDescriptor{profile: "local"}) != "waiting for profile local" {
		t.Fatal("fallback progress labels changed")
	}
	if safeProgressLabel(strings.Repeat("a", 80)) != strings.Repeat("a", 64) {
		t.Fatal("progress label was not bounded")
	}

	var mode progressModeValue
	if err := mode.Set(" PLAIN "); err != nil || mode.String() != "plain" {
		t.Fatalf("progress mode = %q, %v", mode.String(), err)
	}
	if err := mode.Set("animated"); err == nil || (*progressModeValue)(nil).String() != "" {
		t.Fatalf("invalid/nil progress mode = %v/%q", err, (*progressModeValue)(nil).String())
	}
}

func TestProgressStopsWritingAfterWriterFailure(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write failed")
	plain := &plainProgress{destination: failingWriter{err: writeErr}}
	plain.Phase(progressCollecting)
	plain.Phase(progressWaiting)
	if !errors.Is(plain.Close(), writeErr) || plain.last != progressCollecting {
		t.Fatalf("plain progress state/error = %q/%v", plain.last, plain.Close())
	}
	if err := drawProgress(
		failingWriter{err: writeErr}, progressDescriptor{}, progressCollecting,
		time.Time{}, time.Time{}, 0, writeErr,
	); !errors.Is(err, writeErr) {
		t.Fatalf("drawProgress(previous error) = %v", err)
	}
}
