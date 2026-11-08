package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitSystemFallbackNeverMasksUserOrExplicitErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvironmentPath, "")
	system := writeConfig(t, validOpenAIConfig())
	t.Setenv(SystemEnvironmentPath, system)
	path, origin, err := ResolvePath("")
	if err != nil || path != system || origin != "system_fallback" {
		t.Fatalf("system fallback = %q, %q, %v", path, origin, err)
	}
	user, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(user), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeErr := os.WriteFile(user, []byte("broken: ["), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	path, origin, err = ResolvePath("")
	if err != nil || path != user || origin != "user_default" {
		t.Fatalf("existing user selection = %q, %q, %v", path, origin, err)
	}
	if _, loadErr := LoadFile(path); loadErr == nil {
		t.Fatal("malformed user file must fail rather than use system configuration")
	}
	missing := filepath.Join(t.TempDir(), "missing.yml")
	t.Setenv(EnvironmentPath, missing)
	path, origin, err = ResolvePath("")
	if err != nil || path != missing || origin != "environment" {
		t.Fatalf("explicit environment selection = %q, %q, %v", path, origin, err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("missing explicit file must fail")
	}
}

func TestSystemFallbackRequiresExplicitAbsolutePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvironmentPath, "")
	t.Setenv(SystemEnvironmentPath, "relative.yml")
	if _, _, err := ResolvePath(""); err == nil {
		t.Fatal("relative system fallback must fail")
	}
}

func TestTransportValidationDoesNotReadReferencedFiles(t *testing.T) {
	t.Parallel()
	configuration := strings.Replace(validOpenAIConfig(), "model: structured-model", `model: structured-model
    transport:
      ca_file: /not-present/ca.pem
      client_certificate_file: /not-present/client.pem
      client_key_file: /not-present/key.pem
      proxy_url: http://proxy.example.invalid:3128`, 1)
	file, err := LoadFile(writeConfig(t, configuration))
	if err != nil {
		t.Fatal(err)
	}
	if file.Profiles["hosted"].Transport == nil {
		t.Fatal("transport configuration was discarded")
	}
	for _, invalid := range []string{
		strings.Replace(configuration, "client_key_file: /not-present/key.pem", "client_key_file: relative", 1),
		strings.Replace(configuration, "http://proxy.example.invalid:3128", "http://name:secret-canary@proxy.example.invalid:3128", 1),
		strings.Replace(configuration, "client_key_file: /not-present/key.pem", "unknown_transport_setting: true", 1),
	} {
		if _, err := LoadFile(writeConfig(t, invalid)); err == nil {
			t.Fatal("invalid transport accepted")
		}
	}
}

func TestSchemaIncludesTransportAndRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(JSONSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"client_certificate_file", "client_key_file", "ca_file", "proxy_url", `"additionalProperties":false`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("schema missing %s", field)
		}
	}
}
