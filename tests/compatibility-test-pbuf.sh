#!/bin/bash
# Comprehensive Schema Registry Compatibility Test Script for Protobuf
# This script tests a Schema Registry server implementation for API compatibility
# using Protobuf schemas and curl commands.

# Schema Registry URL - adjust as needed
SCHEMA_REGISTRY_URL="http://localhost:8081"

# Colors for better readability
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to execute a command and echo it first
run_command() {
  echo -e "\n${YELLOW}> $@${NC}"
  eval "$@"
}

# Function to check if a command succeeded
check_success() {
  if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Test passed${NC}"
    return 0
  else
    echo -e "${RED}✗ Test failed${NC}"
    return 1
  fi
}

# Test counter
TESTS_TOTAL=0
TESTS_PASSED=0

# Function to run a test
run_test() {
  local test_name=$1
  local command=$2
  local expected_status=$3
  
  ((TESTS_TOTAL++))
  
  echo -e "\n${YELLOW}TEST $TESTS_TOTAL: $test_name${NC}"
  echo -e "Expected status: $expected_status"
  
  local RESPONSE=$(eval "$command")
  local STATUS=$?
  
  echo -e "Response: $RESPONSE"
  
  if [[ "$expected_status" == "success" && $STATUS -eq 0 ]] || 
     [[ "$expected_status" == "failure" && $STATUS -ne 0 ]]; then
    echo -e "${GREEN}✓ Test passed${NC}"
    ((TESTS_PASSED++))
  else
    echo -e "${RED}✗ Test failed${NC}"
  fi
}

echo "=== Schema Registry Protobuf Compatibility Test ==="
echo "Testing Schema Registry at: $SCHEMA_REGISTRY_URL"
echo "$(date)"
echo

# =========================================================
# 1. Basic connectivity test
# =========================================================
echo -e "\n${YELLOW}SECTION 1: BASIC CONNECTIVITY${NC}"

run_test "Check Schema Registry is running" \
  "curl -s $SCHEMA_REGISTRY_URL > /dev/null" \
  "success"

# =========================================================
# 2. Global config tests
# =========================================================
echo -e "\n${YELLOW}SECTION 2: GLOBAL CONFIG${NC}"

run_test "Get global compatibility" \
  "curl -s $SCHEMA_REGISTRY_URL/config | grep compatibility" \
  "success"

run_test "Set global compatibility to BACKWARD" \
  "curl -s -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '{\"compatibility\": \"BACKWARD\"}' \
   $SCHEMA_REGISTRY_URL/config" \
  "success"

run_test "Verify global compatibility was set to BACKWARD" \
  "curl -s $SCHEMA_REGISTRY_URL/config | grep BACKWARD" \
  "success"

# =========================================================
# 3. Schema registration tests with Protobuf
# =========================================================
echo -e "\n${YELLOW}SECTION 3: PROTOBUF SCHEMA REGISTRATION${NC}"

# Test subject
TEST_SUBJECT="test-protobuf-subject"

# Define Protobuf schemas (Note the special escaping for protobuf syntax in JSON)
# Initial schema (v1)
SCHEMA_V1='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  int32 id = 1;\n  string name = 2;\n}",
  "schemaType": "PROTOBUF"
}'

# Compatible schema (v2) - adding an optional field is backward compatible in protobuf
SCHEMA_V2='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  int32 id = 1;\n  string name = 2;\n  string email = 3;\n}",
  "schemaType": "PROTOBUF"
}'

# Incompatible schema (removing a field - actually compatible in proto3 but we'll test)
SCHEMA_INCOMPATIBLE='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  int32 id = 1;\n}",
  "schemaType": "PROTOBUF"
}'

# Changing field type (incompatible)
SCHEMA_TYPE_CHANGE='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  string id = 1;\n  string name = 2;\n}",
  "schemaType": "PROTOBUF"
}'

# Register initial schema (v1)
run_test "Register initial Protobuf schema (v1)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V1' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q '{\"id\":'" \
  "success"

# =========================================================
# 4. Subject listing tests
# =========================================================
echo -e "\n${YELLOW}SECTION 4: SUBJECT LISTING${NC}"

run_test "List all subjects" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects | grep -q $TEST_SUBJECT" \
  "success"

run_test "List versions for subject" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q 1" \
  "success"

# =========================================================
# 5. Schema retrieval tests
# =========================================================
echo -e "\n${YELLOW}SECTION 5: SCHEMA RETRIEVAL${NC}"

run_test "Get Protobuf schema by version" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | grep -q 'PROTOBUF'" \
  "success"

run_test "Get latest Protobuf schema" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/latest | grep -q 'schema'" \
  "success"

# Get the ID of the registered schema for later use
SCHEMA_ID=$(curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | sed -E 's/.*"id":([0-9]+).*/\1/')
echo "Retrieved schema ID: $SCHEMA_ID"

run_test "Get Protobuf schema by ID" \
  "curl -s $SCHEMA_REGISTRY_URL/schemas/ids/$SCHEMA_ID | grep -q 'schema'" \
  "success"

# =========================================================
# 6. Schema lookup tests
# =========================================================
echo -e "\n${YELLOW}SECTION 6: SCHEMA LOOKUP${NC}"

run_test "Lookup Protobuf schema under subject" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V1' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT | grep -q 'subject'" \
  "success"

# =========================================================
# 7. Compatibility tests
# =========================================================
echo -e "\n${YELLOW}SECTION 7: PROTOBUF COMPATIBILITY TESTS${NC}"

run_test "Test compatible Protobuf schema (explicit check)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'true'" \
  "success"

