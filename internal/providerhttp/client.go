// Package providerhttp contains the shared fail-closed HTTP transport policy
// for structured generator adapters.
package providerhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

// Endpoint validates one exact, non-discoverable provider URL.
//
//nolint:cyclop // URL and declared-locality invariants must be evaluated as one anti-downgrade policy.
func Endpoint(raw string, locality provider.Locality) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Path == "" || parsed.Path == "/" {
		return nil, errors.New("provider endpoint must be an exact absolute URL without credentials, query, or fragment")
	}
	loopback := loopbackHost(parsed.Hostname())
	switch locality {
	case provider.LocalityLocal:
		if parsed.Scheme != "http" && parsed.Scheme != "https" || !loopback {
			return nil, errors.New("local provider endpoint must use HTTP(S) on a loopback host")
		}
	case provider.LocalityRemote:
		if parsed.Scheme != "https" || loopback {
			return nil, errors.New("remote provider endpoint must use HTTPS and not identify a loopback host")
		}
	default:
		return nil, errors.New("provider locality must be local or remote")
	}

	return parsed, nil
}

// Client returns a proxy-free client that never follows redirects. Local
// profiles also verify every resolved dial address remains loopback. The
// response-header deadline should match the enclosing generation deadline;
// callers that omit it retain the conservative legacy default.
func Client(locality provider.Locality, responseHeaderTimeout time.Duration) *http.Client {
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = 30 * time.Second
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	if locality == provider.LocalityLocal {
		transport.DialContext = loopbackDialContext
	}

	return &http.Client{
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ReadJSON consumes one successful bounded JSON response. Error response
// bodies are deliberately not returned because they may echo evidence or credentials.
func ReadJSON(response *http.Response, maximumBytes int64) ([]byte, error) {
	if response == nil || response.Body == nil || maximumBytes < 1 {
		return nil, provider.NewFailure(
			provider.FailureResponseInvalid, errors.New("read provider response: invalid response or limit"),
		)
	}
	defer response.Body.Close() //nolint:errcheck // The bounded read result determines transport success.
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, provider.NewFailure(
			provider.FailureHTTPStatus, fmt.Errorf("provider returned HTTP status %d", response.StatusCode),
		)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, provider.NewFailure(
			provider.FailureResponseContentType, errors.New("provider returned a non-JSON content type"),
		)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maximumBytes+1))
	if err != nil {
		return nil, provider.NewFailure(provider.FailureResponseRead, fmt.Errorf("read provider response: %w", err))
	}
	if int64(len(encoded)) > maximumBytes {
		return nil, provider.NewFailure(
			provider.FailureResponseOversized, fmt.Errorf("provider response exceeds %d bytes", maximumBytes),
		)
	}

	return encoded, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)

	return address != nil && address.IsLoopback()
}

func loopbackDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("dial local provider: %w", err)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve local provider: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("resolve local provider: no addresses")
	}
	for _, candidate := range addresses {
		if !candidate.IP.IsLoopback() {
			return nil, errors.New("resolve local provider: non-loopback address rejected")
		}
	}
	dialer := &net.Dialer{}
	var lastErr error
	for _, candidate := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}

	return nil, fmt.Errorf("dial local provider: %w", lastErr)
}
