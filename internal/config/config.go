// Package config loads strict, versioned generated-diagnosis profiles.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

const (
	// SchemaVersion is the current diagnosis.yml schema.
	SchemaVersion = 2
	// EnvironmentPath names the optional diagnosis-configuration path override.
	EnvironmentPath    = "JOBMAN_DIAGNOSE_CONFIG"
	maximumConfigBytes = 1024 * 1024
	maximumConfigDepth = 32
	maximumProfiles    = 32
)

// File is one diagnosis.yml document.
type File struct {
	SchemaVersion int                `json:"schema_version" yaml:"schema_version"`
	Defaults      Defaults           `json:"defaults" yaml:"defaults"`
	Profiles      map[string]Profile `json:"profiles" yaml:"profiles"`
}

// Defaults contains explicit per-user invocation defaults. It never enables
// generated analysis by itself; --ai or --profile remains required.
type Defaults struct {
	Profile string `json:"profile" yaml:"profile"`
}

// Profile selects one explicit generator transport and disclosure policy.
type Profile struct {
	Provider           string                 `json:"provider" yaml:"provider"`
	Locality           provider.Locality      `json:"locality" yaml:"locality"`
	Endpoint           string                 `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Command            *Command               `json:"command,omitempty" yaml:"command,omitempty"`
	Model              string                 `json:"model" yaml:"model"`
	RequireJSONSchema  bool                   `json:"require_json_schema" yaml:"require_json_schema"`
	Timeout            string                 `json:"timeout" yaml:"timeout"`
	MaximumInputBytes  int                    `json:"maximum_input_bytes" yaml:"maximum_input_bytes"`
	MaximumOutputBytes int                    `json:"maximum_output_bytes" yaml:"maximum_output_bytes"`
	Disclosure         map[string]ClassLimits `json:"disclosure" yaml:"disclosure"`
	Credential         *SecretReference       `json:"credential,omitempty" yaml:"credential,omitempty"`
	timeout            time.Duration
}

// Command identifies one absolute bridge executable and fixed argument list.
type Command struct {
	Executable string   `json:"executable" yaml:"executable"`
	Arguments  []string `json:"arguments,omitempty" yaml:"arguments,omitempty"`
}

// ClassLimits bounds one approved evidence class independently.
type ClassLimits struct {
	MaximumItems     uint64 `json:"maximum_items,omitempty" yaml:"maximum_items,omitempty"`
	MaximumArtifacts uint64 `json:"maximum_artifacts,omitempty" yaml:"maximum_artifacts,omitempty"`
	MaximumBytes     uint64 `json:"maximum_bytes" yaml:"maximum_bytes"`
}

// SecretReference names a credential source without containing a credential.
type SecretReference struct {
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
}

// TimeoutDuration returns the validated request deadline.
func (profile Profile) TimeoutDuration() time.Duration { return profile.timeout }

// DefaultPath returns the platform per-user diagnosis configuration path.
func DefaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve diagnosis configuration directory: %w", err)
	}

	return filepath.Join(directory, "jobman", "diagnosis.yml"), nil
}

// ResolvePath applies CLI, environment, then per-user path precedence.
func ResolvePath(explicit string) (string, string, error) {
	if explicit != "" {
		return explicit, "command_line", nil
	}
	if environment, ok := os.LookupEnv(EnvironmentPath); ok && environment != "" {
		if !filepath.IsAbs(environment) || filepath.Clean(environment) != environment {
			return "", "", fmt.Errorf("%s must name a clean absolute path", EnvironmentPath)
		}

		return environment, "environment", nil
	}
	path, err := DefaultPath()
	if err != nil {
		return "", "", err
	}

	return path, "user_default", nil
}

// LoadFile reads and validates one bounded strict configuration document.
func LoadFile(path string) (File, error) {
	if strings.TrimSpace(path) == "" {
		return File{}, errors.New("load diagnosis configuration: path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration %q: inspect: %w", path, err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return File{}, fmt.Errorf("load diagnosis configuration %q: file must be regular and not group/world writable", path)
	}
	// #nosec G304 -- the path is explicit or resolved only from the per-user configuration policy above.
	file, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration %q: open: %w", path, err)
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, maximumConfigBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration %q: read: %w", path, err)
	}
	if len(encoded) > maximumConfigBytes {
		return File{}, fmt.Errorf("load diagnosis configuration: input exceeds %d bytes", maximumConfigBytes)
	}
	if err := validateYAMLStructure(encoded); err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(encoded))
	decoder.KnownFields(true)
	var configuration File
	if err := decoder.Decode(&configuration); err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple YAML documents are not allowed")
		}
		return File{}, fmt.Errorf("load diagnosis configuration: %w", err)
	}
	if err := configuration.Validate(); err != nil {
		return File{}, fmt.Errorf("load diagnosis configuration: %w", err)
	}

	return configuration, nil
}

// SelectProfile returns an explicit profile or the configured default.
func (configuration File) SelectProfile(name string) (string, Profile, error) {
	if name == "" {
		name = configuration.Defaults.Profile
	}
	if !validName(name) {
		return "", Profile{}, errors.New("select diagnosis profile: a valid profile name is required")
	}
	profile, ok := configuration.Profiles[name]
	if !ok {
		return "", Profile{}, fmt.Errorf("select diagnosis profile: profile %q is not defined", name)
	}

	return name, profile, nil
}

// Load reads a configuration and returns an explicit or default profile.
func Load(path, profileName string) (Profile, error) {
	configuration, err := LoadFile(path)
	if err != nil {
		return Profile{}, err
	}
	_, profile, err := configuration.SelectProfile(profileName)

	return profile, err
}

// Validate checks every profile without resolving credentials or contacting a provider.
func (configuration *File) Validate() error {
	if configuration == nil || configuration.SchemaVersion != SchemaVersion ||
		len(configuration.Profiles) == 0 || len(configuration.Profiles) > maximumProfiles {
		return errors.New("schema_version must be 2 and profiles must contain between 1 and 32 entries")
	}
	if !validName(configuration.Defaults.Profile) {
		return errors.New("defaults.profile must name a valid profile")
	}
	for name, profile := range configuration.Profiles {
		if !validName(name) {
			return fmt.Errorf("profile name %q is invalid", name)
		}
		if err := profile.validate(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
		configuration.Profiles[name] = profile
	}
	if _, ok := configuration.Profiles[configuration.Defaults.Profile]; !ok {
		return fmt.Errorf("defaults.profile %q is not defined", configuration.Defaults.Profile)
	}

	return nil
}

//nolint:cyclop // Profile validation is a single fail-closed boundary over transport, limits, and disclosure.
func (profile *Profile) validate() error {
	if profile.Provider != "command" && profile.Provider != "openai_compatible" && profile.Provider != "ollama" {
		return errors.New("provider must be command, openai_compatible, or ollama")
	}
	if profile.Locality != provider.LocalityLocal && profile.Locality != provider.LocalityRemote {
		return errors.New("locality must be local or remote")
	}
	if strings.TrimSpace(profile.Model) == "" || len(profile.Model) > 256 || strings.ContainsAny(profile.Model, "\r\n\x00") {
		return errors.New("model must contain between 1 and 256 safe bytes")
	}
	if !profile.RequireJSONSchema {
		return errors.New("require_json_schema must be true")
	}
	duration, err := time.ParseDuration(profile.Timeout)
	if err != nil || duration < 100*time.Millisecond || duration > 5*time.Minute {
		return errors.New("timeout must be between 100ms and 5m")
	}
	profile.timeout = duration
	if profile.MaximumInputBytes < 4*1024 || profile.MaximumInputBytes > 2*1024*1024 ||
		profile.MaximumOutputBytes < 1024 || profile.MaximumOutputBytes > 256*1024 {
		return errors.New("input/output byte limits are outside supported bounds")
	}
	if err := profile.validateDisclosure(); err != nil {
		return err
	}
	if err := validateSecretReference(profile.Credential); err != nil {
		return err
	}

	return profile.validateTransport()
}

//nolint:cyclop // Supported classes have deliberately asymmetric limit invariants.
func (profile Profile) validateDisclosure() error {
	if len(profile.Disclosure) == 0 || len(profile.Disclosure) > 5 {
		return errors.New("disclosure must explicitly allow metadata and optional bounded context classes")
	}
	if _, ok := profile.Disclosure["metadata"]; !ok {
		return errors.New("disclosure must include bounded metadata")
	}
	for class, limits := range profile.Disclosure {
		if class != "metadata" && class != "command" && class != "path" &&
			class != "environment_name" && class != "log_content" {
			return fmt.Errorf("disclosure class %q is not supported", class)
		}
		// #nosec G115 -- profile.MaximumInputBytes was validated positive and at most 2 MiB above.
		maximumInputBytes := uint64(profile.MaximumInputBytes)
		if limits.MaximumBytes == 0 || limits.MaximumBytes > maximumInputBytes {
			return fmt.Errorf("disclosure class %q has an invalid maximum_bytes", class)
		}
		switch class {
		case "metadata", "command", "path", "environment_name":
			if limits.MaximumItems == 0 || limits.MaximumItems > 1024 || limits.MaximumArtifacts != 0 {
				return fmt.Errorf("%s disclosure requires maximum_items between 1 and 1024", class)
			}
		case "log_content":
			if limits.MaximumArtifacts == 0 || limits.MaximumArtifacts > 16 || limits.MaximumItems != 0 {
				return errors.New("log_content disclosure requires maximum_artifacts between 1 and 16")
			}
		}
	}

	return nil
}

//nolint:cyclop,gocognit // Provider-specific union validation stays centralized to prevent ambiguous profiles.
func (profile Profile) validateTransport() error {
	if profile.Provider == "command" {
		if profile.Locality != provider.LocalityLocal || profile.Endpoint != "" || profile.Command == nil {
			return errors.New("command provider requires local locality, a command, and no endpoint")
		}
		if !filepath.IsAbs(profile.Command.Executable) || filepath.Clean(profile.Command.Executable) != profile.Command.Executable {
			return errors.New("command executable must be a clean absolute path")
		}
		if len(profile.Command.Arguments) > 64 {
			return errors.New("command accepts at most 64 fixed arguments")
		}
		for _, argument := range profile.Command.Arguments {
			if len(argument) > 4096 || strings.ContainsRune(argument, '\x00') {
				return errors.New("command argument is too long or contains NUL")
			}
		}

		return nil
	}
	if profile.Command != nil || profile.Endpoint == "" {
		return errors.New("HTTP provider requires an endpoint and no command")
	}
	parsed, err := url.Parse(profile.Endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Path == "" || parsed.Path == "/" {
		return errors.New("endpoint must be an exact absolute URL without credentials, query, or fragment")
	}
	localHost := isLoopbackHost(parsed.Hostname())
	if profile.Locality == provider.LocalityLocal {
		if parsed.Scheme != "http" && parsed.Scheme != "https" || !localHost {
			return errors.New("local endpoint must use HTTP(S) on a loopback host")
		}
	} else if parsed.Scheme != "https" || localHost {
		return errors.New("remote endpoint must use HTTPS and must not identify a loopback host")
	}
	if profile.Provider == "ollama" {
		if profile.Locality != provider.LocalityLocal || parsed.Path != "/api/chat" {
			return errors.New("ollama requires a local endpoint whose exact path is /api/chat")
		}
	}

	return nil
}

func validateSecretReference(reference *SecretReference) error {
	if reference == nil {
		return nil
	}
	if (reference.Environment == "") == (reference.File == "") {
		return errors.New("credential must name exactly one environment or file source")
	}
	if reference.Environment != "" && !validEnvironmentName(reference.Environment) {
		return errors.New("credential environment name is invalid")
	}
	if reference.File != "" && (!filepath.IsAbs(reference.File) || filepath.Clean(reference.File) != reference.File) {
		return errors.New("credential file must be a clean absolute path")
	}

	return nil
}

func validateYAMLStructure(encoded []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(encoded, &document); err != nil {
		return err
	}
	return walkYAML(&document, 0)
}

func walkYAML(node *yaml.Node, depth int) error {
	if depth > maximumConfigDepth {
		return fmt.Errorf("YAML nesting exceeds %d", maximumConfigDepth)
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("YAML aliases and anchors are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Value == "<<" {
				return errors.New("YAML mapping keys must be ordinary strings")
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return fmt.Errorf("duplicate YAML mapping key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := walkYAML(child, depth+1); err != nil {
			return err
		}
	}

	return nil
}

func validName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}

	return true
}

func validEnvironmentName(value string) bool {
	if value == "" || len(value) > 128 || value[0] != '_' && (value[0] < 'A' || value[0] > 'Z') {
		return false
	}
	for _, character := range value[1:] {
		if character == '_' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		return false
	}

	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)

	return address != nil && address.IsLoopback()
}

// ApprovedClasses returns the deterministic sorted intersection of the
// profile allowlist and repeated CLI approvals.
func (profile Profile) ApprovedClasses(approved []string) ([]string, error) {
	seen := make(map[string]struct{}, len(approved))
	for _, class := range approved {
		if class != "metadata" && class != "command" && class != "path" &&
			class != "environment_name" && class != "log_content" {
			return nil, fmt.Errorf("unsupported --share class %q", class)
		}
		seen[class] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for class := range seen {
		if _, allowed := profile.Disclosure[class]; allowed {
			result = append(result, class)
		}
	}
	slices.Sort(result)
	if len(result) == 0 {
		return nil, errors.New("selected AI profile has no disclosure class approved for this invocation")
	}

	return result, nil
}
