// Package cli implements the jobman-diagnose executable boundary.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/buildinfo"
	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/coreclient"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/generation"
	"github.com/ryancswallace/jobman-diagnose/internal/presentation"
	"github.com/ryancswallace/jobman-diagnose/internal/securefile"
	"github.com/ryancswallace/jobman-diagnose/internal/sourcecontext"
	"github.com/ryancswallace/jobman-diagnose/internal/supportbundle"
)

var errUsage = errors.New("invalid command usage")

const causalContextSearchBytes = 1024 * 1024

// ExitCode maps companion errors to stable process statuses.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		return 2
	case errors.Is(err, context.Canceled):
		return 130
	default:
		return 1
	}
}

// Run parses one invocation, obtains evidence, diagnoses it, and renders the
// result. It never calls os.Exit.
func Run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runWithEnvironment(arguments, stdin, stdout, stderr, defaultRuntimeEnvironment())
}

func runWithEnvironment(
	arguments []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	environment runtimeEnvironment,
) error {
	if handled, err := runInspectionCommand(arguments, stdout, stderr); handled {
		return commandLineError(err)
	}
	parsed, err := parse(arguments, stderr)
	if err != nil {
		return commandLineError(err)
	}
	if parsed.version {
		_, writeErr := fmt.Fprintf(
			stdout, "jobman-diagnose %s (evidence schema %d, report schema %d, configuration schema %d)\n",
			buildinfo.Version, diagnostic.SchemaVersion, diagnosis.SchemaVersion, diagnosisconfig.SchemaVersion,
		)
		return writeErr
	}
	selection, err := selectGenerator(parsed)
	if err != nil {
		return err
	}

	return runDiagnosis(parsed, selection, stdin, stdout, stderr, environment)
}

type generatorSelection struct {
	name         string
	profile      *diagnosisconfig.Profile
	approved     []string
	sourceMode   diagnosis.SourceContextMode
	sourceRadius uint64
}

func runInspectionCommand(arguments []string, stdout, stderr io.Writer) (bool, error) {
	if len(arguments) == 0 {
		return false, nil
	}
	switch arguments[0] {
	case "config":
		return true, runConfigCommand(arguments[1:], stdout, stderr)
	case "profiles":
		return true, runProfilesCommand(arguments[1:], stdout, stderr)
	case "doctor":
		return true, runDoctorCommand(arguments[1:], stdout, stderr)
	default:
		return false, nil
	}
}

func selectGenerator(parsed options) (generatorSelection, error) {
	if !parsed.aiEnabled() {
		return generatorSelection{}, nil
	}
	configurationPath, origin, err := diagnosisconfig.ResolvePath(parsed.diagnosisConfig)
	if err != nil {
		return generatorSelection{}, err
	}
	configuration, err := diagnosisconfig.LoadFile(configurationPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return generatorSelection{}, fmt.Errorf(
				"AI diagnosis requested, but no configuration was found at %q (%s); create it or use --diagnosis-config PATH: %w",
				configurationPath, origin, err,
			)
		}

		return generatorSelection{}, fmt.Errorf(
			"AI diagnosis requested but configuration from %s could not be loaded: %w", origin, err,
		)
	}
	name, profile, err := configuration.SelectProfile(parsed.profile)
	if err != nil {
		return generatorSelection{}, err
	}
	mode, radius, err := resolveSourceContext(parsed, profile)
	if err != nil {
		return generatorSelection{}, err
	}
	approved := slices.Clone(parsed.share)
	if mode != "" {
		if _, allowed := profile.Disclosure[string(diagnosis.DisclosureSourceContent)]; !allowed {
			return generatorSelection{}, fmt.Errorf(
				"AI source context requested, but profile %q does not allow source_content disclosure", name,
			)
		}
		approved = append(approved, string(diagnosis.DisclosureSourceContent))
	} else if slices.Contains(approved, string(diagnosis.DisclosureSourceContent)) {
		return generatorSelection{}, usageError(errors.New(
			"source_content sharing requires profile source_context or --ai-source limited|full",
		))
	}

	return generatorSelection{
		name: name, profile: &profile, approved: approved, sourceMode: mode, sourceRadius: radius,
	}, nil
}

