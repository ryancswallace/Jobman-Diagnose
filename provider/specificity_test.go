package provider

import "testing"

func TestSpecificHypothesisTextAcceptsIncidentSpecificPythonFailureLabDiagnoses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		summary     string
		rootCause   string
		explanation string
	}{
		{
			name: "zero division", summary: "Batch nightly-042 divides total cost by zero units",
			rootCause:   "The batch declares units as 0, so average_unit_cost has a zero divisor.",
			explanation: "summarize_batch passes that divisor to division and Python raises ZeroDivisionError.",
		},
		{
			name: "missing environment", summary: "Payment initialization is missing JOBMAN_DEMO_PAYMENTS_API_URL",
			rootCause:   "The required payments service endpoint variable is unset or empty.",
			explanation: "required_environment rejects the absent value before reconciliation can initialize.",
		},
		{
			name: "invalid json", summary: "Deployment JSON has a trailing comma after the search feature",
			rootCause:   "The features array contains a comma immediately before its closing bracket.",
			explanation: "json.loads cannot parse that token sequence and raises JSONDecodeError.",
		},
		{
			name: "configuration schema", summary: "Deployment configuration violates four required field constraints",
			rootCause:   "Region moon-1, retries three, timeout -5, and a missing database.dsn are all invalid.",
			explanation: "validate accumulates those violations and raises ValueError before deployment starts.",
		},
		{
			name: "missing module", summary: "The acme_internal_feature_flags Python module is not installed",
			rootCause:   "The deployed environment lacks the private feature-flag package.",
			explanation: "Python cannot resolve the import and raises ModuleNotFoundError during startup.",
		},
		{
			name: "missing file", summary: "The deployed customer-segments.csv input file is absent",
			rootCause:   "The expected inputs/customer-segments.csv path does not exist beside the script.",
			explanation: "Path.read_text opens the missing application input and raises FileNotFoundError.",
		},
		{
			name: "permission denied", summary: "The service identity cannot read /srv/payments/private-key.pem",
			rootCause:   "Access to the configured signing key is denied for the target identity.",
			explanation: "Loading signing material raises PermissionError before payment work begins.",
		},
		{
			name: "connection refused", summary: "The inventory service refuses the connection at 127.0.0.1:4319",
			rootCause:   "No accepting inventory-service listener is available at the configured endpoint.",
			explanation: "The connection attempt raises ConnectionRefusedError before an inventory snapshot is fetched.",
		},
		{
			name: "chained timeout", summary: "Inventory synchronization exhausted retries after a 750 ms upstream timeout",
			rootCause:   "The inventory service did not return the required snapshot within 750 ms.",
			explanation: "TimeoutError is chained into UpstreamUnavailable after synchronization attempt 3.",
		},
		{
			name: "missing executable", summary: "The warehouse-migrate helper executable is missing",
			rootCause:   "The target environment cannot resolve warehouse-migrate from its executable search path.",
			explanation: "subprocess.run cannot launch the migration check and raises FileNotFoundError.",
		},
		{
			name: "child exit", summary: "Database schema version 8 is incompatible with expected version 11",
			rootCause:   "Migrations 009 through 011 have not been applied to the database.",
			explanation: "The compatibility child exits 17 and subprocess.run raises CalledProcessError.",
		},
		{
			name: "exception group", summary: "Concurrent reconciliation loses customer C-1042 and rejects invoice INV-778",
			rootCause:   "One task cannot find the customer while another sees a negative settlement amount.",
			explanation: "Both task failures escape the TaskGroup together as an ExceptionGroup.",
		},
		{
			name: "unicode decode", summary: "Partner record 184 contains Latin-1 byte e9 but is decoded as UTF-8",
			rootCause:   "The feed encoding does not match the UTF-8 decoder selected by the target.",
			explanation: "record.decode rejects byte e9 and raises UnicodeDecodeError.",
		},
		{
			name: "business invariant", summary: "Order ORD-2048 requests 12 units when only 4 are available",
			rootCause:   "Available WIDGET-BLUE inventory is below the order quantity.",
			explanation: "The reservation invariant fails and raises AssertionError before stock is reserved.",
		},
		{
			name: "job timeout", summary: "Queue consumer never receives partition ownership before the two-second run timeout",
			rootCause:   "The consumer remains in rebalancing without a partition assignment.",
			explanation: "Jobman's run deadline expires while the target is still waiting and terminates it.",
		},
		{
			name: "signal", summary: "The worker terminates itself with SIGTERM after an unrecoverable coordinator state",
			rootCause:   "The worker classifies its coordinator state as unrecoverable.",
			explanation: "It sends SIGTERM to its own process, producing the signal-termination outcome.",
		},
		{
			name: "syntax", summary: "calculate_total is missing a colon after its function signature",
			rootCause:   "The def statement is syntactically incomplete.",
			explanation: "Python's parser rejects the file with SyntaxError before any application code runs.",
		},
		{
			name: "pipeline cause chain", summary: "Record partner-west:8841 has invalid decimal amount 1,204.5O",
			rootCause:   "The amount contains grouping punctuation and the letter O where Decimal expects a numeric value.",
			explanation: "InvalidOperation is chained into RecordTransformError for quarterly-rebate.csv.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			hypothesis := Hypothesis{
				Summary: test.summary, RootCause: test.rootCause, Explanation: test.explanation,
			}
			if !specificHypothesisText(hypothesis) {
				t.Fatalf("specificHypothesisText() rejected %#v", hypothesis)
			}
		})
	}
}

func TestSpecificHypothesisTextRejectsGenericAndEvidencePlumbingDiagnoses(t *testing.T) {
	t.Parallel()

	tests := []Hypothesis{
		{
			Summary:   "Invalid target input caused the target to exit with a nonzero status.",
			RootCause: "Invalid target input", Explanation: "The application stopped after validation.",
		},
		{
			Summary:   "The application encountered an error while processing the request.",
			RootCause: "An error occurred", Explanation: "The error caused the process to stop.",
		},
		{
			Summary:     "Python output was classified as a failure.",
			RootCause:   "The log contains a Python exception traceback.",
			Explanation: "The exact sanitized byte range is attributed as companion enrichment.",
		},
		{
			Summary: "A specific-looking summary", RootCause: "A specific-looking summary!",
			Explanation: "A different causal path.",
		},
	}
	for index, hypothesis := range tests {
		if specificHypothesisText(hypothesis) {
			t.Fatalf("specificHypothesisText(%d) accepted %#v", index, hypothesis)
		}
	}
}
