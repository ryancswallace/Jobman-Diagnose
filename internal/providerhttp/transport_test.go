package providerhttp

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestTransportValidationPreservesLocalityAndSecretBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		endpoint string
		locality provider.Locality
		options  provider.TransportOptions
	}{
		{"https://localhost/generate", provider.LocalityLocal, provider.TransportOptions{ProxyURL: "http://proxy.example.invalid"}},
		{"http://localhost/generate", provider.LocalityLocal, provider.TransportOptions{CAFile: "/ca.pem"}},
		{"https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{ClientKeyFile: "/key.pem"}},
		{"https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{CAFile: "relative.pem"}},
		// #nosec G101 -- synthetic canary proves credentials in proxy URLs are rejected.
		{"https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{ProxyURL: "http://user:canary@proxy.example.invalid"}},
		{"https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{ProxyURL: "http://proxy.example.invalid?"}},
		{"https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{
			ProxyURL: "https://proxy.example.invalid", ClientCertificateFile: "/client.pem", ClientKeyFile: "/key.pem",
		}},
	} {
		err := ValidateTransport(test.endpoint, test.locality, test.options)
		if err == nil || strings.Contains(err.Error(), "canary") {
			t.Fatalf("expected safe transport rejection, got %v", err)
		}
	}
	options := provider.TransportOptions{ProxyURL: "https://proxy.example.invalid:8443"}
	client, err := ClientWithOptions("https://model.example.invalid/generate", provider.LocalityRemote, time.Second, options)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("explicit verified proxy transport was not installed")
	}
	if redirectErr := client.CheckRedirect(nil, nil); !errors.Is(redirectErr, http.ErrUseLastResponse) {
		t.Fatal("redirect rejection changed")
	}
	missing := filepath.Join(t.TempDir(), "not-present")
	if err := ValidateTransport("https://model.example.invalid/generate", provider.LocalityRemote, provider.TransportOptions{
		ProxyURL: "http://proxy.example.invalid", ClientCertificateFile: filepath.Join(missing, "client.pem"), ClientKeyFile: filepath.Join(missing, "key.pem"),
	}); err != nil {
		t.Fatalf("HTTP CONNECT with provider mTLS rejected: %v", err)
	}
}

func TestTransportUsesConfiguredCAAndClientCertificate(t *testing.T) {
	t.Parallel()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			t.Error("client certificate not presented")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	t.Cleanup(server.Close)
	directory := t.TempDir()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certificatePath, keyPath := filepath.Join(directory, "cert.pem"), filepath.Join(directory, "key.pem")
	if writeErr := os.WriteFile(certificatePath, certificate, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(keyPath, key, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	options := provider.TransportOptions{CAFile: certificatePath, ClientCertificateFile: certificatePath, ClientKeyFile: keyPath}
	client, err := ClientWithOptions(server.URL+"/generate", provider.LocalityLocal, time.Second, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/generate", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
	if runtime.GOOS != "windows" {
		// #nosec G302 -- intentionally unsafe permissions verify private-key rejection.
		if err := os.Chmod(keyPath, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ClientWithOptions(server.URL+"/generate", provider.LocalityLocal, time.Second, options); err == nil {
			t.Fatal("public client key accepted")
		}
	}
}

func TestTransportDoesNotTrustUnknownCA(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := ClientWithOptions(server.URL+"/generate", provider.LocalityLocal, time.Second, provider.TransportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/generate", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := client.Do(request); err == nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Fatal("untrusted server certificate accepted")
	}
}

func TestTransportFilesFailClosedWithoutDisclosingContents(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	invalid := filepath.Join(directory, "invalid.pem")
	oversized := filepath.Join(directory, "oversized.pem")
	link := filepath.Join(directory, "link.pem")
	if err := os.WriteFile(invalid, []byte("secret-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", 1024*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{invalid, oversized, directory, filepath.Join(directory, "missing.pem")}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(invalid, link); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, link)
	}
	for _, path := range paths {
		_, err := ClientWithOptions("https://model.example.invalid/generate", provider.LocalityRemote, time.Second, provider.TransportOptions{CAFile: path})
		if err == nil || strings.Contains(err.Error(), "secret-canary") {
			t.Fatalf("expected bounded safe file rejection, got %v", err)
		}
	}
	_, err := ClientWithOptions("https://model.example.invalid/generate", provider.LocalityRemote, time.Second, provider.TransportOptions{
		ClientCertificateFile: invalid, ClientKeyFile: invalid,
	})
	if err == nil || strings.Contains(err.Error(), "secret-canary") {
		t.Fatalf("expected safe key-pair rejection, got %v", err)
	}
}
