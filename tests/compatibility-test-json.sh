#!/bin/bash
# Comprehensive Schema Registry Compatibility Test Script for JSON Schema
# This script tests a Schema Registry server implementation for API compatibility
# using JSON Schema format and curl commands.

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

echo "=== Schema Registry JSON Schema Compatibility Test ==="
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
# 3. Schema registration tests with JSON Schema
# =========================================================
echo -e "\n${YELLOW}SECTION 3: JSON SCHEMA REGISTRATION${NC}"

# Test subject
TEST_SUBJECT="test-json-schema-subject"

# Define JSON Schema schemas
# Note the use of schemaType:"JSON" and the proper JSON Schema format
SCHEMA_V1='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"title\":\"User\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"}},\"required\":[\"id\",\"name\"]}",
  "schemaType": "JSON"
}'

SCHEMA_V2='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"title\":\"User\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"},\"email\":{\"type\":\"string\",\"format\":\"email\"}},\"required\":[\"id\",\"name\"]}",
  "schemaType": "JSON"
}'

SCHEMA_INCOMPATIBLE='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"title\":\"User\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"}},\"required\":[\"id\"]}",
  "schemaType": "JSON"
}'

# Register initial schema (v1)
run_test "Register initial JSON Schema (v1)" \
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

run_test "Get JSON Schema by version" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | grep -q 'schemaType'" \
  "success"

run_test "Get latest JSON Schema" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/latest | grep -q 'schema'" \
  "success"

# Get the ID of the registered schema for later use
SCHEMA_ID=$(curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | sed -E 's/.*"id":([0-9]+).*/\1/')
echo "Retrieved schema ID: $SCHEMA_ID"

run_test "Get JSON Schema by ID" \
  "curl -s $SCHEMA_REGISTRY_URL/schemas/ids/$SCHEMA_ID | grep -q 'schema'" \
  "success"

# =========================================================
# 6. Schema lookup tests
# =========================================================
echo -e "\n${YELLOW}SECTION 6: SCHEMA LOOKUP${NC}"

run_test "Lookup JSON Schema under subject" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V1' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT | grep -q 'subject'" \
  "success"

# =========================================================
# 7. Compatibility tests
# =========================================================
echo -e "\n${YELLOW}SECTION 7: JSON SCHEMA COMPATIBILITY TESTS${NC}"

run_test "Test compatible JSON Schema (explicit check)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'true'" \
  "success"

run_test "Test incompatible JSON Schema (explicit check)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_INCOMPATIBLE' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'false'" \
  "success"

run_test "Register compatible JSON Schema (v2)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q '{\"id\":'" \
  "success"

run_test "Verify incompatible JSON Schema is rejected" \
  "! curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_INCOMPATIBLE' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q '{\"id\":'" \
  "success"

# =========================================================
# 8. Subject compatibility configuration
# =========================================================
echo -e "\n${YELLOW}SECTION 8: SUBJECT-LEVEL COMPATIBILITY${NC}"

run_test "Set subject-specific compatibility to FORWARD" \
  "curl -s -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '{\"compatibility\": \"FORWARD\"}' \
   $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT" \
  "success"

run_test "Get subject-specific compatibility" \
  "curl -s $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT | grep -q FORWARD" \
  "success"

# =========================================================
# 9. Schema normalization tests (JSON Schema specific)
# =========================================================
echo -e "\n${YELLOW}SECTION 9: JSON SCHEMA NORMALIZATION${NC}"

# Define two schemas that are semantically equivalent but formatted differently
SCHEMA_NORMAL='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"title\":\"User\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"}},\"required\":[\"id\",\"name\"]}",
  "schemaType": "JSON"
}'

SCHEMA_REORDERED='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"type\":\"object\",\"title\":\"User\",\"required\":[\"name\",\"id\"],\"properties\":{\"name\":{\"type\":\"string\"},\"id\":{\"type\":\"integer\"}}}",
  "schemaType": "JSON"
}'

# Test normalization
run_test "Test JSON Schema normalization" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_REORDERED' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT?normalize=true | grep -q 'subject'" \
  "success"

# =========================================================
# 10. JSON Schema references (with $ref)
# =========================================================
echo -e "\n${YELLOW}SECTION 10: JSON SCHEMA REFERENCES${NC}"

# Define address schema
ADDRESS_SCHEMA='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"$id\":\"https://example.com/address.schema.json\",\"title\":\"Address\",\"type\":\"object\",\"properties\":{\"street\":{\"type\":\"string\"},\"city\":{\"type\":\"string\"},\"zipCode\":{\"type\":\"string\"}},\"required\":[\"street\",\"city\",\"zipCode\"]}",
  "schemaType": "JSON"
}'

# Define user schema with reference to address schema
USER_WITH_REF_SCHEMA='{
  "schema": "{\"$schema\":\"http://json-schema.org/draft-07/schema#\",\"$id\":\"https://example.com/user-address.schema.json\",\"title\":\"UserWithAddress\",\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"},\"address\":{\"$ref\":\"https://example.com/address.schema.json\"}},\"required\":[\"id\",\"name\",\"address\"]}",
  "schemaType": "JSON",
  "references": [
    {
      "name": "https://example.com/address.schema.json",
      "subject": "address-json-schema",
      "version": 1
    }
  ]
}'

# Register address schema
run_test "Register Address JSON Schema for reference" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$ADDRESS_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/address-json-schema/versions | grep -q '{\"id\":'" \
  "success"

# Register user schema with reference
run_test "Register JSON Schema with reference" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$USER_WITH_REF_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/user-with-address-json-schema/versions | grep -q '{\"id\":'" \
  "success"

# Get schema with reference
run_test "Get JSON Schema with reference" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/user-with-address-json-schema/versions/latest | grep -q 'references'" \
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

run_test "Register Avro schema alongside JSON Schema" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$AVRO_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/avro-type-test/versions | grep -q '{\"id\":'" \
  "success"

run_test "Verify schema type in retrieval" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/avro-type-test/versions/latest | grep -q 'AVRO'" \
  "success"

# =========================================================
# Display summary
# =========================================================
echo -e "\n${YELLOW}TEST SUMMARY${NC}"
echo "Tests passed: $TESTS_PASSED/$TESTS_TOTAL"

if [ $TESTS_PASSED -eq $TESTS_TOTAL ]; then
  echo -e "${GREEN}All tests passed! The Schema Registry implementation appears compatible with JSON Schema.${NC}"
  exit 0
else
  echo -e "${RED}Some tests failed. The Schema Registry implementation may not be fully compatible with JSON Schema.${NC}"
  exit 1
fi