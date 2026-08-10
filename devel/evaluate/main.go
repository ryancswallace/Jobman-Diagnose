// Command evaluate runs Jobman Diagnose's checked-in quality corpus. Live
// provider use requires the explicit --live flag and an explicit config path.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/buildinfo"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/evaluation"
	"github.com/ryancswallace/jobman-diagnose/internal/generation"
	"github.com/ryancswallace/jobman-diagnose/internal/securefile"
)

type options struct {
	corpus          string
	live            bool
	diagnosisConfig string
	profile         string
	share           string
	allowFallback   bool
	output          string
	summary         bool
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(arguments []string, stdout, stderr io.Writer) int {
	if err := run(arguments, stdout, stderr); err != nil {
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

//nolint:cyclop,gocognit // The development command keeps mode selection, evaluation, output, and gate status in one auditable flow.
func run(arguments []string, stdout, stderr io.Writer) error {
	parsed, err := parse(arguments, stderr)
	if err != nil {
		return err
	}
	corpus, err := evaluation.Load(parsed.corpus)
	if err != nil {
		return err
	}
	clock := func() time.Time { return time.Now().UTC() }
	deterministic, err := engine.New(buildinfo.Version, clock)
	if err != nil {
		return err
	}
	var diagnostician diagnosis.Diagnostician = deterministic
	mode := "deterministic"
	if parsed.live {
		configuration, loadErr := config.LoadFile(parsed.diagnosisConfig)
		if loadErr != nil {
			return fmt.Errorf("load live evaluation configuration: %w", loadErr)
		}
		profileName, profile, selectErr := configuration.SelectProfile(parsed.profile)
		if selectErr != nil {
			return selectErr
		}
		generator, generatorErr := generation.NewGenerator(profile)
		if generatorErr != nil {
			return generatorErr
		}
		approved, approvalErr := parseShare(parsed.share)
		if approvalErr != nil {
			return approvalErr
		}
		augmenter, augmenterErr := generation.NewAugmenter(
			deterministic, generator, profileName, profile, approved, !parsed.allowFallback, nil,
		)
		if augmenterErr != nil {
			return augmenterErr
		}
		diagnostician = augmenter
		mode = "live:" + profileName
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	summary, err := evaluation.Run(ctx, corpus, diagnostician, mode)
	if err != nil {
		return err
	}
	write := func(destination io.Writer) error {
		encoder := json.NewEncoder(destination)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}
	if parsed.output != "" {
		if err := securefile.WriteAtomic(parsed.output, write); err != nil {
			return fmt.Errorf("write evaluation report: %w", err)
		}
	} else if parsed.summary {
		if _, err := fmt.Fprintf(
			stdout, "evaluation: %d/%d cases passed; primary %.3f, citations %.3f, actions %.3f, retry %.3f, unsupported %.3f\n",
			summary.Passed, summary.Cases, summary.Metrics.PrimaryCodePrecision,
			summary.Metrics.CitationValidity, summary.Metrics.SafeActionRate,
			summary.Metrics.RetryAdviceAccuracy, summary.Metrics.UnsupportedClaimRate,
		); err != nil {
			return fmt.Errorf("write evaluation summary: %w", err)
		}
	} else if err := write(stdout); err != nil {
		return fmt.Errorf("write evaluation report: %w", err)
	}
	if summary.Passed != summary.Cases {
		return fmt.Errorf("evaluation failed: %d of %d cases passed", summary.Passed, summary.Cases)
	}

	return nil
}

func parse(arguments []string, stderr io.Writer) (options, error) {
	parsed := options{corpus: "testdata/evaluation/manifest.json", share: "metadata"}
	flags := flag.NewFlagSet("jobman-diagnose-evaluate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&parsed.corpus, "corpus", parsed.corpus, "path to the versioned evaluation corpus")
	flags.BoolVar(&parsed.live, "live", false, "invoke an explicitly configured provider")
	flags.StringVar(&parsed.diagnosisConfig, "diagnosis-config", "", "explicit diagnosis.yml path for live evaluation")
	flags.StringVar(&parsed.profile, "profile", "", "named live profile (the configured default when omitted)")
	flags.StringVar(&parsed.share, "share", parsed.share, "comma-separated approved disclosure classes")
	flags.BoolVar(&parsed.allowFallback, "allow-fallback", false, "allow provider failures to use deterministic fallback")
	flags.StringVar(&parsed.output, "output", "", "write the evaluation result to a new private file")
	flags.BoolVar(&parsed.summary, "summary", false, "print one concise metric line instead of result JSON")
	if err := flags.Parse(arguments); err != nil {
		return parsed, err
	}
	if flags.NArg() != 0 || strings.TrimSpace(parsed.corpus) == "" {
		return parsed, errors.New("evaluation accepts flags only and requires a corpus path")
	}
	if parsed.live && strings.TrimSpace(parsed.diagnosisConfig) == "" {
		return parsed, errors.New("--live requires an explicit --diagnosis-config path")
	}
	if !parsed.live && (parsed.diagnosisConfig != "" || parsed.profile != "" || parsed.allowFallback || parsed.share != "metadata") {
		return parsed, errors.New("provider options require --live")
	}

	return parsed, nil
}

func parseShare(value string) ([]string, error) {
	values := strings.Split(value, ",")
	result := make([]string, 0, len(values))
	allowed := map[string]struct{}{
		"metadata": {}, "command": {}, "path": {}, "environment_name": {}, "log_content": {},
	}
	for _, current := range values {
		current = strings.TrimSpace(current)
		if _, ok := allowed[current]; !ok {
			return nil, fmt.Errorf("unsupported disclosure class %q", current)
		}
		if !slices.Contains(result, current) {
			result = append(result, current)
		}
	}
	if !slices.Contains(result, "metadata") {
		return nil, errors.New("live evaluation requires metadata disclosure")
	}

	return result, nil
}
