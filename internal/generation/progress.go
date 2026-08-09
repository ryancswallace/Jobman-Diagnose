package generation

// ProgressEvent identifies a user-visible phase of optional generated analysis.
// The generation package emits events but never renders or logs them.
type ProgressEvent string

const (
	// ProgressPreparing indicates that the bounded provider projection is being prepared.
	ProgressPreparing ProgressEvent = "preparing"
	// ProgressWaiting indicates that a configured provider request is in flight.
	ProgressWaiting ProgressEvent = "waiting"
	// ProgressValidating indicates that a provider response is being validated.
	ProgressValidating ProgressEvent = "validating"
	// ProgressFallback indicates that optional generation is falling back to the deterministic report.
	ProgressFallback ProgressEvent = "fallback"
)

// ProgressObserver receives best-effort lifecycle events synchronously. A nil
// observer disables progress reporting without changing diagnosis behavior.
type ProgressObserver func(ProgressEvent)
