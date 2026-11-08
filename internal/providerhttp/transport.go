package providerhttp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

// ValidateTransport checks references without opening files or contacting a provider.
func ValidateTransport(endpoint string, locality provider.Locality, options provider.TransportOptions) error {
	parsed, err := Endpoint(endpoint, locality)
	if err != nil {
		return err
	}
	if options == (provider.TransportOptions{}) {
		return nil
	}
	if parsed.Scheme != "https" {
		return errors.New("configured HTTP transport requires an HTTPS endpoint")
	}
	for _, path := range []string{options.CAFile, options.ClientCertificateFile, options.ClientKeyFile} {
		if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
			return errors.New("transport file references must be clean absolute paths")
		}
	}
	if (options.ClientCertificateFile == "") != (options.ClientKeyFile == "") {
		return errors.New("transport client certificate and key references must be provided together")
	}
	if options.ProxyURL != "" {
		return validateProxy(options.ProxyURL, locality, options.ClientCertificateFile != "")
	}
	return nil
}

func validateProxy(raw string, locality provider.Locality, clientCertificate bool) error {
	if locality == provider.LocalityLocal {
		return errors.New("local profiles cannot use a proxy")
	}
	proxy, err := url.Parse(raw)
	if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" ||
		proxy.User != nil || proxy.Path != "" || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Fragment != "" || proxy.Opaque != "" {
		return errors.New("proxy must be an explicit HTTP(S) origin without credentials, path, query, or fragment")
	}
	if proxy.Scheme == "https" && clientCertificate {
		return errors.New("client certificates cannot be combined with an HTTPS proxy")
	}
	return nil
}

// ClientWithOptions creates a bounded explicit transport. References are resolved
// only when generation is requested, never during configuration validation.
func ClientWithOptions(endpoint string, locality provider.Locality, timeout time.Duration, options provider.TransportOptions) (*http.Client, error) {
	if err := ValidateTransport(endpoint, locality, options); err != nil {
		return nil, err
	}
	client := Client(locality, timeout)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		return nil, errors.New("provider transport is unavailable")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.CAFile != "" {
		encoded, err := readTransportFile(options.CAFile, false)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM(encoded) {
			return nil, errors.New("transport CA file contains no certificates")
		}
	}
	if options.ClientCertificateFile != "" {
		certificate, err := readTransportFile(options.ClientCertificateFile, false)
		if err != nil {
			return nil, err
		}
		key, err := readTransportFile(options.ClientKeyFile, true)
		if err != nil {
			return nil, err
		}
		defer clear(key)
		pair, err := tls.X509KeyPair(certificate, key)
		if err != nil {
			return nil, errors.New("transport client certificate or key is invalid")
		}
		tlsConfig.Certificates = []tls.Certificate{pair}
	}
	transport.TLSClientConfig = tlsConfig
	if options.ProxyURL != "" {
		proxy, err := url.Parse(options.ProxyURL)
		if err != nil {
			return nil, errors.New("transport proxy is invalid")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return client, nil
}

func readTransportFile(path string, private bool) ([]byte, error) {
	const limit = 1024 * 1024
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("transport file must be readable, regular, and bounded")
	}
	mask := os.FileMode(0o022)
	if private {
		mask = 0o077
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&mask != 0 {
		return nil, errors.New("transport file permissions are unsafe")
	}
	// #nosec G304 -- only validated explicit transport references reach this boundary.
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open transport file")
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if errors.Join(readErr, closeErr) != nil || len(encoded) > limit {
		return nil, errors.New("cannot read bounded transport file")
	}
	return encoded, nil
}
