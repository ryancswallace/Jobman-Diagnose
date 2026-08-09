// Package coreclient obtains sealed evidence from the Jobman process boundary.
package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"
)

const (
	// ExtensionProtocolVersion is the Jobman-to-companion environment protocol.
	ExtensionProtocolVersion = "1"
	maximumCoreOutputBytes   = 3 * 1024 * 1024
	maximumCoreErrorBytes    = 64 * 1024
)

// Options selects a core executable and nonsecret invocation context.
type Options struct {
	Executable  string
	StateDir    string
	ConfigPath  string
	Environment []string
}

// Client invokes a specific absolute Jobman executable without a shell.
type Client struct {
	executable  string
	stateDir    string
	configPath  string
	environment []string
	run         runCommand
}

type runCommand func(context.Context, string, []string, []string) ([]byte, []byte, error)

// New validates the extension protocol or explicit executable selection.
func New(options Options) (*Client, error) {
	environment := options.Environment
	if environment == nil {
		environment = os.Environ()
	}
	executable, stateDir, configPath, err := resolveOptions(options, environment)
	if err != nil {
		return nil, err
	}

	return &Client{
		executable: executable, stateDir: stateDir, configPath: configPath,
		environment: slices.Clone(environment), run: execute,
	}, nil
}

// Collect runs the stable Jobman evidence command and verifies its result.
func (client *Client) Collect(ctx context.Context, request diagnostic.EvidenceRequest) (diagnostic.Evidence, error) {
	if ctx == nil {
		return diagnostic.Evidence{}, errors.New("collect core evidence: nil context")
	}
	arguments := client.arguments(request)
	environment := replaceEnvironment(client.environment, "JOBMAN_NO_EXTENSIONS", "1")
	stdout, stderr, err := client.run(ctx, client.executable, arguments, environment)
	if err != nil {
		message := strings.TrimSpace(string(stderr))
		if message == "" {
			return diagnostic.Evidence{}, fmt.Errorf("collect core evidence: %w", err)
		}

		return diagnostic.Evidence{}, fmt.Errorf("collect core evidence: %s: %w", message, err)
	}
	evidence, err := DecodeEvidence(bytes.NewReader(stdout))
	if err != nil {
		return diagnostic.Evidence{}, fmt.Errorf("collect core evidence: decode output: %w", err)
	}

	return evidence, nil
}

func (client *Client) arguments(request diagnostic.EvidenceRequest) []string {
	arguments := make([]string, 0, 18)
	if client.stateDir != "" {
		arguments = append(arguments, "--state-dir", client.stateDir)
	}
	if client.configPath != "" {
		arguments = append(arguments, "--config", client.configPath)
	}
	arguments = append(arguments, "show", "evidence")
	if request.Run != 0 {
		arguments = append(arguments, "--run="+strconv.FormatInt(request.Run, 10))
	}
	if request.AllRuns {
		arguments = append(arguments, "--all-runs")
	}
	if request.IncludeCommand {
		arguments = append(arguments, "--command")
	}
	if request.IncludePaths {
		arguments = append(arguments, "--paths")
	}
	if request.IncludeEnvironmentNames {
		arguments = append(arguments, "--environment-names")
	}
	if request.Logs != "" {
		arguments = append(arguments, "--logs", string(request.Logs))
	}
	if request.LogBytes != 0 {
		arguments = append(arguments, "--log-bytes", strconv.FormatUint(request.LogBytes, 10)+"B")
	}
	if request.Similar != 0 {
		arguments = append(arguments, "--similar", strconv.FormatUint(request.Similar, 10))
	}
	arguments = append(arguments, "--json", request.Selector)

	return arguments
}

