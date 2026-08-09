package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestLoadStrictProfileAndApproveDisclosureIntersection(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `schema_version: 2
defaults:
  profile: local-ollama
profiles:
  local-ollama:
    provider: ollama
    locality: local
    endpoint: http://127.0.0.1:11434/api/chat
    model: qwen3:8b
    require_json_schema: true
    timeout: 30s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 128
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
      log_content:
        maximum_artifacts: 2
        maximum_bytes: 65536
`)
	profile, err := Load(path, "local-ollama")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Provider != "ollama" || profile.Locality != provider.LocalityLocal || profile.TimeoutDuration() == 0 {
		t.Fatalf("profile = %#v", profile)
	}
	approved, err := profile.ApprovedClasses([]string{
		"log_content", "command", "path", "environment_name", "metadata", "metadata",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(approved, ",") != "command,environment_name,log_content,metadata,path" {
		t.Fatalf("approved classes = %v", approved)
	}
}

func TestLoadRejectsAmbiguousOrCredentialBearingYAML(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"duplicate":                  strings.Replace(validCommandConfig(), "provider: command", "provider: command\n    provider: command", 1),
		"unknown literal credential": strings.Replace(validCommandConfig(), "model: bridge", "model: bridge\n    api_key: secret-literal", 1),
		"anchor":                     strings.Replace(validCommandConfig(), "provider: command", "provider: &provider command", 1),
		"multiple documents":         validCommandConfig() + "\n---\nschema_version: 2\ndefaults: {profile: bridge}\nprofiles: {}\n",
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(writeConfig(t, encoded), "bridge"); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadRejectsUnsafeTransportAndDisclosure(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"remote HTTP":           strings.Replace(validOpenAIConfig(), "https://example.com", "http://example.com", 1),
		"remote loopback":       strings.Replace(validOpenAIConfig(), "https://example.com", "https://127.0.0.1", 1),
		"endpoint credential":   strings.Replace(validOpenAIConfig(), "https://example.com", "https://token@example.com", 1),
		"local only disclosure": strings.Replace(validOpenAIConfig(), "metadata:", "local_only:", 1),
		"schema disabled":       strings.Replace(validOpenAIConfig(), "require_json_schema: true", "require_json_schema: false", 1),
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(writeConfig(t, encoded), "hosted"); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestResolveCredentialByEnvironmentOrPrivateFile(t *testing.T) {
	t.Parallel()

	value, err := ResolveCredential(&SecretReference{Environment: "MODEL_TOKEN"}, func(name string) (string, bool) {
		return "resolved-secret", name == "MODEL_TOKEN"
	})
	if err != nil || string(value) != "resolved-secret" {
		t.Fatalf("ResolveCredential(environment) = %q, %v", value, err)
	}
	path := filepath.Join(t.TempDir(), "credential")
	if writeErr := os.WriteFile(path, []byte("file-secret\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	value, err = ResolveCredential(&SecretReference{File: path}, nil)
	if err != nil || string(value) != "file-secret" {
		t.Fatalf("ResolveCredential(file) = %q, %v", value, err)
	}
	if runtime.GOOS != "windows" {
		// #nosec G302 -- intentionally make the test fixture unsafe and assert rejection.
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveCredential(&SecretReference{File: path}, nil); err == nil {
			t.Fatal("ResolveCredential(public file) error = nil")
		}
	}
}

func TestResolvePathAndSelectDefaultProfile(t *testing.T) {
	t.Setenv(EnvironmentPath, "")
	t.Setenv("HOME", t.TempDir())
	defaultPath, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	resolved, origin, err := ResolvePath("")
	if err != nil || resolved != defaultPath || origin != "user_default" {
		t.Fatalf("ResolvePath(default) = %q / %q / %v", resolved, origin, err)
	}
	explicit := filepath.Join(t.TempDir(), "explicit.yml")
	if resolved, origin, err = ResolvePath(explicit); err != nil || resolved != explicit || origin != "command_line" {
		t.Fatalf("ResolvePath(explicit) = %q / %q / %v", resolved, origin, err)
	}
	environment := filepath.Join(t.TempDir(), "environment.yml")
	t.Setenv(EnvironmentPath, environment)
	if resolved, origin, err = ResolvePath(""); err != nil || resolved != environment || origin != "environment" {
		t.Fatalf("ResolvePath(environment) = %q / %q / %v", resolved, origin, err)
	}
	configuration, err := LoadFile(writeConfig(t, validOpenAIConfig()))
	if err != nil {
		t.Fatal(err)
	}
	name, profile, err := configuration.SelectProfile("")
	if err != nil || name != "hosted" || profile.Model != "structured-model" {
		t.Fatalf("SelectProfile(default) = %q / %#v / %v", name, profile, err)
	}
}

func TestLoadFileRejectsWritableOrIndirectConfiguration(t *testing.T) {
	path := writeConfig(t, validOpenAIConfig())
	if runtime.GOOS != "windows" {
		// #nosec G302 -- the test deliberately grants unsafe write permissions.
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFile(path); err == nil {
			t.Fatal("LoadFile(group/world writable) error = nil")
		}
	}
	target := writeConfig(t, validOpenAIConfig())
	link := filepath.Join(t.TempDir(), "diagnosis-link.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err == nil {
		t.Fatal("LoadFile(symlink) error = nil")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diagnosis.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func validCommandConfig() string {
	return `schema_version: 2
defaults:
  profile: bridge
profiles:
  bridge:
    provider: command
    locality: local
    command:
      executable: /usr/bin/true
      arguments: [--structured]
    model: bridge
    require_json_schema: true
    timeout: 10s
    maximum_input_bytes: 65536
    maximum_output_bytes: 16384
    disclosure:
      metadata:
        maximum_items: 64
        maximum_bytes: 32768
`
}

func validOpenAIConfig() string {
	return `schema_version: 2
defaults:
  profile: hosted
profiles:
  hosted:
    provider: openai_compatible
    locality: remote
    endpoint: https://example.com/v1/chat/completions
    model: structured-model
    require_json_schema: true
    timeout: 30s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 128
        maximum_bytes: 131072
    credential:
      environment: MODEL_TOKEN
`
}
