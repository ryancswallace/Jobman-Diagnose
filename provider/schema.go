package provider

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

//go:embed proposal.schema.json
var proposalSchema json.RawMessage

// ProposalJSONSchema returns an independent copy of the reviewed proposal
// schema template. SealRequest specializes this template with the exact
// request identity and authority catalogs before sending it to a generator.
func ProposalJSONSchema() json.RawMessage {
	return append(json.RawMessage(nil), proposalSchema...)
}

// proposalJSONSchemaForRequest specializes the reviewed template with every
// request-specific scalar authority that JSON Schema can enforce. Relational
// invariants such as disjoint evidence sets and duplicate hypothesis codes
// remain host-validated after decoding.
func proposalJSONSchemaForRequest(request Request) (json.RawMessage, error) {
	if !validDigest(request.RequestID) {
		return nil, errors.New("build proposal schema: request ID is invalid")
	}
	evidenceIDs, findingIDs, actionIDs := proposalAuthorityIDs(request)
	if len(evidenceIDs) == 0 || len(findingIDs) == 0 || len(request.AllowedCategories) == 0 ||
		len(request.AllowedHypothesisCodes) == 0 {
		return nil, errors.New("build proposal schema: request authority catalogs are incomplete")
	}

	var document map[string]any
	if decodeErr := json.Unmarshal(proposalSchema, &document); decodeErr != nil {
		return nil, fmt.Errorf("build proposal schema: decode reviewed template: %w", decodeErr)
	}
	properties, err := schemaObject(document, "properties")
	if err != nil {
		return nil, err
	}
	if err = specializeRequestID(properties, request.RequestID); err != nil {
		return nil, err
	}
	if err = specializeEvidenceCoverage(properties, request.DiagnosticFocus); err != nil {
		return nil, err
	}
	if err = specializeHypotheses(properties, request, evidenceIDs, findingIDs); err != nil {
		return nil, err
	}
	if err = specializeActions(properties, actionIDs); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("build proposal schema: encode specialization: %w", err)
	}

	return encoded, nil
}

func specializeEvidenceCoverage(properties map[string]any, focus []DiagnosticFocus) error {
	coverage, err := schemaObject(properties, "evidence_coverage")
	if err != nil {
		return err
	}
	coverage["minItems"] = float64(len(focus))
	coverage["maxItems"] = float64(len(focus))
	if len(focus) == 0 {
		return nil
	}
	entry, err := schemaObject(coverage, "items", "properties")
	if err != nil {
		return err
	}
	identifiers := make([]string, 0, len(focus))
	lines := make([]string, 0, len(focus)*2)
	for _, item := range focus {
		identifiers = append(identifiers, item.EvidenceID)
		lines = append(lines, item.Lines...)
	}
	slices.Sort(lines)
	lines = slices.Compact(lines)
	if enumErr := setSchemaEnum(entry, "evidence_id", identifiers); enumErr != nil {
		return enumErr
	}
	diagnosticLines, err := schemaObject(entry, "diagnostic_lines")
	if err != nil {
		return err
	}
	diagnosticItems, err := schemaObject(diagnosticLines, "items")
	if err != nil {
		return err
	}
	diagnosticItems["enum"] = lines

	return nil
}

func proposalAuthorityIDs(request Request) ([]string, []string, []string) {
	evidenceIDs := append(slices.Clone(request.Manifest.ItemIDs), request.Manifest.ArtifactIDs...)
	evidenceIDs = append(evidenceIDs, request.Manifest.EnrichmentIDs...)
	slices.Sort(evidenceIDs)
	findingIDs := make([]string, 0, len(request.Deterministic))
	for _, candidate := range request.Deterministic {
		findingIDs = append(findingIDs, candidate.ID)
	}
	slices.Sort(findingIDs)
	actionIDs := make([]string, 0, len(request.AllowedActions))
	for _, action := range request.AllowedActions {
		actionIDs = append(actionIDs, action.ID)
	}
	slices.Sort(actionIDs)

	return evidenceIDs, findingIDs, actionIDs
}

func specializeRequestID(properties map[string]any, requestIDValue string) error {
	requestID, err := schemaObject(properties, "request_id")
	if err != nil {
		return err
	}
	delete(requestID, "pattern")
	requestID["const"] = requestIDValue

	return nil
}

