package providerhttp

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestEndpointPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		locality provider.Locality
		valid    bool
	}{
		{"http://127.0.0.1:11434/api/chat", provider.LocalityLocal, true},
		{"https://localhost/api", provider.LocalityLocal, true},
		{"https://example.com/v1/chat", provider.LocalityRemote, true},
		{"relative", provider.LocalityLocal, false},
		{"http://user@example.com/api", provider.LocalityRemote, false},
		{"http://127.0.0.1/api?query=1", provider.LocalityLocal, false},
		{"http://example.com/api", provider.LocalityLocal, false},
		{"http://example.com/api", provider.LocalityRemote, false},
		{"https://127.0.0.1/api", provider.LocalityRemote, false},
		{"https://example.com/api", "unknown", false},
	}
	for _, test := range tests {
		_, err := Endpoint(test.raw, test.locality)
		if (err == nil) != test.valid {
			t.Fatalf("Endpoint(%q, %q) error = %v", test.raw, test.locality, err)
		}
	}
	if !loopbackHost("LOCALHOST") || !loopbackHost("::1") || loopbackHost("example.com") {
		t.Fatal("loopbackHost() classification changed")
	}
}

//nolint:bodyclose // ReadJSON owns and closes every non-nil response body passed to it.
func TestReadJSONClassifiesResponseFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response *http.Response
		limit    int64
		code     provider.FailureCode
	}{
		{name: "nil", limit: 1, code: provider.FailureResponseInvalid},
		{name: "zero limit", response: response(http.StatusOK, "application/json", "{}"), code: provider.FailureResponseInvalid},
		{name: "status", response: response(http.StatusBadGateway, "application/json", "secret"), limit: 10, code: provider.FailureHTTPStatus},
		{name: "content type", response: response(http.StatusOK, "text/plain", "{}"), limit: 10, code: provider.FailureResponseContentType},
		{name: "malformed content type", response: response(http.StatusOK, `bad;="`, "{}"), limit: 10, code: provider.FailureResponseContentType},
		{name: "read", response: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: errorReadCloser{}}, limit: 10, code: provider.FailureResponseRead},
		{name: "oversized", response: response(http.StatusOK, "application/json", "12345"), limit: 4, code: provider.FailureResponseOversized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ReadJSON(test.response, test.limit)
			code, _, ok := provider.Diagnostic(err)
			if !ok || code != test.code {
				t.Fatalf("ReadJSON() error/code = %v / %q", err, code)
			}
		})
	}
	valid := response(http.StatusOK, "application/json; charset=utf-8", "{}")
	if encoded, err := ReadJSON(valid, 10); err != nil || string(encoded) != "{}" {
		t.Fatalf("ReadJSON(valid) = %q, %v", encoded, err)
	}
}

func TestClientRejectsRedirectsAndUsesProxyFreeTransport(t *testing.T) {
	t.Parallel()

	client := Client(provider.LocalityRemote, time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("remote transport type = %T", client.Transport)
	}
	if transport.Proxy != nil || transport.DialContext == nil {
		t.Fatalf("remote transport = %#v", transport)
	}
	if err := client.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() error = %v", err)
	}
	localClient := Client(provider.LocalityLocal, time.Second)
	local, ok := localClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("local transport type = %T", localClient.Transport)
	}
	if local.DialContext == nil {
		t.Fatal("local client has no guarded dialer")
	}
	if _, err := loopbackDialContext(t.Context(), "tcp", "invalid-address"); err == nil {
		t.Fatal("loopbackDialContext(invalid address) error = nil")
	}
}

func response(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errorReadCloser) Close() error             { return nil }
