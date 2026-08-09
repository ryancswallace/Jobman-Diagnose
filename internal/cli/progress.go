package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/ryancswallace/jobman-diagnose/internal/generation"
)

const (
	progressDelay    = 300 * time.Millisecond
	progressInterval = 100 * time.Millisecond
)

type progressMode string

const (
	progressAuto  progressMode = "auto"
	progressPlain progressMode = "plain"
	progressOff   progressMode = "off"
)

type progressPhase string

const (
	progressCollecting progressPhase = "collecting"
	progressPreparing  progressPhase = "preparing"
	progressWaiting    progressPhase = "waiting"
	progressValidating progressPhase = "validating"
	progressFallback   progressPhase = "fallback"
)

type progressDescriptor struct {
	profile string
	model   string
	timeout time.Duration
}

type progressReporter interface {
	Phase(progressPhase)
	Close() error
}

type progressTiming struct {
	now   func() time.Time
	delay <-chan time.Time
	ticks <-chan time.Time
	stop  func()
}

type runtimeEnvironment struct {
	interactive       func(io.Writer) bool
	newProgressTiming func() progressTiming
}

func defaultRuntimeEnvironment() runtimeEnvironment {
	return runtimeEnvironment{
		interactive: terminalWriter,
		newProgressTiming: func() progressTiming {
			timer := time.NewTimer(progressDelay)
			ticker := time.NewTicker(progressInterval)
			return progressTiming{
				now: time.Now, delay: timer.C, ticks: ticker.C,
				stop: func() {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					ticker.Stop()
				},
			}
		},
	}
}

