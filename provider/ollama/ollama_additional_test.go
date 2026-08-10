package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestNewRejectsInvalidOllamaConfiguration(t *testing.T) {
	t.Parallel()

	base := Config{Endpoint: "http://127.0.0.1:11434/api/chat", Model: "model", MaximumInputBytes: 4096, MaximumOutputBytes: 1024}
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
		if _, err := New(configuration); err == nil {
			t.Fatalf("configuration variant %d error = nil", index)
		}
	}
}

func TestOllamaDescribesConfiguredCapabilities(t *testing.T) {
	t.Parallel()

	generator := newTestOllama(t)
	capabilities := generator.Capabilities()
	if generator.Name() != "ollama" || !capabilities.NativeJSONSchema ||
		capabilities.Locality != provider.LocalityLocal || capabilities.MaximumInputBytes != 64*1024 ||
		capabilities.MaximumOutputBytes != 16*1024 {
		t.Fatalf("description = %q / %#v", generator.Name(), capabilities)
	}
}

func TestGenerateClassifiesOllamaFailures(t *testing.T) {
	t.Parallel()

	request := ollamaRequest(t)
	generator := newTestOllama(t)
	if _, err := generator.Generate(nil, request); failureCode(err) != provider.FailureInvalidRequest { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatalf("nil context error = %v", err)
	}
	invalid := request
	invalid.RequestID = "bad"
	if _, err := generator.Generate(t.Context(), invalid); failureCode(err) != provider.FailureInvalidRequest {
		t.Fatalf("invalid request error = %v", err)
	}
	generator.config.MaximumInputBytes = 1
	if _, err := generator.Generate(t.Context(), request); failureCode(err) != provider.FailureInputOversized {
		t.Fatalf("oversized request error = %v", err)
	}

	tests := []struct {
		name string
		body string
		code provider.FailureCode
	}{
		{name: "invalid envelope", body: `{`, code: provider.FailureResponseInvalid},
		{name: "incomplete", body: `{"done":false,"message":{"content":"{}"}}`, code: provider.FailureResponseIncomplete},
		{name: "empty", body: `{"done":true,"message":{"content":""}}`, code: provider.FailureContentEmpty},
		{name: "oversized", body: `{"done":true,"message":{"content":"12345"}}`, code: provider.FailureContentOversized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := newTestOllama(t)
			current.config.MaximumOutputBytes = 4
			current.client = responseClient(test.body)
			_, err := current.Generate(t.Context(), request)
			if failureCode(err) != test.code {
				t.Fatalf("Generate() error/code = %v / %q", err, failureCode(err))
			}
		})
	}

	generator = newTestOllama(t)
	generator.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})}
	if _, err := generator.Generate(t.Context(), request); failureCode(err) != provider.FailureRequestFailed {
		t.Fatalf("transport error = %v", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := generator.Generate(canceled, request); failureCode(err) != provider.FailureRequestCanceled {
		t.Fatalf("canceled error = %v", err)
	}
	deadline, cancelDeadline := context.WithTimeout(t.Context(), -1)
	defer cancelDeadline()
	if _, err := generator.Generate(deadline, request); failureCode(err) != provider.FailureRequestTimeout {
		t.Fatalf("deadline error = %v", err)
	}

	generator = newTestOllama(t)
	generator.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`)),
		}, nil
	})}
	if _, err := generator.Generate(t.Context(), request); err == nil || !strings.Contains(err.Error(), "non-success") {
		t.Fatalf("HTTP status error = %v", err)
	}
}

func TestGenerateSendsOllamaCredential(t *testing.T) {
	t.Parallel()

	generator := newTestOllama(t)
	generator.config.Credential = []byte("test-token")
	generator.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"done":true,"message":{"content":"{}"}}`)),
		}, nil
	})}
	if _, err := generator.Generate(t.Context(), ollamaRequest(t)); err != nil {
		t.Fatal(err)
	}
}

func newTestOllama(t *testing.T) *Generator {
	t.Helper()
	generator, err := New(Config{
		Endpoint: "http://127.0.0.1:11434/api/chat", Model: "model",
		MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return generator
}

func responseClient(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func failureCode(err error) provider.FailureCode {
	code, _, _ := provider.Diagnostic(err)
	return code
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
