package models

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed rca_report_schema.yaml
var rcaSchemaYAML []byte

//go:embed chat_response_schema.yaml
var chatSchemaYAML []byte

// RCAReportSchema returns the RCA report JSON Schema as a map,
// ready to pass to agent.StructuredOutput.Schema.
func RCAReportSchema() (map[string]any, error) {
	return SchemaFromYAML(rcaSchemaYAML)
}

// ChatResponseSchema returns the chat response JSON Schema as a map.
func ChatResponseSchema() (map[string]any, error) {
	return SchemaFromYAML(chatSchemaYAML)
}

// SchemaFromYAML parses a YAML-encoded JSON Schema into a map[string]any.
// This can be used for any embedded YAML schema, not just the RCA report.
func SchemaFromYAML(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
