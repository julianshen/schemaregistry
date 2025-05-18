package avro

import (
	"encoding/json"
	"fmt"

	"schemaregistry/internal/schema/types"

	"github.com/hamba/avro/v2"
)

// Format implements types.SchemaFormat for Avro
type Format struct{}

// New creates a new Avro format implementation
func New() *Format {
	return &Format{}
}

func (f *Format) Validate(schemaStr string) error {
	_, err := avro.Parse(schemaStr)
	return err
}

func (f *Format) Serialize(data interface{}, schemaStr string) ([]byte, error) {
	schema, err := avro.Parse(schemaStr)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	// Convert data to native format if needed
	native, err := f.toNative(data)
	if err != nil {
		return nil, fmt.Errorf("convert to native: %w", err)
	}

	// Serialize to binary
	return avro.Marshal(schema, native)
}

func (f *Format) Deserialize(data []byte, schemaStr string) (interface{}, error) {
	schema, err := avro.Parse(schemaStr)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	// Deserialize from binary
	var native interface{}
	if err := avro.Unmarshal(schema, data, &native); err != nil {
		return nil, fmt.Errorf("deserialize: %w", err)
	}

	return native, nil
}

func (f *Format) CheckCompatibility(oldSchema, newSchema string, level types.CompatibilityLevel) (bool, error) {
	// Parse schemas to get their structure
	oldSchemaMap, err := f.parseSchema(oldSchema)
	if err != nil {
		return false, fmt.Errorf("parse old schema: %w", err)
	}

	newSchemaMap, err := f.parseSchema(newSchema)
	if err != nil {
		return false, fmt.Errorf("parse new schema: %w", err)
	}

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		// For transitive, registry layer will check against all previous versions
		return f.checkBackwardCompatibility(oldSchemaMap, newSchemaMap)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		// For transitive, registry layer will check against all previous versions
		return f.checkForwardCompatibility(oldSchemaMap, newSchemaMap)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		// For transitive, registry layer will check against all previous versions
		backward, err := f.checkBackwardCompatibility(oldSchemaMap, newSchemaMap)
		if err != nil || !backward {
			return false, err
		}
		return f.checkForwardCompatibility(oldSchemaMap, newSchemaMap)
	default:
		return true, nil
	}
}

func (f *Format) toNative(data interface{}) (interface{}, error) {
	// If data is already in native format, return as is
	if _, ok := data.(map[string]interface{}); ok {
		return data, nil
	}

	// Convert to JSON first
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal to JSON: %w", err)
	}

	// Then parse as native format
	var native interface{}
	if err := json.Unmarshal(jsonData, &native); err != nil {
		return nil, fmt.Errorf("unmarshal to native: %w", err)
	}

	return native, nil
}

func (f *Format) parseSchema(schemaStr string) (map[string]interface{}, error) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schemaMap); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return schemaMap, nil
}

func (f *Format) checkBackwardCompatibility(oldSchema, newSchema map[string]interface{}) (bool, error) {
	// Get fields from both schemas
	oldFields := f.getFields(oldSchema)
	newFields := f.getFields(newSchema)

	// Check if all required fields in old schema exist in new schema
	for field, info := range oldFields {
		if info.required {
			if _, exists := newFields[field]; !exists {
				return false, fmt.Errorf("required field %s removed in new schema", field)
			}
		}
	}

	// Check if field types are compatible
	for field, oldInfo := range oldFields {
		if newInfo, exists := newFields[field]; exists {
			if !f.isTypeCompatible(oldInfo.type_, newInfo.type_) {
				return false, fmt.Errorf("incompatible type change for field %s", field)
			}
		}
	}

	return true, nil
}

func (f *Format) checkForwardCompatibility(oldSchema, newSchema map[string]interface{}) (bool, error) {
	// Get fields from both schemas
	oldFields := f.getFields(oldSchema)
	newFields := f.getFields(newSchema)

	// Check if all required fields in new schema exist in old schema
	for field, info := range newFields {
		if info.required {
			if _, exists := oldFields[field]; !exists {
				return false, fmt.Errorf("required field %s added in new schema", field)
			}
		}
	}

	// Check if field types are compatible
	for field, newInfo := range newFields {
		if oldInfo, exists := oldFields[field]; exists {
			if !f.isTypeCompatible(oldInfo.type_, newInfo.type_) {
				return false, fmt.Errorf("incompatible type change for field %s", field)
			}
		}
	}

	return true, nil
}

