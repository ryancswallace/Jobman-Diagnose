// Package sourcecontext collects explicitly approved, point-in-time source
// snapshots for generated diagnosis. Source text is untrusted supplemental
// context and is never represented as core Jobman evidence.
package sourcecontext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

const (
	collectorName    = "companion.source_context"
	collectorVersion = "1"
	hardMaximumBytes = 1024 * 1024
	// DefaultLinesBeforeAndAfter preserves the CLI's original limited window
	// when a profile does not select a different symmetric radius.
	DefaultLinesBeforeAndAfter = 20
	maximumLinesBeforeAndAfter = 1024 * 1024
)

var (
	pythonLocation = regexp.MustCompile(`File\s+"([^"]+)",\s+line\s+([0-9]+)`)
	pathLocation   = regexp.MustCompile(`(?m)([^\s"'():]+\.[A-Za-z0-9]+):([0-9]+)(?::[0-9]+)?`)
	frameLocation  = regexp.MustCompile(`\(([^\s():]+\.[A-Za-z0-9]+):([0-9]+)\)`)
)

// Options specifies one explicit source disclosure. File may be omitted when
// exactly one source-looking argument can be inferred from the recorded direct
// target command. MaximumBytes is the selected profile's source_content cap.
type Options struct {
	Mode diagnosis.SourceContextMode
	File string
	Line uint64
	// LinesBeforeAndAfter sets the symmetric limited-source radius. Zero uses
	// DefaultLinesBeforeAndAfter for backward compatibility.
	LinesBeforeAndAfter uint64
	// MaximumArtifacts bounds automatic multi-file collection. Zero preserves
	// the historical single-file behavior. Explicit File and full-file mode
	// always select exactly one source artifact.
	MaximumArtifacts uint64
	MaximumBytes     uint64
}

// Collect reads bounded regular source files and returns immutable point-in-time
// snapshots. Limited automatic collection starts with direct command sources,
// then adds runtime-referenced files under the recorded working directory.
// Explicit and full-file collection remain single-file operations.
func Collect(ctx context.Context, evidence diagnostic.Evidence, options Options) ([]diagnosis.SourceContext, error) {
	if err := validateCollection(ctx, evidence, options); err != nil {
		return nil, err
	}
	paths, err := resolveSourcePaths(evidence, options)
	if err != nil {
		return nil, err
	}
	contexts := make([]diagnosis.SourceContext, 0, len(paths))
	var selectedBytes uint64
	for _, path := range paths {
		language, mediaType, ok := sourceType(path)
		if !ok {
			return nil, fmt.Errorf("collect source context: %q does not have a supported source-file extension", path)
		}
		data, capturedAt, readErr := readSource(ctx, path)
		if readErr != nil {
			return nil, readErr
		}
		selection, selectionErr := selectSourceContext(path, data, evidence.Artifacts, options)
		if selectionErr != nil {
			return nil, selectionErr
		}
		selected := slices.Clone(data[selection.byteStart:selection.byteEnd])
		if selectedBytes > options.MaximumBytes || uint64(len(selected)) > options.MaximumBytes-selectedBytes {
			if options.File == "" && options.Mode == diagnosis.SourceContextLimited && len(contexts) != 0 {
				continue
			}
			return nil, fmt.Errorf(
				"collect source context: selected %s contexts require %d bytes but the profile allows %d",
				options.Mode, selectedBytes+uint64(len(selected)), options.MaximumBytes,
			)
		}
		selectedBytes += uint64(len(selected))
		context := diagnosis.SourceContext{
			ID: fmt.Sprintf("context:source:%03d", len(contexts)+1), Role: "source.context", Path: path,
			Language: language, MediaType: mediaType, Mode: options.Mode,
			AnchorLine: selection.anchor, AnchorReason: selection.reason,
			StartLine: selection.startLine, EndLine: selection.endLine,
			TotalLines: selection.totalLines, ByteStart: selection.byteStart, ByteEnd: selection.byteEnd,
			FileBytes: uint64(len(data)), ContentBytes: uint64(len(selected)), Data: selected,
			Digest: digest(data), ContentDigest: digest(selected), CapturedAt: capturedAt,
			Collector: diagnosis.AnalyzerDescriptor{Name: collectorName, Version: collectorVersion},
			Quality:   diagnostic.QualityPointInTime, Disclosure: diagnosis.DisclosureSourceContent,
		}
		if options.Mode == diagnosis.SourceContextFull {
			context.AnchorLine = 0
			context.AnchorReason = "full_file"
		}
		contexts = append(contexts, context)
	}

	return contexts, nil
}

