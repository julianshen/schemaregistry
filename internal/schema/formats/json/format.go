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
	// Parse schema
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader([]byte(schemaStr))); err != nil {
		return fmt.Errorf("add schema resource: %w", err)
	}

	// Compile schema
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}

	// Validate schema
	if err := schema.Validate(nil); err != nil {
		return fmt.Errorf("validate schema: %w", err)
	}

	return nil
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
	// Parse schemas
	oldProps := f.getSchemaProperties(oldSchema)
	newProps := f.getSchemaProperties(newSchema)

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		return f.isBackwardCompatible(oldProps, newProps)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		return f.isForwardCompatible(oldProps, newProps)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		backward, err := f.isBackwardCompatible(oldProps, newProps)
		if err != nil || !backward {
			return false, err
		}
		return f.isForwardCompatible(oldProps, newProps)
	case types.None:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported compatibility level: %s", level)
	}
}

// isBackwardCompatible checks if new schema can read data written with old schema
func (f *Format) isBackwardCompatible(oldProps, newProps map[string]propertyInfo) (bool, error) {
	// Check each property in the old schema
	for name, oldProp := range oldProps {
		newProp, exists := newProps[name]
		if !exists {
			// Property was removed
			if oldProp.required {
				return false, fmt.Errorf("required property %s was removed", name)
			}
			continue
		}

		// Check type compatibility
		if !f.isTypeCompatible(oldProp.type_, newProp.type_) {
			return false, fmt.Errorf("incompatible types for property %s: %s -> %s", name, oldProp.type_, newProp.type_)
		}

		// Check if property became required
		if !oldProp.required && newProp.required {
			return false, fmt.Errorf("property %s became required", name)
		}
	}

	return true, nil
}

// isForwardCompatible checks if old schema can read data written with new schema
func (f *Format) isForwardCompatible(oldProps, newProps map[string]propertyInfo) (bool, error) {
	// Check each property in the new schema
	for name, newProp := range newProps {
		oldProp, exists := oldProps[name]
		if !exists {
			// New property was added
			if newProp.required {
				return false, fmt.Errorf("new required property %s was added", name)
			}
			continue
		}

		// Check type compatibility
		if !f.isTypeCompatible(newProp.type_, oldProp.type_) {
			return false, fmt.Errorf("incompatible types for property %s: %s -> %s", name, newProp.type_, oldProp.type_)
		}

		// Check if property became optional
		if oldProp.required && !newProp.required {
			return false, fmt.Errorf("property %s became optional", name)
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
