#!/bin/bash
# Comprehensive Schema Registry Compatibility Test Script
# This script tests a Schema Registry server implementation for API compatibility
# using Avro schemas and curl commands.

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

echo "=== Schema Registry Compatibility Test ==="
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
# 3. Schema registration tests
# =========================================================
echo -e "\n${YELLOW}SECTION 3: SCHEMA REGISTRATION${NC}"

# Test subject
TEST_SUBJECT="test-compatibility-subject"

# Define simple Avro schemas
SCHEMA_V1='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"}]}"}'
SCHEMA_V2='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'
SCHEMA_INCOMPATIBLE='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"}]}"}'

# Register initial schema (v1)
run_test "Register initial schema (v1)" \
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

run_test "Get schema by version" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | grep -q 'schema'" \
  "success"

run_test "Get latest schema" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/latest | grep -q 'schema'" \
  "success"

# Get the ID of the registered schema for later use
SCHEMA_ID=$(curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | sed -E 's/.*"id":([0-9]+).*/\1/')
echo "Retrieved schema ID: $SCHEMA_ID"

run_test "Get schema by ID" \
  "curl -s $SCHEMA_REGISTRY_URL/schemas/ids/$SCHEMA_ID | grep -q 'schema'" \
  "success"

# =========================================================
# 6. Schema lookup tests
# =========================================================
echo -e "\n${YELLOW}SECTION 6: SCHEMA LOOKUP${NC}"

run_test "Lookup schema under subject" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V1' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT | grep -q 'subject'" \
  "success"

# =========================================================
# 7. Compatibility tests
# =========================================================
echo -e "\n${YELLOW}SECTION 7: COMPATIBILITY TESTS${NC}"

run_test "Test compatible schema (explicit check)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'true'" \
  "success"

run_test "Test incompatible schema (explicit check)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_INCOMPATIBLE' \
   $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest | grep -q 'false'" \
  "success"

run_test "Register compatible schema (v2)" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$SCHEMA_V2' \
   $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions | grep -q '{\"id\":'" \
  "success"

run_test "Verify incompatible schema is rejected" \
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
# 9. Schema deletion tests
# =========================================================
echo -e "\n${YELLOW}SECTION 9: SCHEMA DELETION${NC}"

run_test "Delete specific version" \
  "curl -s -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/1 | grep -q 1" \
  "success"

run_test "Delete subject" \
  "curl -s -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT | grep -q '[1-9]'" \
  "success"

# =========================================================
# 10. Schema references tests (advanced)
# =========================================================
echo -e "\n${YELLOW}SECTION 10: SCHEMA REFERENCES${NC}"

# Define schemas with references
ADDRESS_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"Address\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"street\",\"type\":\"string\"},{\"name\":\"city\",\"type\":\"string\"},{\"name\":\"zipCode\",\"type\":\"string\"}]}"}'

USER_WITH_REF_SCHEMA='{
  "schema": "{\"type\":\"record\",\"name\":\"UserWithAddress\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"address\",\"type\":\"com.example.Address\"}]}",
  "references": [
    {
      "name": "com.example.Address",
      "subject": "com.example.Address",
      "version": 1
    }
  ]
}'

# Register schemas with references
run_test "Register Address schema for reference" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$ADDRESS_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/com.example.Address/versions | grep -q '{\"id\":'" \
  "success"

run_test "Register schema with reference" \
  "curl -s -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
   --data '$USER_WITH_REF_SCHEMA' \
   $SCHEMA_REGISTRY_URL/subjects/com.example.UserWithAddress/versions | grep -q '{\"id\":'" \
  "success"

run_test "Get schema with reference" \
  "curl -s $SCHEMA_REGISTRY_URL/subjects/com.example.UserWithAddress/versions/latest | grep -q 'references'" \
  "success"

# =========================================================
# 11. Alternative content types (advanced)
# =========================================================
echo -e "\n${YELLOW}SECTION 11: CONTENT TYPE HANDLING${NC}"

run_test "Register schema with alternative content type" \
  "curl -s -X POST -H \"Content-Type: application/json\" \
   --data '$SCHEMA_V1' \
   $SCHEMA_REGISTRY_URL/subjects/alt-content-type/versions | grep -q '{\"id\":'" \
  "success"

run_test "Accept alternative content types" \
  "curl -s -H \"Accept: application/json\" \
   $SCHEMA_REGISTRY_URL/subjects | grep -q 'alt-content-type'" \
  "success"

# =========================================================
# 12. Schema types (Avro, JSON Schema, Protobuf) - Basic check
# =========================================================
echo -e "\n${YELLOW}SECTION 12: SCHEMA TYPE SUPPORT${NC}"

run_test "Check supported schema types" \
  "curl -s $SCHEMA_REGISTRY_URL/schemas/types | grep -q 'AVRO'" \
  "success"

# =========================================================
# Display summary
# =========================================================
echo -e "\n${YELLOW}TEST SUMMARY${NC}"
echo "Tests passed: $TESTS_PASSED/$TESTS_TOTAL"

if [ $TESTS_PASSED -eq $TESTS_TOTAL ]; then
  echo -e "${GREEN}All tests passed! The Schema Registry implementation appears compatible.${NC}"
  exit 0
else
  echo -e "${RED}Some tests failed. The Schema Registry implementation may not be fully compatible.${NC}"
  exit 1
fi