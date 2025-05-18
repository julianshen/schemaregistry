package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

	"schemaregistry/internal/schema/types"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Format implements types.SchemaFormat for JSON Schema
type Format struct{}

// New creates a new JSON format implementation
func New() *Format {
	return &Format{}
}

func (f *Format) Validate(schemaStr string) error {
	_, err := jsonschema.CompileString("schema.json", schemaStr)
	return err
}

func (f *Format) Serialize(data interface{}, schemaStr string) ([]byte, error) {
	// Compile schema
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader([]byte(schemaStr))); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}

	// Validate data against schema
	if err := schema.Validate(data); err != nil {
		return nil, fmt.Errorf("validate data: %w", err)
	}

	return json.Marshal(data)
}

func (f *Format) Deserialize(data []byte, schemaStr string) (interface{}, error) {
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	// Compile schema
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader([]byte(schemaStr))); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}

	// Validate data against schema
	if err := schema.Validate(result); err != nil {
		return nil, fmt.Errorf("validate data: %w", err)
	}

	return result, nil
}

func (f *Format) CheckCompatibility(oldSchema, newSchema string, level types.CompatibilityLevel) (bool, error) {
	// Compile both schemas
	oldCompiler := jsonschema.NewCompiler()
	if err := oldCompiler.AddResource("old.json", bytes.NewReader([]byte(oldSchema))); err != nil {
		return false, fmt.Errorf("add old schema resource: %w", err)
	}
	oldSchemaObj, err := oldCompiler.Compile("old.json")
	if err != nil {
		return false, fmt.Errorf("compile old schema: %w", err)
	}

	newCompiler := jsonschema.NewCompiler()
	if err := newCompiler.AddResource("new.json", bytes.NewReader([]byte(newSchema))); err != nil {
		return false, fmt.Errorf("add new schema resource: %w", err)
	}
	newSchemaObj, err := newCompiler.Compile("new.json")
	if err != nil {
		return false, fmt.Errorf("compile new schema: %w", err)
	}

	slog.Debug("CheckCompatibility called", "level", level, "oldSchema", oldSchema, "newSchema", newSchema)

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		return f.checkBackwardCompatibility(oldSchemaObj, newSchemaObj, oldSchema, newSchema)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		return f.checkForwardCompatibility(oldSchemaObj, newSchemaObj, oldSchema, newSchema)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		backward, err := f.checkBackwardCompatibility(oldSchemaObj, newSchemaObj, oldSchema, newSchema)
		if err != nil || !backward {
			return false, err
		}
		return f.checkForwardCompatibility(oldSchemaObj, newSchemaObj, oldSchema, newSchema)
	default:
		return true, nil
	}
}

func (f *Format) checkBackwardCompatibility(oldSchema, newSchema *jsonschema.Schema, oldSchemaStr, newSchemaStr string) (bool, error) {
	oldProps := f.getSchemaProperties(oldSchemaStr)
	newProps := f.getSchemaProperties(newSchemaStr)

	slog.Debug("checkBackwardCompatibility: oldProps", "props", oldProps)
	slog.Debug("checkBackwardCompatibility: newProps", "props", newProps)

	// Check if all required properties in old schema exist in new schema
	for prop, info := range oldProps {
		if info.required {
			if _, exists := newProps[prop]; !exists {
				slog.Debug("Property missing in new schema", "property", prop)
				return false, fmt.Errorf("required property %s removed in new schema", prop)
			}
		}
	}

	// Check if property types are compatible
	for prop, oldInfo := range oldProps {
		if newInfo, exists := newProps[prop]; exists {
			if !f.isTypeCompatible(oldInfo.type_, newInfo.type_) {
				slog.Debug("Type mismatch detected", "property", prop, "oldType", oldInfo.type_, "newType", newInfo.type_)
				return false, fmt.Errorf("incompatible type change for property %s", prop)
			}
		}
	}

	return true, nil
}

func (f *Format) checkForwardCompatibility(oldSchema, newSchema *jsonschema.Schema, oldSchemaStr, newSchemaStr string) (bool, error) {
	oldProps := f.getSchemaProperties(oldSchemaStr)
	newProps := f.getSchemaProperties(newSchemaStr)

	// Check if all required properties in new schema exist in old schema
	for prop, info := range newProps {
		if info.required {
			if _, exists := oldProps[prop]; !exists {
				return false, fmt.Errorf("required property %s added in new schema", prop)
			}
		}
	}

	// Check if property types are compatible
	for prop, newInfo := range newProps {
		if oldInfo, exists := oldProps[prop]; exists {
			if !f.isTypeCompatible(oldInfo.type_, newInfo.type_) {
				return false, fmt.Errorf("incompatible type change for property %s", prop)
			}
		}
	}

	return true, nil
}

type propertyInfo struct {
	required bool
	type_    string
}

func (f *Format) getSchemaProperties(schemaStr string) map[string]propertyInfo {
	props := make(map[string]propertyInfo)

	// Parse schema JSON
	var schemaMap map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schemaMap); err != nil {
		return props
	}

	// Extract properties
	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		required := make(map[string]bool)
		if requiredProps, ok := schemaMap["required"].([]interface{}); ok {
			for _, req := range requiredProps {
				if name, ok := req.(string); ok {
					required[name] = true
				}
			}
		}

		for name, prop := range properties {
			if propMap, ok := prop.(map[string]interface{}); ok {
				type_ := "object" // default type
				if t, ok := propMap["type"].(string); ok {
					type_ = t
				}

				props[name] = propertyInfo{
					required: required[name],
					type_:    type_,
				}
			}
		}
	}

	return props
}

func (f *Format) isTypeCompatible(oldType, newType string) bool {
	slog.Debug("isTypeCompatible called", "oldType", oldType, "newType", newType)
	// Handle type compatibility rules
	switch oldType {
	case "null":
		return newType == "null"
	case "boolean":
		return newType == "boolean"
	case "integer":
		return newType == "integer" // Don't allow integer -> number conversion
	case "number":
		return newType == "number"
	case "string":
		return newType == "string"
	case "array":
		return newType == "array"
	case "object":
		return newType == "object"
	default:
		return false
	}
}
