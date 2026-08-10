package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProposalSchemaRejectsInvalidAuthority(t *testing.T) {
	request := validRequest(t)
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{
			name: "request ID", mutate: func(value *Request) { value.RequestID = "invalid" },
			want: "request ID is invalid",
		},
		{
			name: "evidence", mutate: func(value *Request) {
				value.Manifest.ItemIDs = nil
				value.Manifest.ArtifactIDs = nil
				value.Manifest.EnrichmentIDs = nil
			},
			want: "authority catalogs are incomplete",
		},
		{
			name: "findings", mutate: func(value *Request) { value.Deterministic = nil },
			want: "authority catalogs are incomplete",
		},
		{
			name: "categories", mutate: func(value *Request) { value.AllowedCategories = nil },
			want: "authority catalogs are incomplete",
		},
		{
			name: "hypothesis codes", mutate: func(value *Request) { value.AllowedHypothesisCodes = nil },
			want: "authority catalogs are incomplete",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := request
			test.mutate(&current)
			if _, err := proposalJSONSchemaForRequest(current); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("proposalJSONSchemaForRequest() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestProposalSchemaRejectsCorruptReviewedTemplate(t *testing.T) {
	request := validRequest(t)
	original := append(json.RawMessage(nil), proposalSchema...)
	t.Cleanup(func() { proposalSchema = original })

	proposalSchema = json.RawMessage(`{`)
	if _, err := proposalJSONSchemaForRequest(request); err == nil || !strings.Contains(err.Error(), "decode reviewed template") {
		t.Fatalf("proposalJSONSchemaForRequest(malformed template) error = %v", err)
	}
	proposalSchema = original

	tests := []struct {
		name   string
		mutate func(*testing.T, map[string]any)
	}{
		{name: "properties", mutate: func(_ *testing.T, document map[string]any) { delete(document, "properties") }},
		{name: "request ID", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, document, "properties"), "request_id")
		}},
		{name: "hypotheses", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, document, "properties"), "hypotheses")
		}},
		{name: "hypothesis code", mutate: func(t *testing.T, document map[string]any) {
			delete(hypothesisSchemaProperties(t, document), "code")
		}},
		{name: "hypothesis category", mutate: func(t *testing.T, document map[string]any) {
			delete(hypothesisSchemaProperties(t, document), "category")
		}},
		{name: "supporting evidence", mutate: func(t *testing.T, document map[string]any) {
			delete(hypothesisSchemaProperties(t, document), "supporting_evidence")
		}},
		{name: "supporting evidence items", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, hypothesisSchemaProperties(t, document), "supporting_evidence"), "items")
		}},
		{name: "contradicting evidence items", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, hypothesisSchemaProperties(t, document), "contradicting_evidence"), "items")
		}},
		{name: "contradicted findings", mutate: func(t *testing.T, document map[string]any) {
			delete(hypothesisSchemaProperties(t, document), "contradicts_findings")
		}},
		{name: "recommended actions", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, document, "properties"), "recommended_action_ids")
		}},
		{name: "recommended action items", mutate: func(t *testing.T, document map[string]any) {
			delete(mustSchemaObject(t, document, "properties", "recommended_action_ids"), "items")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(original, &document); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, document)
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			proposalSchema = encoded
			_, specializationErr := proposalJSONSchemaForRequest(request)
			proposalSchema = original
			if specializationErr == nil || !strings.Contains(specializationErr.Error(), "reviewed template path") {
				t.Fatalf("proposalJSONSchemaForRequest(corrupt template) error = %v", specializationErr)
			}
		})
	}
}

func TestSetSchemaEnumRejectsMissingAuthorityAndField(t *testing.T) {
	if err := setSchemaEnum(map[string]any{}, "code", nil); err == nil || !strings.Contains(err.Error(), "authority is empty") {
		t.Fatalf("setSchemaEnum(empty authority) error = %v", err)
	}
	if err := setSchemaEnum(map[string]any{}, "code", []string{"generated.test"}); err == nil ||
		!strings.Contains(err.Error(), "reviewed template path") {
		t.Fatalf("setSchemaEnum(missing field) error = %v", err)
	}
}

func hypothesisSchemaProperties(t *testing.T, document map[string]any) map[string]any {
	t.Helper()

	return mustSchemaObject(t, document, "properties", "hypotheses", "items", "properties")
}
