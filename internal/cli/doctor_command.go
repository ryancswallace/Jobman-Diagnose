package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/doctor"
	"github.com/ryancswallace/jobman-diagnose/internal/generation"
)

func runDoctorCommand(arguments []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runDoctorCommandContext(ctx, arguments, stdout, stderr)
}

func runDoctorCommandContext(ctx context.Context, arguments []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("jobman-diagnose doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configurationPath := ""
	profileName := ""
	jsonOutput := false
	flags.StringVar(&configurationPath, "diagnosis-config", "", "override the per-user diagnosis configuration path")
	flags.StringVar(&profileName, "profile", "", "check a named profile instead of the configured default")
	flags.BoolVar(&jsonOutput, "json", false, "emit the versioned doctor result as JSON")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError(errors.New("doctor does not accept positional arguments"))
	}
	path, _, err := diagnosisconfig.ResolvePath(configurationPath)
	if err != nil {
		return err
	}
	configuration, err := diagnosisconfig.LoadFile(path)
	if err != nil {
		return err
	}
	selectedName, profile, err := configuration.SelectProfile(profileName)
	if err != nil {
		return err
	}
	generator, err := generation.NewGenerator(profile)
	if err != nil {
		report := doctor.SetupFailure(selectedName, profile)
		return writeDoctorResult(stdout, report, jsonOutput)
	}
	report, err := doctor.Run(ctx, selectedName, profile, generator)
	if err != nil {
		return err
	}

	return writeDoctorResult(stdout, report, jsonOutput)
}

func writeDoctorResult(stdout io.Writer, report doctor.Report, jsonOutput bool) error {
	if jsonOutput {
		if err := doctor.Encode(stdout, report); err != nil {
			return fmt.Errorf("write provider doctor result: %w", err)
		}
	} else {
		if err := doctor.WriteHuman(stdout, report); err != nil {
			return fmt.Errorf("write provider doctor result: %w", err)
		}
	}
	if !report.Ready {
		return errors.New("provider/model doctor found a failed readiness check")
	}

	return nil
}
