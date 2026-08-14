package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

type fakeGenerator struct {
	profile  config.Profile
	name     string
	response func(provider.Request) (provider.Response, error)
	locality provider.Locality
}

func (generator *fakeGenerator) Name() string {
	if generator.name != "" {
		return generator.name
	}
	return "openai_compatible"
}

func (generator *fakeGenerator) Capabilities() provider.Capabilities {
	locality := generator.locality
	if locality == "" {
		locality = generator.profile.Locality
	}
	return provider.Capabilities{
		NativeJSONSchema: true, MaximumInputBytes: generator.profile.MaximumInputBytes,
		MaximumOutputBytes: generator.profile.MaximumOutputBytes, Locality: locality,
	}
}

func (generator *fakeGenerator) Generate(_ context.Context, request provider.Request) (provider.Response, error) {
	return generator.response(request)
}

func TestRunValidatesCompleteProviderModelContract(t *testing.T) {
	t.Parallel()

	profile := doctorProfile(t)
	generator := &fakeGenerator{profile: profile, response: validProbeResponse(t, profile)}
	report, err := Run(t.Context(), "test", profile, generator)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready || len(report.Checks) != 8 || report.Checks[len(report.Checks)-1].Code != "causal_probe" {
		t.Fatalf("doctor report = %#v", report)
	}
	var encoded bytes.Buffer
	if err := Encode(&encoded, report); err != nil || !json.Valid(encoded.Bytes()) ||
		strings.Contains(encoded.String(), "127.0.0.1:65535") {
		t.Fatalf("encoded report/error = %q / %v", encoded.String(), err)
	}
	encoded.Reset()
	if err := WriteHuman(&encoded, report); err != nil || !strings.Contains(encoded.String(), "Result: ready") ||
		!strings.Contains(encoded.String(), "test-model") {
		t.Fatalf("human report/error = %q / %v", encoded.String(), err)
	}
}

func TestRunReportsSafeReadinessFailures(t *testing.T) {
	t.Parallel()

	profile := doctorProfile(t)
	secret := "provider response contains secret-looking text"
	tests := []struct {
		name      string
		generator *fakeGenerator
		wantCode  string
		wantText  string
	}{
		{
			name: "capabilities",
			generator: &fakeGenerator{
				profile: profile, locality: provider.LocalityRemote,
				response: validProbeResponse(t, profile),
			},
			wantCode: "adapter_capabilities", wantText: "do not satisfy",
		},
		{
			name: "classified provider failure",
			generator: &fakeGenerator{profile: profile, response: func(provider.Request) (provider.Response, error) {
				return provider.Response{}, provider.NewFailure(provider.FailureHTTPStatus, errors.New(secret))
			}},
			wantCode: "structured_generation", wantText: "http_status",
		},
		{
			name: "provenance",
			generator: &fakeGenerator{profile: profile, response: func(request provider.Request) (provider.Response, error) {
				response, err := validProbeResponse(t, profile)(request)
				response.Model = "other-model"
				return response, err
			}},
			wantCode: "response_provenance", wantText: "did not match",
		},
		{
			name: "abstention",
			generator: &fakeGenerator{profile: profile, response: func(request provider.Request) (provider.Response, error) {
				encoded, err := json.Marshal(provider.Proposal{
					Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion, RequestID: request.RequestID,
					Hypotheses: []provider.Hypothesis{}, RecommendedActions: []string{},
					MissingEvidence: []provider.MissingEvidence{{Code: "generated.more_context", Description: "More context is required."}},
				})
				return provider.Response{JSON: encoded, Provider: "openai_compatible", Model: profile.Model, RequestID: request.RequestID}, err
			}},
			wantCode: "causal_probe", wantText: "did not identify",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report, err := Run(t.Context(), "test", profile, test.generator)
			if err != nil || report.Ready || report.Checks[len(report.Checks)-1].Code != test.wantCode ||
				!strings.Contains(report.Checks[len(report.Checks)-1].Detail, test.wantText) ||
				strings.Contains(report.Checks[len(report.Checks)-1].Detail, secret) {
				t.Fatalf("doctor report/error = %#v / %v", report, err)
			}
		})
	}
}

func TestDoctorRejectsInvalidInputsAndWriters(t *testing.T) {
	t.Parallel()

	profile := doctorProfile(t)
	if _, err := Run(nil, "test", profile, &fakeGenerator{}); err == nil { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatal("Run(nil) error = nil")
	}
	if err := Encode(nil, Report{}); err == nil {
		t.Fatal("Encode(nil) error = nil")
	}
	if err := WriteHuman(nil, Report{}); err == nil {
		t.Fatal("WriteHuman(nil) error = nil")
	}
}

func TestSetupFailureIsSafeAndStructured(t *testing.T) {
	t.Parallel()

	report := SetupFailure("test", doctorProfile(t))
	if report.Ready || len(report.Checks) != 2 || report.Checks[1].Code != "credential_adapter" ||
		!strings.Contains(report.Checks[1].Detail, "credential resolution") {
		t.Fatalf("setup failure = %#v", report)
	}
}

func validProbeResponse(t *testing.T, profile config.Profile) func(provider.Request) (provider.Response, error) {
	t.Helper()
	return func(request provider.Request) (provider.Response, error) {
		proposal := provider.Proposal{
			Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion, RequestID: request.RequestID,
			Hypotheses: []provider.Hypothesis{{
				Code: "generated.dependency_unavailable", Category: "network",
				Summary:            "Inventory synchronization was refused by 127.0.0.1:65535",
				RootCause:          "The synchronize inventory connection to 127.0.0.1:65535 failed with connection refused.",
				Explanation:        "The connection refused signal prevents the inventory synchronization request from completing.",
				SupportingEvidence: []string{"doctor:artifact:stderr"}, ContradictingEvidence: []string{},
				ContradictsFindings: []string{},
			}},
			RecommendedActions: []string{}, MissingEvidence: []provider.MissingEvidence{},
		}
		encoded, err := json.Marshal(proposal)

		return provider.Response{
			JSON: encoded, Provider: "openai_compatible", Model: profile.Model, RequestID: request.RequestID,
		}, err
	}
}

func doctorProfile(t *testing.T) config.Profile {
	t.Helper()
	configuration := config.File{
		SchemaVersion: config.SchemaVersion, Defaults: config.Defaults{Profile: "test"},
		Profiles: map[string]config.Profile{"test": {
			Provider: "openai_compatible", Locality: provider.LocalityLocal,
			Endpoint: "http://127.0.0.1:8000/v1/chat/completions", Model: "test-model",
			RequireJSONSchema: true, Timeout: "2s", MaximumInputBytes: 256 * 1024,
			MaximumOutputBytes: 32 * 1024,
			Disclosure: map[string]config.ClassLimits{
				"metadata": {MaximumItems: 256, MaximumBytes: 128 * 1024},
			},
		}},
	}
	if err := configuration.Validate(); err != nil {
		t.Fatal(err)
	}

	return configuration.Profiles["test"]
}
