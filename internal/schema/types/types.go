package types

// SchemaType represents the type of schema
type SchemaType string

const (
	// JSON represents JSON Schema format
	JSON SchemaType = "JSON"
	// Avro represents Avro format
	Avro SchemaType = "AVRO"
	// Protobuf represents Protocol Buffers format
	Protobuf SchemaType = "PROTOBUF"
)

// CompatibilityLevel represents the compatibility level for schema evolution
type CompatibilityLevel string

const (
	// Backward compatibility: new schema can read data written with old schema
	Backward CompatibilityLevel = "BACKWARD"
	// Forward compatibility: old schema can read data written with new schema
	Forward CompatibilityLevel = "FORWARD"
	// Full compatibility: both backward and forward compatibility
	Full CompatibilityLevel = "FULL"
	// None: no compatibility checking
	None CompatibilityLevel = "NONE"
	// BackwardTransitive: new schema can read data written with all previous schemas
	BackwardTransitive CompatibilityLevel = "BACKWARD_TRANSITIVE"
	// ForwardTransitive: all previous schemas can read data written with new schema
	ForwardTransitive CompatibilityLevel = "FORWARD_TRANSITIVE"
	// FullTransitive: both backward and forward transitive compatibility
	FullTransitive CompatibilityLevel = "FULL_TRANSITIVE"
)

// SchemaReference represents a reference to another schema
type SchemaReference struct {
	Name    string `json:"name"`    // Reference name
	Subject string `json:"subject"` // Name of the referenced subject
	Version int    `json:"version"` // Version number of the referenced subject
}

// Schema represents a stored schema
type Schema struct {
	Schema     string            `json:"schema"`
	Subject    string            `json:"subject"`
	Version    int               `json:"version"`
	ID         int               `json:"id"`
	Type       SchemaType        `json:"type"`
	References []SchemaReference `json:"references,omitempty"`
}

// SchemaFormat defines the interface for schema format implementations
type SchemaFormat interface {
	// Validate validates a schema string
	Validate(schemaStr string) error
	// Serialize serializes data according to a schema
	Serialize(data interface{}, schemaStr string) ([]byte, error)
	// Deserialize deserializes data according to a schema
	Deserialize(data []byte, schemaStr string) (interface{}, error)
	// CheckCompatibility checks if a new schema is compatible with an old schema
	CheckCompatibility(oldSchema, newSchema string, level CompatibilityLevel) (bool, error)
}