func resolveSourceContext(parsed options, profile diagnosisconfig.Profile) (diagnosis.SourceContextMode, uint64, error) {
	mode := parsed.aiSource
	if mode == "" && profile.SourceContext != nil {
		mode = profile.SourceContext.Mode
	}
	if mode == diagnosisconfig.SourceContextModeNone {
		mode = ""
	}
	radius := uint64(0)
	if mode == string(diagnosis.SourceContextLimited) {
		radius = sourcecontext.DefaultLinesBeforeAndAfter
		if profile.SourceContext != nil && profile.SourceContext.Mode == diagnosisconfig.SourceContextModeLimited {
			radius = profile.SourceContext.LinesBeforeAndAfter
		}
	}
	if parsed.sourceFile != "" && mode == "" {
		return "", 0, usageError(errors.New(
			"--source-file requires source sharing from profile source_context or --ai-source limited|full",
		))
	}
	if parsed.sourceLine != 0 && mode != string(diagnosis.SourceContextLimited) {
		return "", 0, usageError(errors.New(
			"--source-line requires limited source sharing from profile source_context or --ai-source limited",
		))
	}

	return diagnosis.SourceContextMode(mode), radius, nil
}

//nolint:cyclop,gocognit // This command boundary owns acquisition, enrichment, optional generation, exports, and rendering.
func runDiagnosis(
	parsed options,
	selection generatorSelection,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	environment runtimeEnvironment,
) (resultErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	descriptor := progressDescriptor{}
	if selection.profile != nil {
		descriptor = progressDescriptor{
			profile: selection.name, model: selection.profile.Model, timeout: selection.profile.TimeoutDuration(),
		}
	}
	interactive := false
	if environment.interactive != nil {
		interactive = environment.interactive(stderr)
	}
	progress := newProgressReporter(
		parsed.progress, selection.profile != nil, parsed.jsonOutput, interactive,
		stderr, descriptor, environment.newProgressTiming,
	)
	progressClosed := false
	var progressErr error
	closeProgress := func() error {
		if !progressClosed {
			progressClosed = true
			progressErr = progress.Close()
		}

		return progressErr
	}
	defer func() {
		if err := closeProgress(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("write AI progress: %w", err)
		}
	}()
	progress.Phase(progressCollecting)
	expandCausalContextSearch(&parsed)
	evidence, err := obtainEvidence(ctx, parsed, stdin)
	if err != nil {
		return err
	}
	if parsed.exportEvidence != "" {
		if exportErr := securefile.WriteAtomic(parsed.exportEvidence, func(destination io.Writer) error {
			return diagnostic.Encode(destination, evidence)
		}); exportErr != nil {
			return fmt.Errorf("export evidence: %w", exportErr)
		}
	}
	progress.Phase(progressPreparing)
	failureEvidence, err := enrichment.Collect(ctx, evidence)
	if err != nil {
		return err
	}
	if selection.sourceMode != "" {
		limits := selection.profile.Disclosure[string(diagnosis.DisclosureSourceContent)]
		source, sourceErr := sourcecontext.Collect(ctx, evidence, sourcecontext.Options{
			Mode: selection.sourceMode, File: parsed.sourceFile, Line: parsed.sourceLine,
			LinesBeforeAndAfter: selection.sourceRadius, MaximumBytes: limits.MaximumBytes,
		})
		if sourceErr != nil {
			return sourceErr
		}
		failureEvidence, err = diagnosis.SealFailureEvidenceWithContext(
			evidence, failureEvidence.Enrichment, source,
		)
		if err != nil {
			return fmt.Errorf("attach source context: %w", err)
		}
	}
	deterministic, err := engine.New(buildinfo.Version, time.Now)
	if err != nil {
		return err
	}
	var diagnostician diagnosis.Diagnostician = deterministic
	if selection.profile != nil {
		generator, generatorErr := generation.NewGenerator(*selection.profile)
		if generatorErr != nil {
			return generatorErr
		}
		augmented, augmenterErr := generation.NewAugmenter(
			deterministic, generator, selection.name, *selection.profile, selection.approved, parsed.requireModel,
			generationProgressObserver(progress),
		)
		if augmenterErr != nil {
			return augmenterErr
		}
		diagnostician = augmented
	}
	report, err := diagnostician.Diagnose(ctx, failureEvidence)
	if err != nil {
		return err
	}
	if err := closeProgress(); err != nil {
		return fmt.Errorf("write AI progress: %w", err)
	}
	if parsed.supportBundle != "" {
		bundle, bundleErr := supportbundle.New(failureEvidence, report, supportbundle.Build{
			Version: buildinfo.Version, Commit: buildinfo.Commit, BuildDate: buildinfo.Date,
		})
		if bundleErr != nil {
			return bundleErr
		}
		if parsed.bundleDryRun {
			if parsed.jsonOutput {
				return supportbundle.EncodeInventory(stdout, bundle.Inventory)
			}
			return supportbundle.WriteInventory(stdout, bundle.Inventory)
		}
		if bundleErr := securefile.WriteAtomic(parsed.supportBundle, func(destination io.Writer) error {
			return supportbundle.Encode(destination, bundle)
		}); bundleErr != nil {
			return fmt.Errorf("write support bundle: %w", bundleErr)
		}
	}
	write := func(destination io.Writer) error {
		if parsed.jsonOutput {
			return diagnosis.Encode(destination, report)
		}
		if err := presentation.HumanWithOptions(destination, report, failureEvidence, presentation.HumanOptions{
			Details: parsed.details,
			Color:   colorEnabled(parsed.color, destination, environment),
		}); err != nil {
			return err
		}
		if parsed.supportBundle != "" {
			if _, err := fmt.Fprintf(destination, "\nSupport bundle: %s\n", parsed.supportBundle); err != nil {
				return fmt.Errorf("write support bundle location: %w", err)
			}
		}

		return nil
	}
	if parsed.output != "" {
		if err := securefile.WriteAtomic(parsed.output, write); err != nil {
			return fmt.Errorf("write diagnosis report: %w", err)
		}

		return nil
	}

	return write(stdout)
}

