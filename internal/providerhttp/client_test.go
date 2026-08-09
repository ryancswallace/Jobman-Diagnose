package providerhttp

import (
	"net/http"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestClientUsesConfiguredResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		given    time.Duration
		expected time.Duration
	}{
		{name: "profile deadline", given: 2 * time.Minute, expected: 2 * time.Minute},
		{name: "legacy default", expected: 30 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := Client(provider.LocalityLocal, test.given)
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport type = %T", client.Transport)
			}
			if transport.ResponseHeaderTimeout != test.expected {
				t.Fatalf("response header timeout = %s, want %s", transport.ResponseHeaderTimeout, test.expected)
			}
		})
	}
}