func specializeHypotheses(
	properties map[string]any,
	request Request,
	evidenceIDs []string,
	findingIDs []string,
) error {
	hypothesisCollection, err := schemaObject(properties, "hypotheses")
	if err != nil {
		return err
	}
	if !requestSupportsGeneratedCause(request) {
		hypothesisCollection["maxItems"] = float64(0)
	}
	hypotheses, err := schemaObject(hypothesisCollection, "items", "properties")
	if err != nil {
		return err
	}
	if shapeErr := specializeHypothesisShape(hypothesisCollection, hypotheses, request); shapeErr != nil {
		return shapeErr
	}
	if enumErr := setSchemaEnum(hypotheses, "code", request.AllowedHypothesisCodes); enumErr != nil {
		return enumErr
	}
	if enumErr := setSchemaEnum(hypotheses, "category", request.AllowedCategories); enumErr != nil {
		return enumErr
	}

	return specializeHypothesisReferences(hypotheses, request, evidenceIDs, findingIDs)
}

func specializeHypothesisShape(
	hypothesisCollection map[string]any,
	hypotheses map[string]any,
	request Request,
) error {
	if request.Stage == AnalysisInitial {
		delete(hypotheses, "resolution")
		hypothesisItem, err := schemaObject(hypothesisCollection, "items")
		if err != nil {
			return err
		}
		removeSchemaRequired(hypothesisItem, "resolution")
	}
	for _, name := range []string{"summary", "root_cause", "explanation", "role"} {
		if _, err := schemaObject(hypotheses, name); err != nil {
			return err
		}
	}
	if request.Stage != AnalysisReview {
		return nil
	}
	for _, name := range []string{"verification", "remediation", "success_condition"} {
		if _, err := schemaObject(hypotheses, "resolution", "properties", name); err != nil {
			return err
		}
	}

	return nil
}

func specializeHypothesisReferences(
	hypotheses map[string]any,
	request Request,
	evidenceIDs []string,
	findingIDs []string,
) error {
	supportingIDs := append(slices.Clone(request.Manifest.ArtifactIDs), request.Manifest.EnrichmentIDs...)
	if len(supportingIDs) == 0 {
		supportingIDs = evidenceIDs
	}
	for name, identifiers := range map[string][]string{
		"supporting_evidence": supportingIDs, "contradicting_evidence": evidenceIDs,
	} {
		field, fieldErr := schemaObject(hypotheses, name)
		if fieldErr != nil {
			return fieldErr
		}
		items, itemsErr := schemaObject(field, "items")
		if itemsErr != nil {
			return itemsErr
		}
		items["enum"] = slices.Clone(identifiers)
	}
	supporting, err := schemaObject(hypotheses, "supporting_evidence")
	if err != nil {
		return err
	}
	supporting["minItems"] = 1
	contradicts, err := schemaObject(hypotheses, "contradicts_findings", "items")
	if err != nil {
		return err
	}
	contradicts["enum"] = slices.Clone(findingIDs)

	return nil
}

func removeSchemaRequired(object map[string]any, name string) {
	required, ok := object["required"].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(required))
	for _, value := range required {
		if value != name {
			filtered = append(filtered, value)
		}
	}
	object["required"] = filtered
}

func requestSupportsGeneratedCause(request Request) bool {
	for _, code := range request.AllowedHypothesisCodes {
		if DirectCauseSignalSupported(code, request.Projection) {
			return true
		}
	}

	return false
}

func specializeActions(properties map[string]any, actionIDs []string) error {
	actions, err := schemaObject(properties, "recommended_action_ids")
	if err != nil {
		return err
	}
	if len(actionIDs) == 0 {
		actions["maxItems"] = 0
	} else {
		actionItems, itemsErr := schemaObject(actions, "items")
		if itemsErr != nil {
			return itemsErr
		}
		actionItems["enum"] = slices.Clone(actionIDs)
	}

	return nil
}

func setSchemaEnum(properties map[string]any, name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("build proposal schema: %s authority is empty", name)
	}
	field, err := schemaObject(properties, name)
	if err != nil {
		return err
	}
	delete(field, "pattern")
	field["enum"] = slices.Clone(values)

	return nil
}

func schemaObject(root map[string]any, path ...string) (map[string]any, error) {
	current := root
	for _, name := range path {
		next, ok := current[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("build proposal schema: reviewed template path %q is invalid", name)
		}
		current = next
	}

	return current, nil
}
