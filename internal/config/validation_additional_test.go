package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestProfileValidationRejectsUnsafeVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Profile)
	}{
		{name: "provider", mutate: func(value *Profile) { value.Provider = "other" }},
		{name: "locality", mutate: func(value *Profile) { value.Locality = "other" }},
		{name: "model empty", mutate: func(value *Profile) { value.Model = " " }},
		{name: "model newline", mutate: func(value *Profile) { value.Model = "model\nother" }},
		{name: "schema disabled", mutate: func(value *Profile) { value.RequireJSONSchema = false }},
		{name: "timeout parse", mutate: func(value *Profile) { value.Timeout = "soon" }},
		{name: "timeout short", mutate: func(value *Profile) { value.Timeout = "99ms" }},
		{name: "timeout long", mutate: func(value *Profile) { value.Timeout = "6m" }},
		{name: "input low", mutate: func(value *Profile) { value.MaximumInputBytes = 4095 }},
		{name: "input high", mutate: func(value *Profile) { value.MaximumInputBytes = 2*1024*1024 + 1 }},
		{name: "output low", mutate: func(value *Profile) { value.MaximumOutputBytes = 1023 }},
		{name: "output high", mutate: func(value *Profile) { value.MaximumOutputBytes = 256*1024 + 1 }},
		{name: "disclosure empty", mutate: func(value *Profile) { value.Disclosure = nil }},
		{name: "metadata absent", mutate: func(value *Profile) {
			value.Disclosure = map[string]ClassLimits{"command": {MaximumItems: 1, MaximumBytes: 1}}
		}},
		{name: "class unsupported", mutate: func(value *Profile) { value.Disclosure["secret"] = ClassLimits{MaximumItems: 1, MaximumBytes: 1} }},
		{name: "bytes zero", mutate: func(value *Profile) { value.Disclosure["metadata"] = ClassLimits{MaximumItems: 1} }},
		{name: "bytes over input", mutate: func(value *Profile) {
			value.Disclosure["metadata"] = ClassLimits{MaximumItems: 1, MaximumBytes: 2 * 1024 * 1024}
		}},
		{name: "metadata items zero", mutate: func(value *Profile) { value.Disclosure["metadata"] = ClassLimits{MaximumBytes: 1} }},
		{name: "metadata artifacts", mutate: func(value *Profile) {
			value.Disclosure["metadata"] = ClassLimits{MaximumItems: 1, MaximumArtifacts: 1, MaximumBytes: 1}
		}},
		{name: "logs artifacts zero", mutate: func(value *Profile) { value.Disclosure["log_content"] = ClassLimits{MaximumBytes: 1} }},
		{name: "logs items", mutate: func(value *Profile) {
			value.Disclosure["log_content"] = ClassLimits{MaximumItems: 1, MaximumArtifacts: 1, MaximumBytes: 1}
		}},
		{name: "source artifacts zero", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumBytes: 1}
		}},
		{name: "source items", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumItems: 1, MaximumArtifacts: 1, MaximumBytes: 1}
		}},
		{name: "source bytes over hard limit", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumArtifacts: 1, MaximumBytes: 1024*1024 + 1}
		}},
		{name: "source context mode", mutate: func(value *Profile) {
			value.SourceContext = &SourceContextPolicy{Mode: "sometimes"}
		}},
		{name: "source context limited missing lines", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumArtifacts: 1, MaximumBytes: 64 * 1024}
			value.SourceContext = &SourceContextPolicy{Mode: SourceContextModeLimited}
		}},
		{name: "source context limited excessive lines", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumArtifacts: 1, MaximumBytes: 64 * 1024}
			value.SourceContext = &SourceContextPolicy{
				Mode: SourceContextModeLimited, LinesBeforeAndAfter: MaximumSourceContextLines + 1,
			}
		}},
		{name: "source context full with lines", mutate: func(value *Profile) {
			value.Disclosure["source_content"] = ClassLimits{MaximumArtifacts: 1, MaximumBytes: 64 * 1024}
			value.SourceContext = &SourceContextPolicy{Mode: SourceContextModeFull, LinesBeforeAndAfter: 20}
		}},
		{name: "source context sharing without disclosure", mutate: func(value *Profile) {
			value.SourceContext = &SourceContextPolicy{Mode: SourceContextModeFull}
		}},
		{name: "credential ambiguous", mutate: func(value *Profile) { value.Credential = &SecretReference{} }},
		{name: "credential environment", mutate: func(value *Profile) { value.Credential = &SecretReference{Environment: "lowercase"} }},
		{name: "credential file", mutate: func(value *Profile) { value.Credential = &SecretReference{File: "relative"} }},
		{name: "HTTP command union", mutate: func(value *Profile) { value.Command = &Command{Executable: "/bin/true"} }},
		{name: "HTTP endpoint empty", mutate: func(value *Profile) { value.Endpoint = "" }},
		{name: "HTTP URL query", mutate: func(value *Profile) { value.Endpoint = "https://example.com/v1/chat?secret=x" }},
		{name: "local remote host", mutate: func(value *Profile) { value.Locality = provider.LocalityLocal }},
		{name: "remote HTTP", mutate: func(value *Profile) { value.Endpoint = "http://example.com/v1/chat" }},
		{name: "remote loopback", mutate: func(value *Profile) { value.Endpoint = "https://127.0.0.1/v1/chat" }},
		{name: "ollama remote", mutate: func(value *Profile) { value.Provider = "ollama" }},
		{name: "ollama path", mutate: func(value *Profile) {
			value.Provider = "ollama"
			value.Locality = provider.LocalityLocal
			value.Endpoint = "http://127.0.0.1/v1/chat"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile := validValidationProfile()
			test.mutate(&profile)
			if err := profile.validate(); err == nil {
				t.Fatal("validate() error = nil")
			}
		})
	}
}

