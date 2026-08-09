package generation

import (
	"fmt"
	"os"

	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/provider/commandbridge"
	"github.com/ryancswallace/jobman-diagnose/provider/ollama"
	"github.com/ryancswallace/jobman-diagnose/provider/openaicompat"
)

// NewGenerator resolves an explicitly referenced credential and constructs
// exactly the adapter named by a validated profile. It performs no endpoint
// discovery or transport fallback.
func NewGenerator(profile config.Profile) (Generator, error) {
	credential, err := config.ResolveCredential(profile.Credential, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	defer clear(credential)
	switch profile.Provider {
	case "command":
		if profile.Command == nil {
			return nil, fmt.Errorf("construct generator: command profile has no command")
		}
		value, err := commandbridge.New(commandbridge.Config{
			Executable: profile.Command.Executable, Arguments: profile.Command.Arguments,
			Model: profile.Model, Credential: credential, MaximumInputBytes: profile.MaximumInputBytes,
			MaximumOutputBytes: profile.MaximumOutputBytes,
		})
		if err != nil {
			return nil, err
		}

		return value, nil
	case "openai_compatible":
		value, err := openaicompat.New(openaicompat.Config{
			Endpoint: profile.Endpoint, Model: profile.Model, Credential: credential, Locality: profile.Locality,
			MaximumInputBytes: profile.MaximumInputBytes, MaximumOutputBytes: profile.MaximumOutputBytes,
			RequestTimeout: profile.TimeoutDuration(),
		})
		if err != nil {
			return nil, err
		}

		return value, nil
	case "ollama":
		value, err := ollama.New(ollama.Config{
			Endpoint: profile.Endpoint, Model: profile.Model, Credential: credential,
			MaximumInputBytes: profile.MaximumInputBytes, MaximumOutputBytes: profile.MaximumOutputBytes,
			RequestTimeout: profile.TimeoutDuration(),
		})
		if err != nil {
			return nil, err
		}

		return value, nil
	default:
		return nil, fmt.Errorf("construct generator: unsupported provider %q", profile.Provider)
	}
}
