package protobuf

import (
	"fmt"

	"schemaregistry/internal/schema/types"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Format implements types.SchemaFormat for Protobuf
type Format struct{}

// New creates a new Protobuf format implementation
func New() *Format {
	return &Format{}
}

func (f *Format) Validate(schemaStr string) error {
	// Parse the schema string as a FileDescriptorProto
	var fileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(schemaStr), &fileDescProto); err != nil {
		return fmt.Errorf("unmarshal schema: %w", err)
	}

	// Create a FileDescriptor from the FileDescriptorProto
	_, err := protodesc.NewFile(&fileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return fmt.Errorf("create file descriptor: %w", err)
	}

	return nil
}

func (f *Format) Serialize(data interface{}, schemaStr string) ([]byte, error) {
	// Parse the schema string as a FileDescriptorProto
	var fileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(schemaStr), &fileDescProto); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	// Create a FileDescriptor from the FileDescriptorProto
	fileDesc, err := protodesc.NewFile(&fileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create file descriptor: %w", err)
	}

	// Get the first message type from the file
	messageType := fileDesc.Messages().Get(0)
	if messageType == nil {
		return nil, fmt.Errorf("no message type found in schema")
	}

	// Create a dynamic message
	message := dynamicpb.NewMessage(messageType)

	// Convert data to protobuf message
	if err := f.toProtoMessage(message, data); err != nil {
		return nil, fmt.Errorf("convert to proto message: %w", err)
	}

	// Serialize the message
	return proto.Marshal(message)
}

func (f *Format) Deserialize(data []byte, schemaStr string) (interface{}, error) {
	// Parse the schema string as a FileDescriptorProto
	var fileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(schemaStr), &fileDescProto); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	// Create a FileDescriptor from the FileDescriptorProto
	fileDesc, err := protodesc.NewFile(&fileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create file descriptor: %w", err)
	}

	// Get the first message type from the file
	messageType := fileDesc.Messages().Get(0)
	if messageType == nil {
		return nil, fmt.Errorf("no message type found in schema")
	}

	// Create a dynamic message
	message := dynamicpb.NewMessage(messageType)

	// Unmarshal the data into the message
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("unmarshal proto message: %w", err)
	}

	// Convert the message to a map
	return f.fromProtoMessage(message), nil
}

func (f *Format) CheckCompatibility(oldSchema, newSchema string, level types.CompatibilityLevel) (bool, error) {
	// Parse schemas
	oldFileDesc, err := f.parseSchema(oldSchema)
	if err != nil {
		return false, fmt.Errorf("parse old schema: %w", err)
	}

	newFileDesc, err := f.parseSchema(newSchema)
	if err != nil {
		return false, fmt.Errorf("parse new schema: %w", err)
	}

	// Get message types
	oldMessageType := oldFileDesc.Messages().Get(0)
	if oldMessageType == nil {
		return false, fmt.Errorf("no message type found in old schema")
	}

	newMessageType := newFileDesc.Messages().Get(0)
	if newMessageType == nil {
		return false, fmt.Errorf("no message type found in new schema")
	}

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		return f.isBackwardCompatible(oldMessageType, newMessageType)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		return f.isForwardCompatible(oldMessageType, newMessageType)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		backward, err := f.isBackwardCompatible(oldMessageType, newMessageType)
		if err != nil || !backward {
			return false, err
		}
		return f.isForwardCompatible(oldMessageType, newMessageType)
	case types.None:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported compatibility level: %s", level)
	}
}

// isBackwardCompatible checks if new schema can read data written with old schema
func (f *Format) isBackwardCompatible(oldMessage, newMessage protoreflect.MessageDescriptor) (bool, error) {
	// Get fields from both messages
	oldFields := f.getFields(oldMessage)
	newFields := f.getFields(newMessage)

	// Check each field in the old message
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
func (f *Format) isForwardCompatible(oldMessage, newMessage protoreflect.MessageDescriptor) (bool, error) {
	// Get fields from both messages
	oldFields := f.getFields(oldMessage)
	newFields := f.getFields(newMessage)

	// Check each field in the new message
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

// parseSchema parses a protobuf schema string into a FileDescriptor
func (f *Format) parseSchema(schemaStr string) (protoreflect.FileDescriptor, error) {
	var fileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(schemaStr), &fileDescProto); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	fileDesc, err := protodesc.NewFile(&fileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create file descriptor: %w", err)
	}

	return fileDesc, nil
}

type fieldInfo struct {
	required bool
	type_    string
}

func (f *Format) getFields(message protoreflect.MessageDescriptor) map[string]fieldInfo {
	fields := make(map[string]fieldInfo)

	// Extract fields from message
	for i := 0; i < message.Fields().Len(); i++ {
		field := message.Fields().Get(i)
		name := string(field.Name())
		required := field.IsRequired()
		type_ := string(field.Kind())

		fields[name] = fieldInfo{
			required: required,
			type_:    type_,
		}
	}

	return fields
}

func (f *Format) isTypeCompatible(oldType, newType string) bool {
	// Handle type compatibility rules
	switch oldType {
	case "double":
		return newType == "double"
	case "float":
		return newType == "float" || newType == "double"
	case "int32":
		return newType == "int32" || newType == "int64" || newType == "uint32" || newType == "uint64" || newType == "sint32" || newType == "sint64" || newType == "fixed32" || newType == "fixed64" || newType == "sfixed32" || newType == "sfixed64"
	case "int64":
		return newType == "int64" || newType == "uint64" || newType == "sint64" || newType == "fixed64" || newType == "sfixed64"
	case "uint32":
		return newType == "uint32" || newType == "uint64" || newType == "fixed32" || newType == "fixed64"
	case "uint64":
		return newType == "uint64" || newType == "fixed64"
	case "sint32":
		return newType == "sint32" || newType == "sint64" || newType == "int32" || newType == "int64"
	case "sint64":
		return newType == "sint64" || newType == "int64"
	case "fixed32":
		return newType == "fixed32" || newType == "fixed64" || newType == "uint32" || newType == "uint64"
	case "fixed64":
		return newType == "fixed64" || newType == "uint64"
	case "sfixed32":
		return newType == "sfixed32" || newType == "sfixed64" || newType == "int32" || newType == "int64"
	case "sfixed64":
		return newType == "sfixed64" || newType == "int64"
	case "bool":
		return newType == "bool"
	case "string":
		return newType == "string"
	case "bytes":
		return newType == "bytes"
	case "enum":
		return newType == "enum"
	case "message":
		return newType == "message"
	default:
		return false
	}
}

func (f *Format) toProtoMessage(message protoreflect.Message, data interface{}) error {
	// TODO: Implement conversion from data to protobuf message
	// This is a simplified version that does nothing
	return nil
}

func (f *Format) fromProtoMessage(message protoreflect.Message) interface{} {
	// TODO: Implement conversion from protobuf message to data
	// This is a simplified version that returns nil
	return nil
}