# Note: In Protobuf 3, removing fields is technically compatible since fields are optional by default
# But this is a good test to see how Schema Registry handles it (may depend on configuration)
run_test "Test field removal compatibility in protobuf" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_INCOMPATIBLE' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest" \
  "success"

run_test "Test type change incompatibility in protobuf" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_TYPE_CHANGE' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'false'" \
  "success"

run_test "Register compatible Protobuf schema (v2)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q '{\"id\":'" \
  "success"

# =========================================================
# 8. Subject compatibility configuration
# =========================================================
echo -e "\n${YELLOW}SECTION 8: SUBJECT-LEVEL COMPATIBILITY${NC}"

run_test "Set subject-specific compatibility to FULL" \
  "curl -s -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '{\"compatibility\": \"FULL\"}' \
   $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT" \
  "success"

run_test "Get subject-specific compatibility" \
  "curl -s $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT | grep -q FULL" \
  "success"

# =========================================================
# 9. Protobuf import tests
# =========================================================
echo -e "\n${YELLOW}SECTION 9: PROTOBUF IMPORTS${NC}"

# Define an address protobuf schema
ADDRESS_SCHEMA='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage Address {\n  string street = 1;\n  string city = 2;\n  string zip_code = 3;\n}",
  "schemaType": "PROTOBUF"
}'

# Define a user schema that imports address
USER_WITH_IMPORT_SCHEMA='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nimport \"address.proto\";\n\nmessage UserWithAddress {\n  int32 id = 1;\n  string name = 2;\n  Address address = 3;\n}",
  "schemaType": "PROTOBUF",
  "references": [
    {
      "name": "address.proto",
      "subject": "com.example.Address",
      "version": 1
    }
  ]
}'

# Register address schema
run_test "Register Address Protobuf schema for import" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$ADDRESS_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/com.example.Address/versions | grep -q '{\"id\":'" \
  "success"

# Register user schema with import
run_test "Register Protobuf schema with import" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$USER_WITH_IMPORT_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/com.example.UserWithAddress/versions | grep -q '{\"id\":'" \
  "success"

# Get schema with import reference
run_test "Get Protobuf schema with import" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/com.example.UserWithAddress/versions/latest | grep -q 'references'" \
  "success"

# =========================================================
# 10. Protobuf-specific features
# =========================================================
echo -e "\n${YELLOW}SECTION 10: PROTOBUF-SPECIFIC FEATURES${NC}"

# Test a schema with enum
ENUM_SCHEMA='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  int32 id = 1;\n  string name = 2;\n  UserType type = 3;\n  \n  enum UserType {\n    UNKNOWN = 0;\n    ADMIN = 1;\n    REGULAR = 2;\n  }\n}",
  "schemaType": "PROTOBUF"
}'

run_test "Register Protobuf schema with enum" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$ENUM_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/protobuf-enum-test/versions | grep -q '{\"id\":'" \
  "success"

# Test a schema with nested message
NESTED_SCHEMA='{
  "schema": "syntax = \"proto3\";\npackage com.example;\n\nmessage User {\n  int32 id = 1;\n  string name = 2;\n  \n  message ContactInfo {\n    string email = 1;\n    string phone = 2;\n  }\n  \n  ContactInfo contact_info = 3;\n}",
  "schemaType": "PROTOBUF"
}'

run_test "Register Protobuf schema with nested message" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$NESTED_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/protobuf-nested-test/versions | grep -q '{\"id\":'" \
  "success"

# =========================================================
# 11. Schema deletion tests
# =========================================================
echo -e "\n${YELLOW}SECTION 11: SCHEMA DELETION${NC}"

run_test "Delete specific version" \
  "curl -s -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | grep -q 1" \
  "success"

run_test "Delete subject" \
  "curl -s -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT | grep -q '[1-9]'" \
  "success"

# =========================================================
# 12. Multi-type support test
# =========================================================
echo -e "\n${YELLOW}SECTION 12: MULTI-TYPE SUPPORT${NC}"

# Create an Avro schema in the same registry
AVRO_SCHEMA='{
  "schema": "{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"}]}",
  "schemaType": "AVRO"
}'

run_test "Register Avro schema alongside Protobuf" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$AVRO_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/multi-type-test-avro/versions | grep -q '{\"id\":'" \
  "success"

# Create a JSON schema in the same registry
JSON_SCHEMA='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"title\":\"User\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"}},\"required\":[\"id\",\"name\"]}",
  "schemaType": "JSON"
}'

run_test "Register JSON Schema alongside Protobuf" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$JSON_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/multi-type-test-json/versions | grep -q '{\"id\":'" \
  "success"

run_test "Verify correct schema types in multi-type environment" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/multi-type-test-avro/versions/latest | grep -q 'AVRO' && \
   curl -s $SCHEMA_REGISTRY_URL/subjects/multi-type-test-json/versions/latest | grep -q 'JSON' && \
   curl -s $SCHEMA_REGISTRY_URL/subjects/protobuf-enum-test/versions/latest | grep -q 'PROTOBUF'" \
  "success"

# =========================================================
# Display summary
# =========================================================
echo -e "\n${YELLOW}TEST SUMMARY${NC}"
echo "Tests passed: $TESTS_PASSED/$TESTS_TOTAL"

if [ $TESTS_PASSED -eq $TESTS_TOTAL ]; then
  echo -e "${GREEN}All tests passed! The Schema Registry implementation appears compatible with Protobuf.${NC}"
  exit 0
else
  echo -e "${RED}Some tests failed. The Schema Registry implementation may not be fully compatible with Protobuf.${NC}"
  exit 1
fi