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
	// Parse both schemas as FileDescriptorProto
	var oldFileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(oldSchema), &oldFileDescProto); err != nil {
		return false, fmt.Errorf("unmarshal old schema: %w", err)
	}

	var newFileDescProto descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(newSchema), &newFileDescProto); err != nil {
		return false, fmt.Errorf("unmarshal new schema: %w", err)
	}

	// Create FileDescriptors from the FileDescriptorProtos
	oldFileDesc, err := protodesc.NewFile(&oldFileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return false, fmt.Errorf("create old file descriptor: %w", err)
	}

	newFileDesc, err := protodesc.NewFile(&newFileDescProto, protoregistry.GlobalFiles)
	if err != nil {
		return false, fmt.Errorf("create new file descriptor: %w", err)
	}

	// Check compatibility based on level
	switch level {
	case types.Backward, types.BackwardTransitive:
		// New schema can read data written with old schema
		return f.checkBackwardCompatibility(oldFileDesc, newFileDesc)
	case types.Forward, types.ForwardTransitive:
		// Old schema can read data written with new schema
		return f.checkForwardCompatibility(oldFileDesc, newFileDesc)
	case types.Full, types.FullTransitive:
		// Both backward and forward compatibility
		backward, err := f.checkBackwardCompatibility(oldFileDesc, newFileDesc)
		if err != nil || !backward {
			return false, err
		}
		return f.checkForwardCompatibility(oldFileDesc, newFileDesc)
	default:
		return true, nil
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

func (f *Format) checkBackwardCompatibility(oldFileDesc, newFileDesc protoreflect.FileDescriptor) (bool, error) {
	// Get all message types from both schemas
	oldMessages := f.getMessageTypes(oldFileDesc)
	newMessages := f.getMessageTypes(newFileDesc)

	// Check if all required fields in old messages exist in new messages
	for name, oldMsg := range oldMessages {
		if newMsg, exists := newMessages[name]; exists {
			if compatible, err := f.areMessagesCompatible(oldMsg, newMsg); !compatible {
				return false, err
			}
		} else {
			return false, fmt.Errorf("message %s removed in new schema", name)
		}
	}

	return true, nil
}

func (f *Format) checkForwardCompatibility(oldFileDesc, newFileDesc protoreflect.FileDescriptor) (bool, error) {
	// Get all message types from both schemas
	oldMessages := f.getMessageTypes(oldFileDesc)
	newMessages := f.getMessageTypes(newFileDesc)

	// Check if all required fields in new messages exist in old messages
	for name, newMsg := range newMessages {
		if oldMsg, exists := oldMessages[name]; exists {
			if compatible, err := f.areMessagesCompatible(oldMsg, newMsg); !compatible {
				return false, err
			}
		} else {
			return false, fmt.Errorf("message %s added in new schema", name)
		}
	}

	return true, nil
}

func (f *Format) getMessageTypes(fileDesc protoreflect.FileDescriptor) map[string]protoreflect.MessageDescriptor {
	messages := make(map[string]protoreflect.MessageDescriptor)
	for i := 0; i < fileDesc.Messages().Len(); i++ {
		msg := fileDesc.Messages().Get(i)
		messages[string(msg.Name())] = msg
	}
	return messages
}

func (f *Format) areMessagesCompatible(oldMsg, newMsg protoreflect.MessageDescriptor) (bool, error) {
	// Check all fields in old message
	for i := 0; i < oldMsg.Fields().Len(); i++ {
		oldField := oldMsg.Fields().Get(i)
		newField := newMsg.Fields().ByNumber(oldField.Number())

		// Field must exist in new message
		if newField == nil {
			return false, fmt.Errorf("field %s removed in new schema", oldField.Name())
		}

		// Field types must be compatible
		if oldField.Kind() != newField.Kind() {
			return false, fmt.Errorf("incompatible type change for field %s", oldField.Name())
		}

		// If field is a message, recursively check compatibility
		if oldField.Kind() == protoreflect.MessageKind {
			oldSubMsg := oldField.Message()
			newSubMsg := newField.Message()
			if compatible, err := f.areMessagesCompatible(oldSubMsg, newSubMsg); !compatible {
				return false, err
			}
		}

		// Check cardinality (required/optional/repeated)
		if oldField.Cardinality() != newField.Cardinality() {
			return false, fmt.Errorf("cardinality change for field %s", oldField.Name())
		}
	}

	return true, nil
}