func validateCollection(ctx context.Context, evidence diagnostic.Evidence, options Options) error {
	if ctx == nil {
		return errors.New("collect source context: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("collect source context: %w", err)
	}
	if err := diagnostic.Verify(evidence); err != nil {
		return fmt.Errorf("collect source context: verify core evidence: %w", err)
	}
	if options.Mode != diagnosis.SourceContextLimited && options.Mode != diagnosis.SourceContextFull {
		return errors.New("collect source context: mode must be limited or full")
	}
	if options.MaximumBytes == 0 || options.MaximumBytes > hardMaximumBytes {
		return fmt.Errorf("collect source context: maximum bytes must be between 1 and %d", hardMaximumBytes)
	}
	if options.MaximumArtifacts > 16 {
		return errors.New("collect source context: maximum artifacts must not exceed 16")
	}
	if options.Mode == diagnosis.SourceContextFull && (options.Line != 0 || options.LinesBeforeAndAfter != 0) {
		return errors.New("collect source context: a source line or context radius is only valid in limited mode")
	}
	if options.LinesBeforeAndAfter > maximumLinesBeforeAndAfter {
		return fmt.Errorf(
			"collect source context: lines before and after must not exceed %d", maximumLinesBeforeAndAfter,
		)
	}

	return nil
}

type sourceSelection struct {
	anchor, startLine, endLine, totalLines uint64
	byteStart, byteEnd                     uint64
	reason                                 string
}

func selectSourceContext(
	path string,
	data []byte,
	artifacts []diagnostic.Artifact,
	options Options,
) (sourceSelection, error) {
	lineStarts := sourceLineStarts(data)
	selection := sourceSelection{
		anchor: options.Line, reason: "explicit_line", startLine: 1,
		endLine: uint64(len(lineStarts)), totalLines: uint64(len(lineStarts)), byteEnd: uint64(len(data)),
	}
	if options.Mode == diagnosis.SourceContextFull {
		selection.anchor = 0
		selection.reason = "full_file"
		return selection, nil
	}
	if selection.anchor == 0 {
		if inferred, found := inferSourceLine(path, artifacts); found {
			selection.anchor, selection.reason = inferred, "runtime_log"
		} else {
			selection.anchor, selection.reason = 1, "file_start"
		}
	}
	if selection.anchor > selection.totalLines {
		if options.Line != 0 {
			return sourceSelection{}, fmt.Errorf(
				"collect source context: line %d is outside %q (1-%d)", selection.anchor, path, len(lineStarts),
			)
		}
		// Retain a bounded current snapshot so later source assessment can
		// classify and withhold stale source instead of aborting diagnosis before
		// it can emit the controlled mismatch warning.
		selection.anchor = selection.totalLines
		selection.reason = "runtime_log_clipped"
	}
	radius := options.LinesBeforeAndAfter
	if radius == 0 {
		radius = DefaultLinesBeforeAndAfter
	}
	if selection.anchor > radius {
		selection.startLine = selection.anchor - radius
	}
	if radius < selection.totalLines-selection.anchor {
		selection.endLine = selection.anchor + radius
	}
	selection.byteStart = lineStarts[selection.startLine-1]
	if selection.endLine < selection.totalLines {
		selection.byteEnd = lineStarts[selection.endLine]
	}

	return selection, nil
}

func resolveSourcePaths(evidence diagnostic.Evidence, options Options) ([]string, error) {
	if options.File != "" {
		return resolveExplicitSourcePath(options.File)
	}
	command, found := targetCommand(evidence.Items)
	if !found {
		return nil, errors.New("collect source context: target command is unavailable; use --source-file PATH")
	}
	candidates := sourceCandidates(command)
	maximumArtifacts := sourceArtifactLimit(options)
	if len(candidates) > 1 && maximumArtifacts == 1 {
		return nil, fmt.Errorf(
			"collect source context: multiple source files appear in the direct target command (%s); use --source-file PATH",
			strings.Join(candidates, ", "),
		)
	}
	workingDirectory, hasWorkingDirectory := targetWorkingDirectory(evidence.Items)
	paths := make([]string, 0, maximumArtifacts)
	for _, candidate := range candidates {
		paths = appendSourcePath(paths, candidate, workingDirectory, hasWorkingDirectory, maximumArtifacts)
	}
	if options.Mode == diagnosis.SourceContextLimited && maximumArtifacts > uint64(len(paths)) && hasWorkingDirectory {
		paths = appendRuntimeSourcePaths(paths, evidence.Artifacts, workingDirectory, maximumArtifacts)
	}
	if len(paths) == 0 {
		if !hasWorkingDirectory {
			return nil, errors.New("collect source context: target working directory is unavailable; use --source-file PATH")
		}
		return nil, errors.New("collect source context: no source file could be inferred from the direct target command or runtime locations; use --source-file PATH")
	}

	return paths, nil
}

func resolveExplicitSourcePath(file string) ([]string, error) {
	path, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("collect source context: resolve explicit source file: %w", err)
	}

	return []string{filepath.Clean(path)}, nil
}

