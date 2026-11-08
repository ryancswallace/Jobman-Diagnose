package config

import (
	"reflect"
	"strings"
)

// JSONSchema describes all native YAML field shapes without resolving any
// configuration or credentials. Native Validate remains authoritative for
// provider unions, disclosure limits, transport rules, and file references.
func JSONSchema() map[string]any {
	schema := schemaType(reflect.TypeFor[File]())
	schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	schema["title"] = "Jobman Diagnose configuration schema 2"
	schema["required"] = []string{"schema_version", "defaults", "profiles"}
	schema["description"] = "Validate semantic constraints offline with jobman-diagnose config validate PATH."
	properties, ok := schema["properties"].(map[string]any)
	if ok {
		properties["schema_version"] = map[string]any{"const": SchemaVersion}
	}
	return schema
}

func schemaType(kind reflect.Type) map[string]any {
	if kind.Kind() == reflect.Pointer {
		return schemaType(kind.Elem())
	}
	switch kind.Kind() {
	case reflect.Struct:
		properties := make(map[string]any)
		for index := range kind.NumField() {
			field := kind.Field(index)
			name := strings.Split(field.Tag.Get("yaml"), ",")[0]
			if field.IsExported() && name != "" && name != "-" {
				properties[name] = schemaType(field.Type)
			}
		}
		return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaType(kind.Elem())}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaType(kind.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return map[string]any{"type": "integer"}
	default:
		return map[string]any{"type": "string"}
	}
}