type fieldInfo struct {
	required bool
	type_    string
}

func (f *Format) getFields(schema map[string]interface{}) map[string]fieldInfo {
	fields := make(map[string]fieldInfo)

	// Extract fields from schema
	if fieldsList, ok := schema["fields"].([]interface{}); ok {
		for _, field := range fieldsList {
			if fieldMap, ok := field.(map[string]interface{}); ok {
				name, _ := fieldMap["name"].(string)
				typeValue := fieldMap["type"]
				required := true // Default to required unless specified as optional

				// Handle type field which can be string or array
				var typeStr string
				switch t := typeValue.(type) {
				case string:
					typeStr = t
				case []interface{}:
					// Check if field is optional (union with null)
					for _, v := range t {
						if v == "null" {
							required = false
						}
						if s, ok := v.(string); ok {
							typeStr = s
						}
					}
				}

				fields[name] = fieldInfo{
					required: required,
					type_:    typeStr,
				}
			}
		}
	}

	return fields
}

func (f *Format) isTypeCompatible(oldType, newType string) bool {
	// Parse both schemas
	oldSchema, err := avro.Parse(oldType)
	if err != nil {
		return false
	}
	newSchema, err := avro.Parse(newType)
	if err != nil {
		return false
	}

	// Get the type names from both schemas
	oldTypeName := oldSchema.Type()
	newTypeName := newSchema.Type()

	// Check primitive type compatibility
	switch oldTypeName {
	case "null":
		return newTypeName == "null"
	case "boolean":
		return newTypeName == "boolean"
	case "int":
		return newTypeName == "int" || newTypeName == "long" || newTypeName == "float" || newTypeName == "double"
	case "long":
		return newTypeName == "long" || newTypeName == "float" || newTypeName == "double"
	case "float":
		return newTypeName == "float" || newTypeName == "double"
	case "double":
		return newTypeName == "double"
	case "bytes":
		return newTypeName == "bytes" || newTypeName == "string"
	case "string":
		return newTypeName == "string"
	case "array":
		if newTypeName != "array" {
			return false
		}
		// Check array element type compatibility
		oldItems := oldSchema.(*avro.ArraySchema).Items()
		newItems := newSchema.(*avro.ArraySchema).Items()
		return f.isTypeCompatible(oldItems.String(), newItems.String())
	case "map":
		if newTypeName != "map" {
			return false
		}
		// Check map value type compatibility
		oldValues := oldSchema.(*avro.MapSchema).Values()
		newValues := newSchema.(*avro.MapSchema).Values()
		return f.isTypeCompatible(oldValues.String(), newValues.String())
	case "record":
		if newTypeName != "record" {
			return false
		}
		// Check record field compatibility
		oldFields := oldSchema.(*avro.RecordSchema).Fields()
		newFields := newSchema.(*avro.RecordSchema).Fields()

		// Create maps for easier field lookup
		newFieldMap := make(map[string]*avro.Field)
		for _, field := range newFields {
			newFieldMap[field.Name()] = field
		}

		// Check each field in the old schema
		for _, oldField := range oldFields {
			newField, exists := newFieldMap[oldField.Name()]
			if !exists {
				// Field was removed
				return false
			}
			if !f.isTypeCompatible(oldField.Type().String(), newField.Type().String()) {
				return false
			}
		}
		return true
	case "enum":
		if newTypeName != "enum" {
			return false
		}
		// Check enum symbol compatibility
		oldSymbols := oldSchema.(*avro.EnumSchema).Symbols()
		newSymbols := newSchema.(*avro.EnumSchema).Symbols()

		// Create map for easier symbol lookup
		newSymbolMap := make(map[string]bool)
		for _, symbol := range newSymbols {
			newSymbolMap[symbol] = true
		}

		// Check if all old symbols exist in new schema
		for _, symbol := range oldSymbols {
			if !newSymbolMap[symbol] {
				return false
			}
		}
		return true
	case "union":
		if newTypeName != "union" {
			return false
		}
		// Check union type compatibility
		oldTypes := oldSchema.(*avro.UnionSchema).Types()
		newTypes := newSchema.(*avro.UnionSchema).Types()

		// Create map for easier type lookup
		newTypeMap := make(map[string]bool)
		for _, t := range newTypes {
			newTypeMap[t.String()] = true
		}

		// Check if all old types exist in new schema
		for _, t := range oldTypes {
			if !newTypeMap[t.String()] {
				return false
			}
		}
		return true
	default:
		return false
	}
}
