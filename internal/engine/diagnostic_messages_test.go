package engine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestSpecificTargetDiagnosticCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		log    string
		code   string
		action string
		retry  diagnosis.RetryVerdict
	}{
		{name: "address", log: "listen tcp: address already in use\n", code: "target.address_in_use_message", action: "inspect_listener_collision", retry: diagnosis.RetryAfterChange},
		{name: "authentication", log: "HTTP 401 Unauthorized\n", code: "target.authentication_denied_message", action: "inspect_authentication", retry: diagnosis.RetryAfterChange},
		{name: "configuration", log: "required environment variable API_URL is missing\n", code: "target.configuration_missing_message", action: "inspect_target_configuration", retry: diagnosis.RetryAfterChange},
		{name: "connection", log: "connect ECONNREFUSED 127.0.0.1:5432\n", code: "target.connection_refused_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterDelay},
		{name: "validation", log: "invalid decimal amount 1.2O\n", code: "target.data_validation_message", action: "inspect_rejected_data", retry: diagnosis.RetryAfterChange},
		{name: "deadlock", log: "ERROR: deadlock detected\n", code: "target.database_deadlock_message", action: "inspect_database_failure", retry: diagnosis.RetryAfterDelay},
		{name: "unique", log: "duplicate key violates unique constraint\n", code: "target.database_unique_violation_message", action: "inspect_database_failure", retry: diagnosis.RetryAfterChange},
		{name: "deadline", log: "request: context deadline exceeded\n", code: "target.deadline_exceeded_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterDelay},
		{name: "dependency", log: "Could not find artifact example:rules:jar:1\n", code: "target.dependency_missing_message", action: "inspect_target_dependency", retry: diagnosis.RetryAfterChange},
		{name: "dns", log: "lookup inventory.internal: no such host\n", code: "target.dns_resolution_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterDelay},
		{name: "descriptors", log: "accept4: too many open files\n", code: "target.file_descriptor_exhausted_message", action: "inspect_resource_limit", retry: diagnosis.RetryAfterChange},
		{name: "linker", log: "ld: undefined reference to symbol\n", code: "target.linker_error_message", action: "inspect_target_dependency", retry: diagnosis.RetryAfterChange},
		{name: "migration rejected", log: "migration rejected: schema mismatch\n", code: "target.migration_rejected_message", action: "inspect_target_configuration", retry: diagnosis.RetryAfterChange},
		{name: "migration required", log: "apply migrations 009 through 011\n", code: "target.migration_required_message", action: "inspect_target_configuration", retry: diagnosis.RetryAfterChange},
		{name: "file", log: "open input.csv: no such file or directory\n", code: "target.missing_file_message", action: "inspect_target_dependency", retry: diagnosis.RetryAfterChange},
		{name: "nested command", log: "helper: command not found\n", code: "target.shell_command_not_found", action: "inspect_target_dependency", retry: diagnosis.RetryAfterChange},
		{name: "permission", log: "open output.csv: permission denied\n", code: "target.permission_message", action: "inspect_permissions", retry: diagnosis.RetryAfterChange},
		{name: "rate", log: "HTTP 429 Too Many Requests\n", code: "target.rate_limited_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterDelay},
		{name: "read only", log: "write config: read-only file system\n", code: "target.read_only_filesystem_message", action: "inspect_filesystem_policy", retry: diagnosis.RetryAfterChange},
		{name: "service", log: "HTTP 503 Service Unavailable\n", code: "target.service_unavailable_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterDelay},
		{name: "storage", log: "write output: no space left on device\n", code: "target.storage_exhausted_message", action: "inspect_resource_limit", retry: diagnosis.RetryAfterChange},
		{name: "tls", log: "x509: certificate signed by unknown authority\n", code: "target.tls_verification_message", action: "inspect_dependency_connection", retry: diagnosis.RetryAfterChange},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := diagnoseTargetLog(t, test.log)
			primary := report.Findings[0]
			if primary.ID != report.PrimaryFindingID || primary.Code != test.code || primary.Confidence.Score != 86 {
				t.Fatalf("primary = %#v", primary)
			}
			if !strings.Contains(strings.ToLower(primary.Summary), "reported") ||
				!strings.Contains(primary.Explanation, "did not independently") {
				t.Fatalf("finding crossed target-report trust boundary: %#v", primary)
			}
			if len(report.Actions) != 1 || report.Actions[0].Code != test.action ||
				report.Actions[0].Execution != diagnosis.ActionExecutionReadOnly || report.Actions[0].SafeToAutomate {
				t.Fatalf("actions = %#v", report.Actions)
			}
			if report.Retry.Verdict != test.retry {
				t.Fatalf("retry = %#v", report.Retry)
			}
		})
	}
}

func TestSpecificTargetDiagnosticsPreferLastSignalAndKeepEarlierFinding(t *testing.T) {
	t.Parallel()

	report := diagnoseTargetLog(t,
		"optional metrics file: permission denied; startup continued\n"+
			"worker startup failed: lookup queue.service.internal: no such host\n",
	)
	if report.Findings[0].Code != "target.dns_resolution_message" {
		t.Fatalf("primary = %#v", report.Findings[0])
	}
	codes := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		codes = append(codes, finding.Code)
	}
	if !slices.Contains(codes, "target.permission_message") {
		t.Fatalf("findings = %#v", report.Findings)
	}
}

func TestSpecificTargetDiagnosticsRemainCompatibleWithGenericEnrichmentFormat(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte("request: context deadline exceeded\n"))
	if err != nil {
		t.Fatal(err)
	}
	failure, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	for index := range failure.Enrichment {
		if failure.Enrichment[index].Code == enrichment.CodeCausalMessage {
			failure.Enrichment[index].Format = "causal_message"
		}
	}
	failure, err = diagnosis.SealFailureEvidence(core, failure.Enrichment)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := New("test", func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), failure)
	if err != nil {
		t.Fatal(err)
	}
	if report.Findings[0].Code != "target.deadline_exceeded_message" {
		t.Fatalf("primary = %#v", report.Findings[0])
	}
}

func TestAmbiguousKilledMessageDoesNotInventResourceCause(t *testing.T) {
	t.Parallel()

	report := diagnoseTargetLog(t, "Killed\ncontainer exited with status 137; reason unavailable\n")
	if report.Findings[0].Code != "core.nonzero_exit" {
		t.Fatalf("primary = %#v", report.Findings[0])
	}
}

func diagnoseTargetLog(t *testing.T, log string) diagnosis.Report {
	t.Helper()

	core, err := testevidence.Failed("nonzero_exit", []byte(log))
	if err != nil {
		t.Fatal(err)
	}
	failure, err := enrichment.Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := New("test", func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), failure)
	if err != nil {
		t.Fatal(err)
	}

	return report
}
