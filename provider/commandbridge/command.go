// Package commandbridge implements the bounded local structured-generator
// protocol over one child process stdin/stdout exchange.
package commandbridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

const (
	// ProtocolVersion identifies the command-bridge environment contract.
	ProtocolVersion    = 3
	maximumStderrBytes = 16 * 1024
)

// Config defines one immutable command bridge.
type Config struct {
	Executable         string
	Arguments          []string
	Model              string
	Credential         []byte
	MaximumInputBytes  int
	MaximumOutputBytes int
}

// Generator executes an explicitly configured local provider command without a shell.
type Generator struct{ config Config }

// New validates the executable and returns a local generator.
func New(configuration Config) (*Generator, error) {
	if !filepath.IsAbs(configuration.Executable) || filepath.Clean(configuration.Executable) != configuration.Executable ||
		strings.TrimSpace(configuration.Model) == "" || configuration.MaximumInputBytes < 1 ||
		configuration.MaximumOutputBytes < 1 || len(configuration.Arguments) > 64 {
		return nil, errors.New("construct command generator: invalid configuration")
	}
	info, err := os.Lstat(configuration.Executable)
	if err != nil {
		return nil, fmt.Errorf("construct command generator: inspect executable: %w", err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("construct command generator: executable must be a regular executable file")
	}
	for _, argument := range configuration.Arguments {
		if len(argument) > 4096 || strings.ContainsRune(argument, '\x00') {
			return nil, errors.New("construct command generator: invalid fixed argument")
		}
	}
	configuration.Arguments = slices.Clone(configuration.Arguments)
	configuration.Credential = slices.Clone(configuration.Credential)

	return &Generator{config: configuration}, nil
}

// Name returns the stable provider identifier.
func (*Generator) Name() string { return "command" }

// Capabilities reports the configured hard transport bounds.
func (generator *Generator) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		NativeJSONSchema: true, MaximumInputBytes: generator.config.MaximumInputBytes,
		MaximumOutputBytes: generator.config.MaximumOutputBytes, Locality: provider.LocalityLocal,
	}
}

// Generate executes one request and accepts one raw proposal object on stdout.
func (generator *Generator) Generate(ctx context.Context, request provider.Request) (provider.Response, error) {
	if ctx == nil {
		return provider.Response{}, provider.NewFailure(
			provider.FailureInvalidRequest, errors.New("command generator: nil context"),
		)
	}
	if err := provider.VerifyRequest(request); err != nil {
		return provider.Response{}, provider.NewFailure(
			provider.FailureInvalidRequest, fmt.Errorf("command generator: %w", err),
		)
	}
	var input bytes.Buffer
	if err := provider.EncodeRequest(&input, request); err != nil {
		return provider.Response{}, provider.NewFailure(provider.FailureInvalidRequest, err)
	}
	if input.Len() > generator.config.MaximumInputBytes {
		return provider.Response{}, provider.NewFailure(
			provider.FailureInputOversized,
			fmt.Errorf("command generator: request exceeds %d bytes", generator.config.MaximumInputBytes),
		)
	}
	// #nosec G204 -- the executable and fixed arguments come from a validated,
	// explicit local profile and are never derived from evidence or model output.
	command := exec.CommandContext(ctx, generator.config.Executable, generator.config.Arguments...)
	command.Stdin = bytes.NewReader(input.Bytes())
	stdout := newBoundedBuffer(generator.config.MaximumOutputBytes)
	stderr := newBoundedBuffer(maximumStderrBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = bridgeEnvironment(generator.config.Model, request.RequestID, generator.config.Credential)
	command.WaitDelay = 2 * time.Second
	configureProcess(command)
	if err := command.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			code := provider.FailureRequestCanceled
			if errors.Is(contextErr, context.DeadlineExceeded) {
				code = provider.FailureRequestTimeout
			}

			return provider.Response{}, provider.NewFailure(code, fmt.Errorf("command generator: %w", contextErr))
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return provider.Response{}, provider.NewFailure(
				provider.FailureProviderExit,
				fmt.Errorf("command generator: provider exited with status %d", exitError.ExitCode()),
			)
		}
		return provider.Response{}, provider.NewFailure(
			provider.FailureRequestFailed, fmt.Errorf("command generator: execute provider: %w", err),
		)
	}
	if stdout.exceeded {
		return provider.Response{}, provider.NewFailure(
			provider.FailureContentOversized,
			fmt.Errorf("command generator: output exceeds %d bytes", generator.config.MaximumOutputBytes),
		)
	}
	if len(stdout.bytes()) == 0 {
		return provider.Response{}, provider.NewFailure(
			provider.FailureOutputEmpty, errors.New("command generator: provider returned no output"),
		)
	}

	return provider.Response{
		JSON: slices.Clone(stdout.bytes()), Provider: generator.Name(), Model: generator.config.Model,
		RequestID: request.RequestID,
	}, nil
}

func bridgeEnvironment(model, requestID string, credential []byte) []string {
	result := platformEnvironment()
	result = append(result,
		fmt.Sprintf("JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL=%d", ProtocolVersion),
		"JOBMAN_DIAGNOSE_PROVIDER_MODEL="+model,
		"JOBMAN_DIAGNOSE_REQUEST_ID="+requestID,
	)
	if len(credential) != 0 {
		result = append(result, "JOBMAN_DIAGNOSE_PROVIDER_CREDENTIAL="+string(credential))
	}

	return result
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func newBoundedBuffer(limit int) *boundedBuffer { return &boundedBuffer{limit: limit} }

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining < len(value) {
		buffer.exceeded = true
		value = value[:max(remaining, 0)]
	}
	if len(value) != 0 {
		_, _ = buffer.buffer.Write(value)
	}

	return original, nil
}

func (buffer *boundedBuffer) bytes() []byte { return buffer.buffer.Bytes() }

var (
	_ provider.StructuredGenerator = (*Generator)(nil)
	_ provider.Describer           = (*Generator)(nil)
)