func sourceArtifactLimit(options Options) uint64 {
	if options.Mode == diagnosis.SourceContextFull || options.Line != 0 {
		return 1
	}
	if options.MaximumArtifacts == 0 {
		return 1
	}

	return options.MaximumArtifacts
}

func appendSourcePath(
	paths []string,
	candidate string,
	workingDirectory string,
	hasWorkingDirectory bool,
	maximumArtifacts uint64,
) []string {
	if uint64(len(paths)) >= maximumArtifacts {
		return paths
	}
	path := candidate
	if !filepath.IsAbs(path) {
		if !hasWorkingDirectory {
			return paths
		}
		path = filepath.Join(workingDirectory, path)
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || slices.Contains(paths, path) {
		return paths
	}

	return append(paths, path)
}

func appendRuntimeSourcePaths(
	paths []string,
	artifacts []diagnostic.Artifact,
	workingDirectory string,
	maximumArtifacts uint64,
) []string {
	locations := sourceLocations(artifacts)
	for index := len(locations) - 1; index >= 0; index-- {
		path, ok := safeRuntimeSourcePath(workingDirectory, locations[index].path)
		if ok {
			paths = appendSourcePath(paths, path, workingDirectory, true, maximumArtifacts)
		}
	}

	return paths
}

func safeRuntimeSourcePath(workingDirectory, reference string) (string, bool) {
	if _, _, ok := sourceType(reference); !ok {
		return "", false
	}
	root := filepath.Clean(workingDirectory)
	path := filepath.Clean(reference)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(path) || !pathWithinRoot(root, path) {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil || !pathWithinRoot(resolvedRoot, resolvedPath) {
		return "", false
	}

	return path, true
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func targetCommand(items []diagnostic.Item) (diagnostic.Command, bool) {
	for _, item := range items {
		if item.Code != diagnostic.CodeTargetCommand {
			continue
		}
		var command diagnostic.Command
		if json.Unmarshal(item.Value, &command) == nil && command.Validate() == nil {
			return command, true
		}
	}

	return diagnostic.Command{}, false
}

func targetWorkingDirectory(items []diagnostic.Item) (string, bool) {
	for _, item := range items {
		if item.Code != diagnostic.CodeTargetWorkingDirectory {
			continue
		}
		var path string
		if json.Unmarshal(item.Value, &path) == nil && filepath.IsAbs(path) {
			return filepath.Clean(path), true
		}
	}

	return "", false
}

func sourceCandidates(command diagnostic.Command) []string {
	values := append([]string{command.Executable}, command.Arguments...)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.ContainsAny(value, " \t\r\n\x00") {
			continue
		}
		if _, _, ok := sourceType(value); !ok || slices.Contains(result, value) {
			continue
		}
		result = append(result, value)
	}

	return result
}

func readSource(ctx context.Context, path string) ([]byte, time.Time, error) {
	before, err := inspectSource(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	file, err := openStableSource(path, before)
	if err != nil {
		return nil, time.Time{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, hardMaximumBytes+1))
	afterRead, afterReadErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(readErr, afterReadErr, closeErr); err != nil {
		return nil, time.Time{}, fmt.Errorf("collect source context: read %q: %w", path, err)
	}
	pathAfterRead, lstatErr := os.Lstat(path)
	if lstatErr != nil || !stableSourceRead(before, afterRead, pathAfterRead, data) {
		return nil, time.Time{}, fmt.Errorf("collect source context: %q changed while it was being read", path)
	}
	if len(data) == 0 || len(data) > hardMaximumBytes {
		return nil, time.Time{}, fmt.Errorf(
			"collect source context: %q must contain between 1 and %d bytes", path, hardMaximumBytes,
		)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, time.Time{}, fmt.Errorf("collect source context: %q must contain NUL-free UTF-8 text", path)
	}
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("collect source context: %w", err)
	}

	return data, time.Now().UTC().Round(0), nil
}

func inspectSource(path string) (os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("collect source context: inspect %q: %w", path, err)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("collect source context: %q must be a regular, non-symlink file", path)
	}
	if before.Size() <= 0 || before.Size() > hardMaximumBytes {
		return nil, fmt.Errorf(
			"collect source context: %q must contain between 1 and %d bytes", path, hardMaximumBytes,
		)
	}

	return before, nil
}

func openStableSource(path string, before os.FileInfo) (*os.File, error) {
	// #nosec G304 -- reading this path is the explicit purpose of --source-file
	// or the bounded, single-file inference policy above.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("collect source context: open %q: %w", path, err)
	}
	afterOpen, statErr := file.Stat()
	if statErr != nil || !afterOpen.Mode().IsRegular() || !os.SameFile(before, afterOpen) {
		identityErr := errors.New("source file identity changed")
		if statErr != nil {
			identityErr = statErr
		}
		return nil, fmt.Errorf(
			"collect source context: %q changed while it was being opened: %w",
			path, errors.Join(identityErr, file.Close()),
		)
	}

	return file, nil
}

