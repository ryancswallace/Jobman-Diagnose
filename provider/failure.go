package provider

import (
	"errors"
	"fmt"
)

// FailureCode is a stable, nonsecret classification for a generator-stage
// failure. It is safe to expose in CLI diagnostics and fallback warnings.
type FailureCode string

// Stable generator failure classifications.
const (
	FailureInvalidRequest      FailureCode = "invalid_request"
	FailureInputOversized      FailureCode = "input_oversized"
	FailureRequestTimeout      FailureCode = "request_timeout"
	FailureRequestCanceled     FailureCode = "request_canceled"
	FailureRequestFailed       FailureCode = "request_failed"
	FailureHTTPStatus          FailureCode = "http_status"
	FailureResponseContentType FailureCode = "invalid_response_content_type"
	FailureResponseRead        FailureCode = "response_read_failed"
	FailureResponseOversized   FailureCode = "response_oversized"
	FailureResponseInvalid     FailureCode = "invalid_response"
	FailureResponseIncomplete  FailureCode = "incomplete_response"
	FailureResponseTruncated   FailureCode = "response_truncated"
	FailureModelRefused        FailureCode = "model_refused"
	FailureContentEmpty        FailureCode = "structured_content_empty"
	FailureContentOversized    FailureCode = "structured_content_oversized"
	FailureProviderExit        FailureCode = "provider_exit"
	FailureOutputEmpty         FailureCode = "provider_output_empty"
)

// FailureError carries a deliberately bounded diagnostic separately from its
// potentially sensitive implementation cause.
type FailureError struct {
	code  FailureCode
	cause error
}

// NewFailure constructs a classified provider failure. User-facing text is
// selected from a closed catalog so an untrusted cause can never enter it.
func NewFailure(code FailureCode, cause error) error {
	return &FailureError{code: code, cause: cause}
}

func (failure *FailureError) Error() string {
	message, ok := failureMessage(failure.code)
	if !ok {
		message = "the provider failed without a recognized classification"
	}
	if failure.cause != nil {
		return fmt.Sprintf("%s: %v", message, failure.cause)
	}

	return message
}

// Unwrap preserves cancellation and low-level error inspection inside trusted
// code without making the cause part of the safe diagnostic.
func (failure *FailureError) Unwrap() error { return failure.cause }

// Diagnostic returns the stable code and safe message from a classified
// provider failure. Generic errors deliberately produce no diagnostic.
func Diagnostic(err error) (FailureCode, string, bool) {
	var failure *FailureError
	if !errors.As(err, &failure) || failure == nil {
		return "", "", false
	}
	message, ok := failureMessage(failure.code)
	if !ok {
		return "", "", false
	}

	return failure.code, message, true
}

//nolint:cyclop // The closed code-to-message catalog is intentionally centralized for auditability.
func failureMessage(code FailureCode) (string, bool) {
	switch code {
	case FailureInvalidRequest:
		return "the bounded generation request was invalid", true
	case FailureInputOversized:
		return "the generation request exceeded the configured input limit", true
	case FailureRequestTimeout:
		return "the provider request exceeded the configured timeout", true
	case FailureRequestCanceled:
		return "the provider request was canceled", true
	case FailureRequestFailed:
		return "the provider request failed before a valid response was received", true
	case FailureHTTPStatus:
		return "the provider returned a non-success HTTP status", true
	case FailureResponseContentType:
		return "the provider response did not have a JSON content type", true
	case FailureResponseRead:
		return "the provider response could not be read", true
	case FailureResponseOversized:
		return "the provider response exceeded the configured output envelope limit", true
	case FailureResponseInvalid:
		return "the provider returned an invalid JSON response envelope", true
	case FailureResponseIncomplete:
		return "the provider response did not finish normally", true
	case FailureResponseTruncated:
		return "the provider stopped because the generation or context limit was reached", true
	case FailureModelRefused:
		return "the model refused the diagnosis request", true
	case FailureContentEmpty:
		return "the provider returned empty structured content", true
	case FailureContentOversized:
		return "the structured content exceeded the configured output limit", true
	case FailureProviderExit:
		return "the local provider process exited unsuccessfully", true
	case FailureOutputEmpty:
		return "the local provider process returned no structured content", true
	default:
		return "", false
	}
}