// DecodeEvidence accepts either a raw evidence value or Jobman's version-1
// CLI envelope. Input is bounded before any decode allocation.
func DecodeEvidence(source io.Reader) (diagnostic.Evidence, error) {
	if source == nil {
		return diagnostic.Evidence{}, errors.New("decode core evidence: nil source")
	}
	encoded, err := io.ReadAll(io.LimitReader(source, maximumCoreOutputBytes+1))
	if err != nil {
		return diagnostic.Evidence{}, fmt.Errorf("decode core evidence: read: %w", err)
	}
	if len(encoded) > maximumCoreOutputBytes {
		return diagnostic.Evidence{}, fmt.Errorf("decode core evidence: input exceeds %d bytes", maximumCoreOutputBytes)
	}
	var header struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return diagnostic.Evidence{}, fmt.Errorf("decode core evidence: %w", err)
	}
	if header.Kind == diagnostic.Kind {
		return diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{MaxBytes: maximumCoreOutputBytes})
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		Data          struct {
			Evidence json.RawMessage `json:"evidence"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := decoder.Decode(&envelope); err != nil {
		return diagnostic.Evidence{}, fmt.Errorf("decode core evidence envelope: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return diagnostic.Evidence{}, err
	}
	if envelope.SchemaVersion != 1 || len(envelope.Data.Evidence) == 0 {
		return diagnostic.Evidence{}, errors.New("decode core evidence: unsupported or incomplete CLI envelope")
	}

	return diagnostic.Decode(bytes.NewReader(envelope.Data.Evidence), diagnostic.DecodeLimits{MaxBytes: maximumCoreOutputBytes})
}

func resolveOptions(options Options, environment []string) (string, string, string, error) {
	executable := options.Executable
	stateDir := options.StateDir
	configPath := options.ConfigPath
	protocol := environmentValue(environment, "JOBMAN_EXTENSION_PROTOCOL")
	if executable == "" && protocol != "" {
		if protocol != ExtensionProtocolVersion {
			return "", "", "", fmt.Errorf("unsupported Jobman extension protocol %q", protocol)
		}
		executable = environmentValue(environment, "JOBMAN_EXECUTABLE")
		if stateDir == "" {
			stateDir = environmentValue(environment, "JOBMAN_STATE_DIR")
		}
		if configPath == "" {
			configPath = environmentValue(environment, "JOBMAN_CONFIG")
		}
	}
	if executable == "" {
		located, err := exec.LookPath("jobman")
		if err != nil {
			return "", "", "", errors.New("locate Jobman: use --jobman or install jobman on PATH")
		}
		executable = located
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve Jobman executable: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect Jobman executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", "", "", errors.New("jobman executable is not an executable regular file")
	}

	return filepath.Clean(absolute), stateDir, configPath, nil
}

func execute(ctx context.Context, executable string, arguments, environment []string) ([]byte, []byte, error) {
	// #nosec G204 -- executable is a validated regular file and arguments never pass through a shell.
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = environment
	stdout := &cappedBuffer{maximum: maximumCoreOutputBytes}
	stderr := &cappedBuffer{maximum: maximumCoreErrorBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.overflow {
		return nil, stderr.Bytes(), fmt.Errorf("jobman output exceeds %d bytes", maximumCoreOutputBytes)
	}
	if stderr.overflow {
		return nil, stderr.Bytes(), errors.New("jobman error output was truncated")
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	maximum  int
	overflow bool
}

func (buffer *cappedBuffer) Write(value []byte) (int, error) {
	remaining := buffer.maximum - buffer.buffer.Len()
	if remaining > 0 {
		_, _ = buffer.buffer.Write(value[:min(len(value), remaining)])
	}
	if len(value) > remaining {
		buffer.overflow = true
	}

	return len(value), nil
}

func (buffer *cappedBuffer) Bytes() []byte { return bytes.Clone(buffer.buffer.Bytes()) }

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for index := len(environment) - 1; index >= 0; index-- {
		if strings.HasPrefix(environment[index], prefix) {
			return strings.TrimPrefix(environment[index], prefix)
		}
	}

	return ""
}

func replaceEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}

	return append(result, prefix+value)
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode core evidence: trailing JSON value")
		}
		return fmt.Errorf("decode core evidence: trailing data: %w", err)
	}

	return nil
}
