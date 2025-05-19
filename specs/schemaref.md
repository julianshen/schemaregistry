# Schema References in Confluent Schema Registry

Schema references are a powerful feature in Confluent Schema Registry that allow you to create relationships between schemas. They enable modular schema design and reuse of schema components across your organization.

## What Are Schema References?

Schema references allow one schema to reference another schema by specifying the subject, version, and a name for the reference. This creates a formal relationship between schemas, similar to importing modules or including header files in programming languages.

## Key Benefits

1. **Modularity**: Break complex schemas into smaller, reusable components
2. **Consistency**: Maintain shared data models across the organization
3. **Reduced Duplication**: Define common structures once and reuse them
4. **Better Evolution**: Update common components in one place
5. **Smaller Messages**: Avoid repeating common schema definitions

## Supported Formats

Schema references are supported in:
- Avro (using named types)
- Protobuf (using imports)
- JSON Schema (using $ref)

## How Schema References Work

When a schema with references is registered:

1. The referenced schemas must already exist in Schema Registry
2. The main schema includes metadata about which schemas it references
3. Schema Registry validates that all references are valid
4. When serializing/deserializing, all referenced schemas are considered

## Example in Avro

Here's how you can create and use schema references in Avro:

### 1. Create and Register an Address Schema

```json
{
  "type": "record",
  "name": "Address",
  "namespace": "com.example",
  "fields": [
    {"name": "street", "type": "string"},
    {"name": "city", "type": "string"},
    {"name": "zipCode", "type": "string"}
  ]
}
```

Register it to subject `com.example.Address`:
```bash
curl -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
  --data '{"schema":"{\"type\":\"record\",\"name\":\"Address\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"street\",\"type\":\"string\"},{\"name\":\"city\",\"type\":\"string\"},{\"name\":\"zipCode\",\"type\":\"string\"}]}"}' \
  http://localhost:8081/subjects/com.example.Address/versions
```

### 2. Create and Register a User Schema with Reference

```json
{
  "type": "record",
  "name": "User",
  "namespace": "com.example",
  "fields": [
    {"name": "id", "type": "int"},
    {"name": "name", "type": "string"},
    {"name": "address", "type": "com.example.Address"}
  ]
}
```

Register it with a reference to the Address schema:
```bash
curl -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
  --data '{
    "schema": "{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"address\",\"type\":\"com.example.Address\"}]}",
    "references": [
      {
        "name": "com.example.Address",
        "subject": "com.example.Address",
        "version": 1
      }
    ]
  }' \
  http://localhost:8081/subjects/com.example.User/versions
```

## Example in Protobuf

Protobuf has native support for imports:

### 1. Create and Register Address.proto

```protobuf
syntax = "proto3";
package com.example;

message Address {
  string street = 1;
  string city = 2;
  string zip_code = 3;
}
```

### 2. Create and Register User.proto with Import

```protobuf
syntax = "proto3";
package com.example;

import "Address.proto";

message User {
  int32 id = 1;
  string name = 2;
  com.example.Address address = 3;
}
```

## Example in JSON Schema

JSON Schema uses `$ref` to reference other schemas:

### 1. Create and Register Address JSON Schema

```json
{
  "$id": "https://example.com/address.schema.json",
  "type": "object",
  "properties": {
    "street": { "type": "string" },
    "city": { "type": "string" },
    "zipCode": { "type": "string" }
  },
  "required": ["street", "city", "zipCode"]
}
```

### 2. Create and Register User JSON Schema with Reference

```json
{
  "$id": "https://example.com/user.schema.json",
  "type": "object",
  "properties": {
    "id": { "type": "integer" },
    "name": { "type": "string" },
    "address": { "$ref": "https://example.com/address.schema.json" }
  },
  "required": ["id", "name", "address"]
}
```

## Schema References and Compatibility

When checking compatibility with schemas that have references:

1. Schema Registry fetches all referenced schemas
2. Compatibility is checked against the complete schema (including all references)
3. Changes to referenced schemas can impact compatibility

## Best Practices for Schema References

1. **Version Control**: Carefully manage versions of referenced schemas
2. **Compatibility Settings**: Consider how changes to referenced schemas affect consumers
3. **Documentation**: Document dependencies between schemas
4. **Organization**: Develop a naming convention for schemas and subjects
5. **Evolution Strategy**: Plan how referenced schemas will evolve

Schema references are a powerful tool for building modular, maintainable data models that promote reuse across your organization while keeping the benefits of strong schema typing and compatibility checking.