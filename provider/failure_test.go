package provider

import (
	"errors"
	"strings"
	"testing"
)

func TestFailureSeparatesSafeDiagnosticFromCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("secret-looking provider detail")
	err := NewFailure(FailureResponseIncomplete, cause)
	code, message, ok := Diagnostic(err)
	if !ok || code != FailureResponseIncomplete || message != "the provider response did not finish normally" {
		t.Fatalf("diagnostic = %q / %q / %t", code, message, ok)
	}
	if strings.Contains(message, "secret-looking") {
		t.Fatalf("safe message exposed cause: %q", message)
	}
	if !errors.Is(err, cause) {
		t.Fatal("classified failure did not preserve its cause")
	}
	if _, _, ok := Diagnostic(cause); ok {
		t.Fatal("generic error unexpectedly produced a safe diagnostic")
	}
}