func terminalWriter(destination io.Writer) bool {
	file, ok := destination.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func newProgressReporter(
	mode progressMode,
	enabled bool,
	jsonOutput bool,
	interactive bool,
	destination io.Writer,
	descriptor progressDescriptor,
	timingFactory func() progressTiming,
) progressReporter {
	if !enabled || mode == progressOff {
		return noProgress{}
	}
	if mode == progressPlain {
		return &plainProgress{destination: destination, descriptor: descriptor}
	}
	if mode != progressAuto || jsonOutput || !interactive {
		return noProgress{}
	}

	return newSpinnerProgress(destination, descriptor, timingFactory())
}

func generationProgressObserver(reporter progressReporter) generation.ProgressObserver {
	return func(event generation.ProgressEvent) {
		switch event {
		case generation.ProgressPreparing:
			reporter.Phase(progressPreparing)
		case generation.ProgressWaiting:
			reporter.Phase(progressWaiting)
		case generation.ProgressValidating:
			reporter.Phase(progressValidating)
		case generation.ProgressFallback:
			reporter.Phase(progressFallback)
		}
	}
}

type noProgress struct{}

func (noProgress) Phase(progressPhase) {}
func (noProgress) Close() error        { return nil }

type plainProgress struct {
	destination io.Writer
	descriptor  progressDescriptor
	last        progressPhase
	err         error
}

func (progress *plainProgress) Phase(phase progressPhase) {
	if progress.err != nil || phase == progress.last {
		return
	}
	progress.last = phase
	_, progress.err = fmt.Fprintf(
		progress.destination, "AI analysis: %s\n", progressPhaseText(phase, progress.descriptor),
	)
}

func (progress *plainProgress) Close() error { return progress.err }

type spinnerProgress struct {
	events   chan spinnerEvent
	finished chan error
}

type spinnerEvent struct {
	phase progressPhase
	close bool
	done  chan struct{}
}

func newSpinnerProgress(
	destination io.Writer,
	descriptor progressDescriptor,
	timing progressTiming,
) *spinnerProgress {
	progress := &spinnerProgress{
		events: make(chan spinnerEvent), finished: make(chan error, 1),
	}
	go progress.run(destination, descriptor, timing)

	return progress
}

func (progress *spinnerProgress) Phase(phase progressPhase) {
	done := make(chan struct{})
	progress.events <- spinnerEvent{phase: phase, done: done}
	<-done
}

func (progress *spinnerProgress) Close() error {
	progress.events <- spinnerEvent{close: true}

	return <-progress.finished
}

func (progress *spinnerProgress) run(
	destination io.Writer,
	descriptor progressDescriptor,
	timing progressTiming,
) {
	phase := progressCollecting
	phaseStarted := timing.now()
	visible := false
	frame := 0
	var writeErr error
	for {
		select {
		case <-timing.delay:
			visible = true
			writeErr = drawProgress(destination, descriptor, phase, phaseStarted, timing.now(), frame, writeErr)
			frame++
		case <-timing.ticks:
			if visible {
				writeErr = drawProgress(destination, descriptor, phase, phaseStarted, timing.now(), frame, writeErr)
				frame++
			}
		case event := <-progress.events:
			if event.close {
				if visible && writeErr == nil {
					_, writeErr = io.WriteString(destination, "\r\x1b[2K")
				}
				timing.stop()
				progress.finished <- writeErr
				return
			}
			if event.phase != phase {
				phase = event.phase
				phaseStarted = timing.now()
			}
			if visible {
				writeErr = drawProgress(destination, descriptor, phase, phaseStarted, timing.now(), frame, writeErr)
				frame++
			}
			close(event.done)
		}
	}
}

func drawProgress(
	destination io.Writer,
	descriptor progressDescriptor,
	phase progressPhase,
	started time.Time,
	now time.Time,
	frame int,
	previousErr error,
) error {
	if previousErr != nil {
		return previousErr
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	line := fmt.Sprintf(
		"%s AI analysis: %s… %s",
		frames[frame%len(frames)], progressPhaseText(phase, descriptor), formatProgressElapsed(now.Sub(started)),
	)
	if phase == progressWaiting && descriptor.timeout > 0 {
		line += " (timeout " + formatProgressDuration(descriptor.timeout) + "; Ctrl-C to cancel)"
	} else {
		line += " (Ctrl-C to cancel)"
	}
	_, err := fmt.Fprintf(destination, "\r\x1b[2K%s", line)

	return err
}

func progressPhaseText(phase progressPhase, descriptor progressDescriptor) string {
	switch phase {
	case progressCollecting:
		return "collecting Jobman evidence"
	case progressPreparing:
		return "preparing bounded, redacted evidence"
	case progressWaiting:
		profile := safeProgressLabel(descriptor.profile)
		model := safeProgressLabel(descriptor.model)
		if model == "" {
			return "waiting for profile " + profile
		}
		return "waiting for profile " + profile + " (model " + model + ")"
	case progressValidating:
		return "validating the model response"
	case progressFallback:
		return "using the deterministic fallback"
	default:
		return "working"
	}
}

func safeProgressLabel(value string) string {
	value = strings.TrimSpace(value)
	runes := make([]rune, 0, min(len(value), 64))
	for _, character := range value {
		if unicode.IsControl(character) {
			character = '�'
		}
		runes = append(runes, character)
		if len(runes) == 64 {
			break
		}
	}

	return string(runes)
}

func formatProgressElapsed(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	return duration.Truncate(time.Second).String()
}

func formatProgressDuration(duration time.Duration) string {
	value := duration.Round(time.Second).String()
	if strings.HasSuffix(value, "m0s") {
		value = strings.TrimSuffix(value, "0s")
	}

	return value
}

type progressModeValue progressMode

func (value *progressModeValue) Set(encoded string) error {
	mode := progressMode(strings.ToLower(strings.TrimSpace(encoded)))
	switch mode {
	case progressAuto, progressPlain, progressOff:
		*value = progressModeValue(mode)
		return nil
	default:
		return fmt.Errorf("must be auto, plain, or off")
	}
}

func (value *progressModeValue) String() string {
	if value == nil {
		return ""
	}

	return string(*value)
}