func commandLineError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	return err
}

type options struct {
	selector         string
	fromEvidence     string
	exportEvidence   string
	output           string
	jobman           string
	stateDir         string
	configPath       string
	request          diagnostic.EvidenceRequest
	jsonOutput       bool
	details          bool
	color            colorMode
	version          bool
	ai               bool
	aiLogs           bool
	aiSource         string
	sourceFile       string
	sourceLine       uint64
	profile          string
	requireModel     bool
	deterministic    bool
	progress         progressMode
	diagnosisConfig  string
	supportBundle    string
	bundleDryRun     bool
	share            stringListValue
	logsExplicit     bool
	logBytesExplicit bool
	includeSystem    bool
}

func (parsed options) aiEnabled() bool {
	return parsed.ai || parsed.aiLogs || parsed.aiSource != "" || parsed.profile != ""
}

func parse(arguments []string, stderr io.Writer) (options, error) {
	parsed := options{
		request:  diagnostic.EvidenceRequest{Logs: diagnostic.LogsMetadata},
		progress: progressAuto,
		color:    colorAuto,
	}
	flags := flag.NewFlagSet("jobman-diagnose", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var usageWriteErr error
	flags.Usage = func() {
		_, usageWriteErr = fmt.Fprintln(stderr, "usage: jobman-diagnose [options] JOB\n       jobman-diagnose [options] --from-evidence PATH\n       jobman-diagnose config {paths|validate|show} [PATH]\n       jobman-diagnose profiles [--diagnosis-config PATH]\n       jobman-diagnose doctor [--diagnosis-config PATH] [--profile NAME] [--json]")
		flags.PrintDefaults()
	}
	registerFlags(flags, &parsed)
	if err := flags.Parse(arguments); err != nil {
		if usageWriteErr != nil {
			return parsed, fmt.Errorf("%w: write usage: %w", errUsage, usageWriteErr)
		}
		if errors.Is(err, flag.ErrHelp) {
			return parsed, flag.ErrHelp
		}
		return parsed, fmt.Errorf("%w: %w", errUsage, err)
	}
	if parsed.version {
		if flags.NArg() != 0 {
			return parsed, usageError(errors.New("--version does not accept a job selector"))
		}

		return parsed, nil
	}
	if flags.NArg() > 1 {
		return parsed, usageError(errors.New("accepts at most one job selector"))
	}
	if flags.NArg() == 1 {
		parsed.selector = flags.Arg(0)
	}
	parsed.request.Selector = parsed.selector
	flags.Visit(func(value *flag.Flag) {
		switch value.Name {
		case "logs":
			parsed.logsExplicit = true
		case "log-bytes":
			parsed.logBytesExplicit = true
		}
	})
	if err := normalizeAIOptions(&parsed); err != nil {
		return parsed, err
	}
	if err := validateOptions(parsed); err != nil {
		return parsed, err
	}

	return parsed, nil
}

func expandCausalContextSearch(parsed *options) {
	if parsed == nil || parsed.fromEvidence != "" || parsed.request.Logs != diagnostic.LogsTail ||
		parsed.logBytesExplicit || !parsed.aiEnabled() ||
		!slices.Contains(parsed.share, string(diagnostic.DisclosureLogContent)) {
		return
	}
	parsed.request.LogBytes = max(parsed.request.LogBytes, causalContextSearchBytes)
}

func registerFlags(flags *flag.FlagSet, parsed *options) {
	flags.Int64Var(&parsed.request.Run, "run", 0, "select a run number or negative index")
	flags.BoolVar(&parsed.request.AllRuns, "all-runs", false, "compare the bounded run history")
	flags.Var((*logModeValue)(&parsed.request.Logs), "logs", "collect logs as metadata, tail, or none")
	flags.Var((*byteSizeValue)(&parsed.request.LogBytes), "log-bytes", "maximum bytes per selected log stream")
	flags.Uint64Var(&parsed.request.Similar, "similar", 0, "request up to N same-fingerprint histories")
	flags.StringVar(&parsed.fromEvidence, "from-evidence", "", "diagnose a saved evidence value or core envelope ('-' for stdin)")
	flags.StringVar(&parsed.exportEvidence, "export-evidence", "", "write sealed core evidence to a new private file")
	flags.StringVar(&parsed.output, "output", "", "write the report to a new private file")
	flags.StringVar(&parsed.supportBundle, "support-bundle", "", "write a private deterministic support archive")
	flags.BoolVar(&parsed.bundleDryRun, "bundle-dry-run", false, "list support archive contents without creating it")
	flags.StringVar(&parsed.jobman, "jobman", "", "path to the core jobman executable")
	flags.StringVar(&parsed.stateDir, "state-dir", "", "pass an explicit core state directory")
	flags.StringVar(&parsed.configPath, "config", "", "pass an explicit core redaction configuration")
	flags.BoolVar(&parsed.jsonOutput, "json", false, "emit the versioned diagnosis report as JSON")
	flags.BoolVar(&parsed.details, "details", false, "include all evidence and technical provenance in human output")
	flags.Var((*colorModeValue)(&parsed.color), "color", "color human output as auto, always, or never")
	flags.BoolVar(&parsed.version, "version", false, "print version and supported schemas")
	flags.BoolVar(&parsed.deterministic, "deterministic", false, "force local deterministic analysis (the default)")
	flags.BoolVar(&parsed.ai, "ai", false, "use the default AI profile and share bounded execution context")
	flags.BoolVar(&parsed.ai, "a", false, "short form of --ai")
	flags.BoolVar(&parsed.aiLogs, "ai-logs", false, "use AI and share a bounded redacted target-log tail")
	flags.StringVar(&parsed.aiSource, "ai-source", "", "override profile source sharing as none, limited, or full")
	flags.StringVar(&parsed.sourceFile, "source-file", "", "current source file to share instead of inferring it from the target command")
	flags.Uint64Var(&parsed.sourceLine, "source-line", 0, "line around which to select limited source context")
	flags.BoolVar(&parsed.includeSystem, "system", false,
		"collect bounded point-in-time filesystem and cgroup constraints")
	flags.StringVar(&parsed.diagnosisConfig, "diagnosis-config", "", "override the per-user diagnosis configuration path")
	flags.StringVar(&parsed.profile, "profile", "", "use a named AI profile instead of the configured default")
	flags.Var(&parsed.share, "share", "approve an additional disclosure class")
	flags.BoolVar(&parsed.requireModel, "require-model", false, "fail rather than degrade when generated analysis is unavailable")
	flags.Var((*progressModeValue)(&parsed.progress), "progress", "show AI progress as auto, plain, or off")
}

func normalizeAIOptions(parsed *options) error {
	if parsed == nil || !parsed.aiEnabled() {
		return nil
	}
	parsed.share = append(parsed.share, string(diagnostic.DisclosureMetadata))
	parsed.share = append(parsed.share, string(diagnostic.DisclosureCommand))
	parsed.share = append(parsed.share, string(diagnostic.DisclosurePath))
	parsed.share = append(parsed.share, string(diagnostic.DisclosureEnvironmentName))
	if parsed.fromEvidence == "" {
		parsed.request.IncludeCommand = true
		parsed.request.IncludePaths = true
		parsed.request.IncludeEnvironmentNames = true
		parsed.includeSystem = true
	}
	if parsed.aiLogs {
		parsed.share = append(parsed.share, string(diagnostic.DisclosureLogContent))
	}
	if parsed.aiSource != "" {
		parsed.aiSource = strings.ToLower(strings.TrimSpace(parsed.aiSource))
		if parsed.aiSource != diagnosisconfig.SourceContextModeNone {
			parsed.share = append(parsed.share, string(diagnosis.DisclosureSourceContent))
		}
	}
	if !slices.Contains(parsed.share, string(diagnostic.DisclosureLogContent)) || parsed.fromEvidence != "" {
		return nil
	}
	if parsed.logsExplicit && parsed.request.Logs != diagnostic.LogsTail {
		return usageError(errors.New("log_content sharing conflicts with an explicit --logs value other than tail"))
	}
	parsed.request.Logs = diagnostic.LogsTail

	return nil
}

//nolint:cyclop,gocognit // CLI option relationships are kept together as the user-facing contract matrix.
func validateOptions(parsed options) error {
	if (parsed.selector == "") == (parsed.fromEvidence == "") {
		return usageError(errors.New("provide exactly one JOB or --from-evidence PATH"))
	}
	if parsed.request.Run != 0 && parsed.request.AllRuns {
		return usageError(errors.New("--run and --all-runs are mutually exclusive"))
	}
	if parsed.request.Logs != diagnostic.LogsTail && parsed.request.LogBytes != 0 {
		return usageError(errors.New("--log-bytes requires --logs tail"))
	}
	if parsed.fromEvidence != "" && hasLiveCollectionOptions(parsed) {
		return usageError(errors.New("live collection options cannot be combined with --from-evidence"))
	}
	if parsed.deterministic && (parsed.aiEnabled() || parsed.requireModel || len(parsed.share) != 0) {
		return usageError(errors.New("--deterministic cannot be combined with AI options"))
	}
	if parsed.details && parsed.jsonOutput {
		return usageError(errors.New("--details cannot be combined with --json"))
	}
	if parsed.requireModel && !parsed.aiEnabled() {
		return usageError(errors.New("--require-model requires --ai, --ai-logs, --ai-source, or --profile"))
	}
	if !parsed.aiEnabled() && len(parsed.share) != 0 {
		return usageError(errors.New("--share requires --ai, --ai-logs, --ai-source, or --profile"))
	}
	if parsed.aiSource != "" && parsed.aiSource != diagnosisconfig.SourceContextModeNone &&
		parsed.aiSource != string(diagnosis.SourceContextLimited) &&
		parsed.aiSource != string(diagnosis.SourceContextFull) {
		return usageError(errors.New("--ai-source must be none, limited, or full"))
	}
	if parsed.sourceFile != "" && !parsed.aiEnabled() {
		return usageError(errors.New("--source-file requires AI diagnosis"))
	}
	if parsed.sourceLine != 0 && !parsed.aiEnabled() {
		return usageError(errors.New("--source-line requires AI diagnosis"))
	}
	if parsed.sourceLine != 0 && parsed.aiSource != "" &&
		parsed.aiSource != string(diagnosis.SourceContextLimited) {
		return usageError(errors.New("--source-line requires --ai-source limited"))
	}
	if parsed.bundleDryRun && parsed.supportBundle == "" {
		return usageError(errors.New("--bundle-dry-run requires --support-bundle PATH"))
	}
	if parsed.bundleDryRun && (parsed.output != "" || parsed.exportEvidence != "") {
		return usageError(errors.New("--bundle-dry-run cannot create --output or --export-evidence files"))
	}
	if parsed.supportBundle != "" && (parsed.supportBundle == parsed.output || parsed.supportBundle == parsed.exportEvidence) {
		return usageError(errors.New("support bundle, report, and evidence paths must be distinct"))
	}

	return nil
}

func hasLiveCollectionOptions(parsed options) bool {
	return parsed.jobman != "" || parsed.stateDir != "" || parsed.configPath != "" ||
		parsed.request.Run != 0 || parsed.request.AllRuns || parsed.request.Similar != 0 ||
		parsed.request.IncludeCommand || parsed.request.IncludePaths || parsed.request.IncludeEnvironmentNames ||
		parsed.includeSystem ||
		parsed.request.Logs != diagnostic.LogsMetadata || parsed.request.LogBytes != 0
}

func obtainEvidence(ctx context.Context, options options, stdin io.Reader) (diagnostic.Evidence, error) {
	if options.fromEvidence != "" {
		if options.fromEvidence == "-" {
			return coreclient.DecodeEvidence(stdin)
		}
		file, err := os.Open(options.fromEvidence)
		if err != nil {
			return diagnostic.Evidence{}, fmt.Errorf("open evidence: %w", err)
		}
		evidence, decodeErr := coreclient.DecodeEvidence(file)
		closeErr := file.Close()
		if err := errors.Join(decodeErr, closeErr); err != nil {
			return diagnostic.Evidence{}, fmt.Errorf("read evidence: %w", err)
		}

		return evidence, nil
	}
	client, err := coreclient.New(coreclient.Options{
		Executable: options.jobman, StateDir: options.stateDir, ConfigPath: options.configPath,
		IncludeSystem: options.includeSystem,
	})
	if err != nil {
		return diagnostic.Evidence{}, err
	}

	return client.Collect(ctx, options.request)
}

func usageError(err error) error { return fmt.Errorf("%w: %w", errUsage, err) }

type logModeValue diagnostic.LogMode

func (value *logModeValue) Set(encoded string) error {
	mode := diagnostic.LogMode(strings.ToLower(strings.TrimSpace(encoded)))
	switch mode {
	case diagnostic.LogsMetadata, diagnostic.LogsTail, diagnostic.LogsNone:
		*value = logModeValue(mode)
		return nil
	default:
		return errors.New("must be metadata, tail, or none")
	}
}

func (value *logModeValue) String() string {
	if value == nil {
		return ""
	}

	return string(*value)
}

type byteSizeValue uint64

func (value *byteSizeValue) Set(encoded string) error {
	parsed, err := parseByteSize(encoded)
	if err != nil {
		return err
	}
	*value = byteSizeValue(parsed)

	return nil
}

func (value *byteSizeValue) String() string {
	if value == nil {
		return "0B"
	}

	return strconv.FormatUint(uint64(*value), 10) + "B"
}

func parseByteSize(encoded string) (uint64, error) {
	value := strings.TrimSpace(encoded)
	multipliers := []struct {
		suffix     string
		multiplier uint64
	}{{"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}}
	for _, candidate := range multipliers {
		if !strings.HasSuffix(value, candidate.suffix) {
			continue
		}
		number := strings.TrimSpace(strings.TrimSuffix(value, candidate.suffix))
		parsed, err := strconv.ParseUint(number, 10, 64)
		if err != nil || parsed > ^uint64(0)/candidate.multiplier {
			return 0, fmt.Errorf("invalid byte size %q", encoded)
		}

		return parsed * candidate.multiplier, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q", encoded)
	}

	return parsed, nil
}

type stringListValue []string

func (value *stringListValue) Set(encoded string) error {
	for _, class := range strings.Split(encoded, ",") {
		class = strings.TrimSpace(class)
		if class != string(diagnostic.DisclosureMetadata) && class != string(diagnostic.DisclosureCommand) &&
			class != string(diagnostic.DisclosurePath) && class != string(diagnostic.DisclosureEnvironmentName) &&
			class != string(diagnostic.DisclosureLogContent) && class != string(diagnosis.DisclosureSourceContent) {
			return errors.New("must be metadata, command, path, environment_name, log_content, or source_content")
		}
		*value = append(*value, class)
	}

	return nil
}

func (value *stringListValue) String() string {
	if value == nil {
		return ""
	}

	return strings.Join(*value, ",")
}
