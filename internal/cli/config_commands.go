package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
)

func runConfigCommand(arguments []string, stdout, stderr io.Writer) error {
	if len(arguments) == 0 {
		return usageError(errors.New("config requires paths, validate, or show"))
	}
	if arguments[0] == "--help" || arguments[0] == "-h" {
		_, err := fmt.Fprintln(stderr, "usage: jobman-diagnose config {paths|validate|show} [PATH]")
		if err != nil {
			return err
		}

		return flag.ErrHelp
	}
	switch arguments[0] {
	case "paths":
		if len(arguments) != 1 {
			return usageError(errors.New("config paths does not accept arguments"))
		}

		return writeConfigPaths(stdout)
	case "validate":
		return validateConfigCommand(arguments[1:], stdout)
	case "show":
		return showConfigCommand(arguments[1:], stdout)
	default:
		return usageError(fmt.Errorf("unknown config command %q", arguments[0]))
	}
}

func writeConfigPaths(stdout io.Writer) error {
	defaultPath, err := diagnosisconfig.DefaultPath()
	if err != nil {
		return err
	}
	environmentPath := os.Getenv(diagnosisconfig.EnvironmentPath)
	effectivePath, origin, err := diagnosisconfig.ResolvePath("")
	if err != nil {
		return err
	}
	if environmentPath == "" {
		environmentPath = "(not set)"
	}
	_, err = fmt.Fprintf(
		stdout, "user:        %s\nenvironment: %s\neffective:   %s (%s)\n",
		defaultPath, environmentPath, effectivePath, origin,
	)

	return err
}

func validateConfigCommand(arguments []string, stdout io.Writer) error {
	if len(arguments) > 1 {
		return usageError(errors.New("config validate accepts at most one path"))
	}
	explicit := ""
	for _, argument := range arguments {
		explicit = argument
	}
	path, _, err := diagnosisconfig.ResolvePath(explicit)
	if err != nil {
		return err
	}
	if _, loadErr := diagnosisconfig.LoadFile(path); loadErr != nil {
		return loadErr
	}
	_, err = fmt.Fprintf(stdout, "diagnosis configuration is valid: %s\n", path)

	return err
}

func showConfigCommand(arguments []string, stdout io.Writer) error {
	if len(arguments) > 1 {
		return usageError(errors.New("config show accepts at most one path"))
	}
	explicit := ""
	for _, argument := range arguments {
		explicit = argument
	}
	path, _, err := diagnosisconfig.ResolvePath(explicit)
	if err != nil {
		return err
	}
	configuration, err := diagnosisconfig.LoadFile(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	return encoder.Encode(configuration)
}

func runProfilesCommand(arguments []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("jobman-diagnose profiles", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configurationPath := ""
	flags.StringVar(&configurationPath, "diagnosis-config", "", "override the per-user diagnosis configuration path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError(errors.New("profiles does not accept positional arguments"))
	}
	path, _, err := diagnosisconfig.ResolvePath(configurationPath)
	if err != nil {
		return err
	}
	configuration, err := diagnosisconfig.LoadFile(path)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(configuration.Profiles))
	for name := range configuration.Profiles {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		profile := configuration.Profiles[name]
		classes := make([]string, 0, len(profile.Disclosure))
		for class := range profile.Disclosure {
			classes = append(classes, class)
		}
		slices.Sort(classes)
		marker := " "
		if name == configuration.Defaults.Profile {
			marker = "*"
		}
		source := "none"
		if profile.SourceContext != nil {
			source = profile.SourceContext.Mode
			if profile.SourceContext.Mode == diagnosisconfig.SourceContextModeLimited {
				source = fmt.Sprintf("limited:%d-lines-each-side", profile.SourceContext.LinesBeforeAndAfter)
			}
		}
		if _, err := fmt.Fprintf(
			stdout, "%s %s\tprovider=%s\tlocality=%s\tmodel=%s\tsource=%s\tdisclosure=%s\n",
			marker, name, profile.Provider, profile.Locality, profile.Model, source, strings.Join(classes, ","),
		); err != nil {
			return err
		}
	}

	return nil
}
