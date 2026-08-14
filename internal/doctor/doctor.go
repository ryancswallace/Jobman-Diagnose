// Package doctor verifies one configured provider/model through the complete
// bounded structured-generation contract without reading job evidence.
package doctor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/generation"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

const (
	// Kind identifies the stable provider-doctor result.
	Kind = "jobman.diagnosis_provider_doctor"
	// SchemaVersion is the current provider-doctor result schema.
	SchemaVersion = 1

	statusPass = "pass"
	statusFail = "fail"
)

const probeLog = "synchronize inventory: dial tcp 127.0.0.1:65535: connect: connection refused\n"

// Check records one safe, nonsecret doctor stage.
type Check struct {
	Code   string `json:"code"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Report describes whether a configured provider/model satisfies the
// generation contract required by diagnosis.
type Report struct {
	Kind          string            `json:"kind"`
	SchemaVersion int               `json:"schema_version"`
	Ready         bool              `json:"ready"`
	Profile       string            `json:"profile"`
	Provider      string            `json:"provider"`
	Locality      provider.Locality `json:"locality"`
	Model         string            `json:"model"`
	Checks        []Check           `json:"checks"`
}

// Run sends one fixed synthetic causal probe through the configured adapter,
// then validates response provenance and proposal semantics exactly as a real
// diagnosis would. It never includes user evidence, configuration contents,
// endpoints, credentials, or provider response text in its report.
//
//nolint:cyclop // Each doctor stage is reported independently and fails closed at its boundary.
func Run(
	ctx context.Context,
	profileName string,
	profile config.Profile,
	generator generation.Generator,
) (Report, error) {
	report := newReport(profileName, profile)
	if ctx == nil || strings.TrimSpace(profileName) == "" || generator == nil {
		return report, errors.New("run provider doctor: context, profile, and generator are required")
	}
	report.pass("configuration_profile", "the selected profile is valid")
	report.pass("credential_adapter", "credential resolution and adapter construction succeeded")
	if err := generation.ValidateGenerator(profile, generator); err != nil {
		report.fail("adapter_capabilities", "the adapter capabilities do not satisfy the selected profile")
		return report, nil
	}
	report.pass("adapter_capabilities", "native JSON Schema, locality, and configured byte limits are available")

	request, err := probeRequest(profile.MaximumOutputBytes)
	if err != nil {
		return report, fmt.Errorf("run provider doctor: construct probe: %w", err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return report, fmt.Errorf("run provider doctor: encode probe: %w", err)
	}
	if len(encoded) > profile.MaximumInputBytes {
		report.fail("request_contract", "the sealed minimal probe exceeds the profile's configured input limit")
		return report, nil
	}
	report.pass("request_contract", "a sealed request-specific JSON Schema probe fits the configured limits")

	requestContext, cancel := context.WithTimeout(ctx, profile.TimeoutDuration())
	defer cancel()
	response, generateErr := generator.Generate(requestContext, request)
	if generateErr != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return report, contextErr
		}
		report.fail("structured_generation", safeProviderFailure(generateErr))
		return report, nil
	}
	report.pass("structured_generation", "the configured provider and model returned structured content")
	if response.RequestID != request.RequestID || response.Provider != generator.Name() || response.Model != profile.Model {
		report.fail("response_provenance", "the response provenance did not match the selected provider, model, and request")
		return report, nil
	}
	report.pass("response_provenance", "the response matched the selected provider, model, and request")

	proposal, err := provider.DecodeProposal(bytes.NewReader(response.JSON), request)
	if err != nil {
		detail := "the model output did not satisfy the bounded proposal schema and semantic rules"
		if errors.Is(err, provider.ErrProposalNotSpecific) {
			detail = "the model output was structurally valid but not incident-specific"
		} else if errors.Is(err, provider.ErrProposalUnsupported) {
			detail = "the model output asserted a cause not supported by its cited probe evidence"
		}
		report.fail("proposal_contract", detail)
		return report, nil
	}
	report.pass("proposal_contract", "the response passed request-specific schema, authority, and evidence validation")
	if len(proposal.Hypotheses) != 1 || proposal.Hypotheses[0].Code != "generated.dependency_unavailable" ||
		!slices.Contains(proposal.Hypotheses[0].SupportingEvidence, "doctor:artifact:stderr") {
		report.fail("causal_probe", "the model did not identify the direct synthetic connection-refusal cause")
		return report, nil
	}
	report.pass("causal_probe", "the model identified and cited the direct synthetic connection-refusal cause")
	report.Ready = true

	return report, nil
}

// SetupFailure returns a safe failed report when credential resolution or
// adapter construction fails before a generator can be exercised.
func SetupFailure(profileName string, profile config.Profile) Report {
	report := newReport(profileName, profile)
	report.pass("configuration_profile", "the selected profile is valid")
	report.fail(
		"credential_adapter",
		"credential resolution or adapter construction failed; inspect the selected profile's credential reference and adapter settings",
	)

	return report
}

func newReport(profileName string, profile config.Profile) Report {
	return Report{
		Kind: Kind, SchemaVersion: SchemaVersion, Profile: profileName,
		Provider: profile.Provider, Locality: profile.Locality, Model: profile.Model, Checks: []Check{},
	}
}

func probeRequest(maximumOutputBytes int) (provider.Request, error) {
	contentDigest := digest([]byte(probeLog))
	capturedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	artifact := provider.ProjectedArtifact{
		ID: "doctor:artifact:stderr", Role: "target.log_tail", Run: 1, Stream: "stderr",
		Selection: "causal_context", AnchorLine: 1, AnchorReason: "causal_diagnostic",
		StartLine: 1, EndLine: 1, TotalLines: 1, ByteEnd: uint64(len(probeLog)),
		FileBytes: uint64(len(probeLog)), Content: probeLog, Encoding: "utf-8-lossy",
		Digest: contentDigest, ContentDigest: contentDigest, CapturedAt: &capturedAt, Quality: "observed",
		SelectedBytes: uint64(len(probeLog)), ContentBytes: uint64(len(probeLog)), Disclosure: "log_content",
	}
	return provider.SealRequest(provider.Request{
		AnalysisEvidenceID: digest([]byte("jobman-diagnose provider doctor synthetic probe")),
		Subject:            provider.Subject{Phase: "completed", Outcome: "failure", SelectedRuns: []uint64{1}},
		Projection: provider.Projection{
			Items: []provider.ProjectedItem{{
				ID: "doctor:item:exit", Code: "jobman.run.exit.code", Value: json.RawMessage(`1`),
				Quality: "observed", Disclosure: "metadata",
			}},
			Artifacts: []provider.ProjectedArtifact{artifact},
		},
		Manifest: provider.ProjectionManifest{
			Classes: []string{"metadata", "log_content"}, ItemIDs: []string{"doctor:item:exit"},
			ArtifactIDs: []string{artifact.ID}, ItemCount: 1, ArtifactCount: 1,
			ArtifactBytes: artifact.ContentBytes,
		},
		Deterministic: []provider.DeterministicCandidate{{
			ID: "doctor:finding:nonzero", Code: "core.nonzero_exit", Category: "process",
			Summary:            "The synthetic target exited unsuccessfully",
			Explanation:        "The synthetic exit status is observed; inspect the cited target diagnostic for its cause.",
			SupportingEvidence: []string{"doctor:item:exit"}, ContradictingEvidence: []string{},
		}},
		AllowedCategories:      []string{"network", "process"},
		AllowedHypothesisCodes: []string{"generated.dependency_unavailable"},
		AllowedActions:         []provider.AllowedAction{}, Instructions: provider.RequiredInstructions(),
		MaximumOutputBytes: maximumOutputBytes,
	})
}

func safeProviderFailure(err error) string {
	if code, message, ok := provider.Diagnostic(err); ok {
		return string(code) + ": " + message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request_timeout: the provider request exceeded the configured timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "request_canceled: the provider request was canceled"
	}

	return "provider_failure_unspecified: the provider failed without a safe diagnostic classification"
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)

	return "sha256:" + hex.EncodeToString(sum[:])
}

func (report *Report) pass(code, detail string) {
	report.Checks = append(report.Checks, Check{Code: code, Status: statusPass, Detail: detail})
}

func (report *Report) fail(code, detail string) {
	report.Checks = append(report.Checks, Check{Code: code, Status: statusFail, Detail: detail})
}

// Encode writes one doctor report as indented JSON.
func Encode(destination io.Writer, report Report) error {
	if destination == nil {
		return errors.New("encode provider doctor report: destination is nil")
	}
	encoder := json.NewEncoder(destination)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	return encoder.Encode(report)
}

// WriteHuman writes a compact check-by-check readiness report.
func WriteHuman(destination io.Writer, report Report) error {
	if destination == nil {
		return errors.New("write provider doctor report: destination is nil")
	}
	if _, err := fmt.Fprintf(
		destination, "Profile:  %s\nProvider: %s (%s)\nModel:    %s\n\n",
		safeLabel(report.Profile), safeLabel(report.Provider), report.Locality, safeLabel(report.Model),
	); err != nil {
		return err
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(destination, "[%s] %s: %s\n", check.Status, check.Code, check.Detail); err != nil {
			return err
		}
	}
	result := "not ready"
	if report.Ready {
		result = "ready"
	}
	_, err := fmt.Fprintf(destination, "\nResult: %s\n", result)

	return err
}

func safeLabel(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return '�'
		}

		return character
	}, value)
}
