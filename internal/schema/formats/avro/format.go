package avro

import (
	"encoding/json"
	"fmt"

	"schemaregistry/internal/schema/types"

	"github.com/hamba/avro/v2"
)

// Format implements types.SchemaFormat for Avro
type Format struct{}

// fieldInfo represents information about an Avro field
type fieldInfo struct {
	required bool
	type_    string
}

// New creates a new Avro format implementation
func New() *Format {
	return &Format{}
}

func (f *Format) Validate(schemaStr string) error {
	// Parse schema
	_, err := avro.Parse(schemaStr)
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	return nil
}

func (f *Format) Serialize(data interface{}, schemaStr string) ([]byte, error) {
	// Parse schema
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
	// Parse schema
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
	// Parse schemas
	oldAvroSchema, err := avro.Parse(oldSchema)
	if err != nil {
		return false, fmt.Errorf("parse old schema: %w", err)
	}

	newAvroSchema, err := avro.Parse(newSchema)
	if err != nil {
		return false, fmt.Errorf("parse new schema: %w", err)
	}

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		return f.isBackwardCompatible(oldAvroSchema, newAvroSchema)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		return f.isForwardCompatible(oldAvroSchema, newAvroSchema)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		backward, err := f.isBackwardCompatible(oldAvroSchema, newAvroSchema)
		if err != nil || !backward {
			return false, err
		}
		return f.isForwardCompatible(oldAvroSchema, newAvroSchema)
	case types.None:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported compatibility level: %s", level)
	}
}

// isBackwardCompatible checks if new schema can read data written with old schema
func (f *Format) isBackwardCompatible(oldSchema, newSchema avro.Schema) (bool, error) {
	// Get fields from both schemas
	oldFields := f.getFields(oldSchema)
	newFields := f.getFields(newSchema)

	// Check each field in the old schema
	for name, oldField := range oldFields {
		newField, exists := newFields[name]
		if !exists {
			// Field was removed
			if oldField.required {
				return false, fmt.Errorf("required field %s was removed", name)
			}
			continue
		}

		// Check type compatibility
		if !f.isTypeCompatible(oldField.type_, newField.type_) {
			return false, fmt.Errorf("incompatible types for field %s: %s -> %s", name, oldField.type_, newField.type_)
		}

		// Check if field became required
		if !oldField.required && newField.required {
			return false, fmt.Errorf("field %s became required", name)
		}
	}

	return true, nil
}

// isForwardCompatible checks if old schema can read data written with new schema
func (f *Format) isForwardCompatible(oldSchema, newSchema avro.Schema) (bool, error) {
	// Get fields from both schemas
	oldFields := f.getFields(oldSchema)
	newFields := f.getFields(newSchema)

	// Check each field in the new schema
	for name, newField := range newFields {
		oldField, exists := oldFields[name]
		if !exists {
			// New field was added
			if newField.required {
				return false, fmt.Errorf("new required field %s was added", name)
			}
			continue
		}

		// Check type compatibility
		if !f.isTypeCompatible(newField.type_, oldField.type_) {
			return false, fmt.Errorf("incompatible types for field %s: %s -> %s", name, newField.type_, oldField.type_)
		}

		// Check if field became optional
		if oldField.required && !newField.required {
			return false, fmt.Errorf("field %s became optional", name)
		}
	}

	return true, nil
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

func (f *Format) getFields(schema avro.Schema) map[string]fieldInfo {
	fields := make(map[string]fieldInfo)

	// Check if schema is a record type
	recordSchema, ok := schema.(*avro.RecordSchema)
	if !ok {
		return fields
	}

	// Extract fields from record schema
	for _, field := range recordSchema.Fields() {
		name := field.Name()
		typeValue := field.Type()
		required := true // Default to required unless specified as optional

		// Handle type field
		var typeStr string
		switch t := typeValue.(type) {
		case *avro.UnionSchema:
			// Check if field is optional (union with null)
			for _, v := range t.Types() {
				if v.Type() == avro.Null {
					required = false
				} else {
					typeStr = string(v.Type())
				}
			}
		default:
			typeStr = string(typeValue.Type())
		}

		fields[name] = fieldInfo{
			required: required,
			type_:    typeStr,
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
