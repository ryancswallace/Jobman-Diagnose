package generation

import (
	"os"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/internal/config"
)

func TestNewGeneratorConstructsConfiguredAdapters(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base := testProfile(t, false)
	tests := []struct {
		name    string
		profile config.Profile
	}{
		{name: "openai compatible", profile: base},
		{name: "ollama", profile: func() config.Profile {
			profile := base
			profile.Provider = "ollama"
			profile.Locality = "local"
			profile.Endpoint = "http://127.0.0.1:11434/api/chat"
			return profile
		}()},
		{name: "command", profile: func() config.Profile {
			profile := base
			profile.Provider = "command"
			profile.Locality = "local"
			profile.Endpoint = ""
			profile.Command = &config.Command{Executable: executable, Arguments: []string{"--structured"}}
			return profile
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			generator, err := NewGenerator(test.profile)
			if err != nil {
				t.Fatal(err)
			}
			if generator == nil {
				t.Fatal("NewGenerator() = nil")
			}
		})
	}
}

func TestNewGeneratorRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("MISSING_GENERATOR_TOKEN", "")

	base := testProfile(t, false)
	tests := []struct {
		name    string
		profile config.Profile
	}{
		{name: "unsupported provider", profile: func() config.Profile {
			profile := base
			profile.Provider = "other"
			return profile
		}()},
		{name: "command without command", profile: func() config.Profile {
			profile := base
			profile.Provider = "command"
			profile.Command = nil
			return profile
		}()},
		{name: "missing credential", profile: func() config.Profile {
			profile := base
			profile.Credential = &config.SecretReference{Environment: "MISSING_GENERATOR_TOKEN"}
			return profile
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewGenerator(test.profile); err == nil {
				t.Fatal("NewGenerator() error = nil")
			}
		})
	}

	credentialFile := t.TempDir()
	profile := base
	profile.Credential = &config.SecretReference{File: credentialFile}
	if _, err := os.Stat(credentialFile); err != nil {
		t.Fatal(err)
	}
	if _, err := NewGenerator(profile); err == nil {
		t.Fatal("NewGenerator(directory credential) error = nil")
	}
}
