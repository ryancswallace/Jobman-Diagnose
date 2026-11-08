package provider

// TransportOptions selects explicit HTTPS trust and egress without carrying
// secret values. Empty options preserve system trust and proxy-free transport.
type TransportOptions struct {
	CAFile                string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
	ClientCertificateFile string `json:"client_certificate_file,omitempty" yaml:"client_certificate_file,omitempty"`
	ClientKeyFile         string `json:"client_key_file,omitempty" yaml:"client_key_file,omitempty"`
	ProxyURL              string `json:"proxy_url,omitempty" yaml:"proxy_url,omitempty"`
}