func TestProfileAllowsExplicitBoundedSourceDisclosure(t *testing.T) {
	t.Parallel()

	profile := validValidationProfile()
	profile.Disclosure["source_content"] = ClassLimits{MaximumArtifacts: 1, MaximumBytes: 64 * 1024}
	profile.SourceContext = &SourceContextPolicy{
		Mode: SourceContextModeLimited, LinesBeforeAndAfter: 20,
	}
	if err := profile.validate(); err != nil {
		t.Fatal(err)
	}
	profile.SourceContext = &SourceContextPolicy{Mode: SourceContextModeFull}
	if err := profile.validate(); err != nil {
		t.Fatal(err)
	}
	profile.Disclosure = map[string]ClassLimits{"metadata": {MaximumItems: 1, MaximumBytes: 1}}
	profile.SourceContext = &SourceContextPolicy{Mode: SourceContextModeNone}
	if err := profile.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandTransportValidation(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	valid := validValidationProfile()
	valid.Provider = "command"
	valid.Locality = provider.LocalityLocal
	valid.Endpoint = ""
	valid.Command = &Command{Executable: executable, Arguments: []string{"--structured"}}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	tests := []func(*Profile){
		func(value *Profile) { value.Locality = provider.LocalityRemote },
		func(value *Profile) { value.Endpoint = "http://127.0.0.1/api" },
		func(value *Profile) { value.Command = nil },
		func(value *Profile) { value.Command.Executable = "relative" },
		func(value *Profile) { value.Command.Arguments = make([]string, 65) },
		func(value *Profile) { value.Command.Arguments = []string{strings.Repeat("x", 4097)} },
		func(value *Profile) { value.Command.Arguments = []string{"nul\x00argument"} },
	}
	for index, mutate := range tests {
		profile := valid
		profile.Command = &Command{Executable: valid.Command.Executable, Arguments: append([]string{}, valid.Command.Arguments...)}
		mutate(&profile)
		if err := profile.validate(); err == nil {
			t.Fatalf("command variant %d error = nil", index)
		}
	}
}

func TestConfigurationAndClassSelectionValidation(t *testing.T) {
	t.Parallel()

	var nilConfiguration *File
	if nilConfiguration.Validate() == nil {
		t.Fatal("nil Validate() error = nil")
	}
	for _, configuration := range []*File{
		{SchemaVersion: 1, Defaults: Defaults{Profile: "test"}, Profiles: map[string]Profile{"test": validValidationProfile()}},
		{SchemaVersion: 2, Defaults: Defaults{}, Profiles: map[string]Profile{"test": validValidationProfile()}},
		{SchemaVersion: 2, Defaults: Defaults{Profile: "test"}, Profiles: map[string]Profile{"Bad Name": validValidationProfile()}},
		{SchemaVersion: 2, Defaults: Defaults{Profile: "missing"}, Profiles: map[string]Profile{"test": validValidationProfile()}},
	} {
		if configuration.Validate() == nil {
			t.Fatalf("Validate(%#v) error = nil", configuration)
		}
	}
	profile := validValidationProfile()
	if _, err := profile.ApprovedClasses([]string{"unknown"}); err == nil {
		t.Fatal("ApprovedClasses(unknown) error = nil")
	}
	if _, err := profile.ApprovedClasses([]string{"command"}); err == nil {
		t.Fatal("ApprovedClasses(disallowed) error = nil")
	}
	if !isLoopbackHost("LOCALHOST") || !isLoopbackHost("::1") || isLoopbackHost("example.com") {
		t.Fatal("isLoopbackHost() classification changed")
	}
	for _, name := range []string{"valid", "valid.name-1", "_valid"} {
		if !validName(name) {
			t.Fatalf("validName(%q) = false", name)
		}
	}
	for _, name := range []string{"", "Bad", strings.Repeat("x", 65)} {
		if validName(name) {
			t.Fatalf("validName(%q) = true", name)
		}
	}
}

func TestResolveCredentialRejectsUnavailableAndUnboundedValues(t *testing.T) {
	t.Parallel()

	if value, err := ResolveCredential(nil, nil); err != nil || value != nil {
		t.Fatalf("ResolveCredential(nil) = %q, %v", value, err)
	}
	reference := &SecretReference{Environment: "TOKEN"}
	if _, err := ResolveCredential(reference, nil); err == nil {
		t.Fatal("ResolveCredential(nil resolver) error = nil")
	}
	if _, err := ResolveCredential(reference, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("ResolveCredential(missing value) error = nil")
	}
	if _, err := ResolveCredential(reference, func(string) (string, bool) { return strings.Repeat("x", maximumSecretBytes+1), true }); err == nil {
		t.Fatal("ResolveCredential(oversized environment) error = nil")
	}
	for _, contents := range [][]byte{nil, bytesOfSize(maximumSecretBytes + 1)} {
		path := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveCredential(&SecretReference{File: path}, nil); err == nil {
			t.Fatalf("ResolveCredential(file size %d) error = nil", len(contents))
		}
	}
	if _, err := ResolveCredential(&SecretReference{File: filepath.Join(t.TempDir(), "missing")}, nil); err == nil {
		t.Fatal("ResolveCredential(missing file) error = nil")
	}
}

func validValidationProfile() Profile {
	return Profile{
		Provider: "openai_compatible", Locality: provider.LocalityRemote,
		Endpoint: "https://example.com/v1/chat", Model: "model", RequireJSONSchema: true,
		Timeout: "10s", MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 16 * 1024,
		Disclosure: map[string]ClassLimits{"metadata": {MaximumItems: 64, MaximumBytes: 32 * 1024}},
	}
}

func bytesOfSize(size int) []byte { return []byte(strings.Repeat("x", size)) }
