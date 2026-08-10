package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestNewRejectsInvalidCompatibleConfiguration(t *testing.T) {
	t.Parallel()

	base := Config{
		Endpoint: "http://127.0.0.1:8000/v1/chat", Model: "model", Locality: provider.LocalityLocal,
		MaximumInputBytes: 4096, MaximumOutputBytes: 1024,
	}
	mutations := []func(*Config){
		func(value *Config) { value.Model = "" },
		func(value *Config) { value.Model = "bad\nmodel" },
		func(value *Config) { value.MaximumInputBytes = 0 },
		func(value *Config) { value.MaximumOutputBytes = 0 },
		func(value *Config) { value.Credential = []byte("bad\ncredential") },
	}
	for index, mutate := range mutations {
		configuration := base
		mutate(&configuration)
		if _, err := newWithClient(configuration, &http.Client{}); err == nil {
			t.Fatalf("configuration variant %d error = nil", index)
		}
	}
	if _, err := newWithClient(base, nil); err == nil {
		t.Fatal("newWithClient(nil) error = nil")
	}
}

func TestGenerateClassifiesCompatibleFailures(t *testing.T) {
	t.Parallel()

	request := openAIRequest(t)
	generator := newTestCompatible(t)
	if _, err := generator.Generate(nil, request); compatibleFailureCode(err) != provider.FailureInvalidRequest { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatalf("nil context error = %v", err)
	}
	invalid := request
	invalid.RequestID = "bad"
	if _, err := generator.Generate(t.Context(), invalid); compatibleFailureCode(err) != provider.FailureInvalidRequest {
		t.Fatalf("invalid request error = %v", err)
	}
	generator.config.MaximumInputBytes = 1
	if _, err := generator.Generate(t.Context(), request); compatibleFailureCode(err) != provider.FailureInputOversized {
		t.Fatalf("oversized request error = %v", err)
	}

	tests := []struct {
		name string
		body string
		code provider.FailureCode
	}{
		{name: "invalid envelope", body: `{`, code: provider.FailureResponseInvalid},
		{name: "no choices", body: `{"choices":[]}`, code: provider.FailureResponseIncomplete},
		{name: "ambiguous choices", body: `{"choices":[{},{}]}`, code: provider.FailureResponseIncomplete},
		{name: "truncated", body: `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`, code: provider.FailureResponseTruncated},
		{name: "unfinished", body: `{"choices":[{"finish_reason":"tool_calls","message":{"content":"{}"}}]}`, code: provider.FailureResponseIncomplete},
		{name: "refusal", body: `{"choices":[{"finish_reason":"stop","message":{"refusal":"no","content":"{}"}}]}`, code: provider.FailureModelRefused},
		{name: "empty", body: `{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`, code: provider.FailureContentEmpty},
		{name: "oversized", body: `{"choices":[{"finish_reason":"stop","message":{"content":"12345"}}]}`, code: provider.FailureContentOversized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := newTestCompatible(t)
			current.config.MaximumOutputBytes = 4
			current.client = compatibleResponseClient(test.body)
			_, err := current.Generate(t.Context(), request)
			if compatibleFailureCode(err) != test.code {
				t.Fatalf("Generate() error/code = %v / %q", err, compatibleFailureCode(err))
			}
		})
	}

	generator = newTestCompatible(t)
	generator.client = &http.Client{Transport: compatibleRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})}
	if _, err := generator.Generate(t.Context(), request); compatibleFailureCode(err) != provider.FailureRequestFailed {
		t.Fatalf("transport error = %v", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := generator.Generate(canceled, request); compatibleFailureCode(err) != provider.FailureRequestCanceled {
		t.Fatalf("canceled error = %v", err)
	}
	deadline, cancelDeadline := context.WithTimeout(t.Context(), -1)
	defer cancelDeadline()
	if _, err := generator.Generate(deadline, request); compatibleFailureCode(err) != provider.FailureRequestTimeout {
		t.Fatalf("deadline error = %v", err)
	}

	generator = newTestCompatible(t)
	generator.client = &http.Client{Transport: compatibleRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`)),
		}, nil
	})}
	if _, err := generator.Generate(t.Context(), request); err == nil || !strings.Contains(err.Error(), "non-success") {
		t.Fatalf("HTTP status error = %v", err)
	}
}

func TestGenerateDropsUnsafeProviderRequestID(t *testing.T) {
	t.Parallel()

	for _, providerRequestID := range []string{"unsafe\nidentifier", strings.Repeat("x", 257)} {
		generator := newTestCompatible(t)
		generator.client = compatibleResponseClient(`{
			"id":` + string(mustJSON(t, providerRequestID)) + `,
			"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]
		}`)
		response, err := generator.Generate(t.Context(), openAIRequest(t))
		if err != nil {
			t.Fatal(err)
		}
		if response.ProviderRequestID != "" {
			t.Fatalf("unsafe provider request ID retained: %q", response.ProviderRequestID)
		}
	}
}

func mustJSON(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func newTestCompatible(t *testing.T) *Generator {
	t.Helper()
	generator, err := New(Config{
		Endpoint: "http://127.0.0.1:8000/v1/chat", Model: "model", Locality: provider.LocalityLocal,
		MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return generator
}

func compatibleResponseClient(body string) *http.Client {
	return &http.Client{Transport: compatibleRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func compatibleFailureCode(err error) provider.FailureCode {
	code, _, _ := provider.Diagnostic(err)
	return code
}

type compatibleRoundTripFunc func(*http.Request) (*http.Response, error)

func (function compatibleRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
