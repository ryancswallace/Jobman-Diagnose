package provider

import (
	_ "embed"
	"encoding/json"
)

//go:embed proposal.schema.json
var proposalSchema json.RawMessage

// ProposalJSONSchema returns an independent copy of the reviewed schema sent
// to native structured-output backends.
func ProposalJSONSchema() json.RawMessage {
	return append(json.RawMessage(nil), proposalSchema...)
}