func stableSourceRead(before, afterRead, pathAfterRead os.FileInfo, data []byte) bool {
	return os.SameFile(before, afterRead) && os.SameFile(afterRead, pathAfterRead) &&
		afterRead.Size() == int64(len(data)) && afterRead.ModTime().Equal(before.ModTime())
}

func sourceLineStarts(data []byte) []uint64 {
	starts := []uint64{0}
	var offset uint64
	for _, value := range data {
		offset++
		if value == '\n' && offset < uint64(len(data)) {
			starts = append(starts, offset)
		}
	}

	return starts
}

func inferSourceLine(sourcePath string, artifacts []diagnostic.Artifact) (uint64, bool) {
	var selected uint64
	for _, location := range sourceLocations(artifacts) {
		if sameSourcePath(sourcePath, location.path) {
			selected = location.line
		}
	}

	return selected, selected != 0
}

func sameSourcePath(sourcePath, reference string) bool {
	reference = filepath.Clean(reference)
	if filepath.IsAbs(reference) {
		return reference == sourcePath
	}

	return filepath.Base(reference) == filepath.Base(sourcePath)
}

func sourceType(path string) (string, string, bool) {
	extension := strings.ToLower(filepath.Ext(path))
	value, ok := map[string][2]string{
		".py": {"python", "text/x-python"}, ".pyw": {"python", "text/x-python"},
		".go": {"go", "text/x-go"}, ".js": {"javascript", "text/javascript"},
		".mjs": {"javascript", "text/javascript"}, ".cjs": {"javascript", "text/javascript"},
		".jsx": {"javascript", "text/jsx"}, ".ts": {"typescript", "text/typescript"},
		".tsx": {"typescript", "text/tsx"}, ".sh": {"shell", "text/x-shellscript"},
		".bash": {"shell", "text/x-shellscript"}, ".zsh": {"shell", "text/x-shellscript"},
		".c": {"c", "text/x-c"}, ".h": {"c", "text/x-c"},
		".cc": {"cpp", "text/x-c++"}, ".cpp": {"cpp", "text/x-c++"},
		".cxx": {"cpp", "text/x-c++"}, ".hpp": {"cpp", "text/x-c++"},
		".java": {"java", "text/x-java"}, ".rs": {"rust", "text/x-rust"},
		".rb": {"ruby", "text/x-ruby"}, ".pl": {"perl", "text/x-perl"},
		".php": {"php", "text/x-php"}, ".lua": {"lua", "text/x-lua"},
		".swift": {"swift", "text/x-swift"}, ".kt": {"kotlin", "text/x-kotlin"},
		".kts": {"kotlin", "text/x-kotlin"}, ".cs": {"csharp", "text/x-csharp"},
	}[extension]

	return value[0], value[1], ok
}

func digest(data []byte) string {
	value := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(value[:])
}
