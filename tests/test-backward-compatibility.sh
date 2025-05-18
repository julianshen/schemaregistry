#!/bin/bash
# Schema Registry Backward Compatibility Test Script for macOS with command echoing

# Schema Registry URL - adjust as needed
SCHEMA_REGISTRY_URL="http://localhost:8081"

# Function to execute a command and echo it first
run_command() {
  echo -e "\n> $@"
  eval "$@"
}

# Exit on error
set -e

echo "=== Schema Registry Backward Compatibility Test ==="
echo

# 1. Check Schema Registry is running
echo "Checking Schema Registry availability..."
run_command "curl -s $SCHEMA_REGISTRY_URL > /dev/null || { echo \"Schema Registry not available at $SCHEMA_REGISTRY_URL\"; exit 1; }"
echo "Schema Registry is available!"
echo

# 2. Set global compatibility mode to BACKWARD
echo "Setting global compatibility mode to BACKWARD..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"BACKWARD\"}' \
  $SCHEMA_REGISTRY_URL/config"
echo -e "\nGlobal compatibility set to BACKWARD"
echo

# 3. Create test subject
TEST_SUBJECT="user-test-value"
echo "Using test subject: $TEST_SUBJECT"
echo

# 4. Register initial schema (v1) with a field that will be modified later
echo "Registering initial schema (v1)..."
V1_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":\"string\"}]}"}'
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V1_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"

echo -e "\nInitial schema registered!"
echo

# 5. Try to register an incompatible schema (removing field without default)
echo "Trying to register incompatible schema (removing 'email' field without default)..."
V2_INCOMPATIBLE_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"}]}"}'

run_command "INCOMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_INCOMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$INCOMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test PASSED: Incompatible schema correctly rejected\"
else
  echo \"Test FAILED: Incompatible schema was accepted\"
  echo \"Response: \$INCOMPATIBLE_RESPONSE\"
fi"
echo

# 6. Check compatibility explicitly
echo "Checking compatibility explicitly..."
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_INCOMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo
echo

# 7. Register a compatible schema (making field optional with default)
echo "Registering compatible schema (making 'email' field optional with default)..."
V2_COMPATIBLE_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "COMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_COMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$COMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Compatible schema correctly accepted\"
else
  echo \"Test FAILED: Compatible schema was rejected\"
  echo \"Response: \$COMPATIBLE_RESPONSE\"
fi"
echo

# 8. Add a new field with default (backward compatible)
echo "Registering schema with new field (with default)..."
V3_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "NEW_FIELD_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V3_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$NEW_FIELD_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Schema with new optional field correctly accepted\"
else
  echo \"Test FAILED: Schema with new optional field was rejected\"
  echo \"Response: \$NEW_FIELD_RESPONSE\"
fi"
echo

# 9. Check versions registered
echo "Checking all versions registered for $TEST_SUBJECT:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 10. Set subject-specific compatibility
echo "Setting subject-specific compatibility to BACKWARD_TRANSITIVE..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"BACKWARD_TRANSITIVE\"}' \
  $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT"
echo -e "\nSubject compatibility set to BACKWARD_TRANSITIVE"
echo

# 11. Check compatibility with all previous versions (BACKWARD_TRANSITIVE)
echo "Checking compatibility with all previous versions..."
V4_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"phone\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions"
echo

# 12. Register the new schema with phone field
echo "Registering schema with phone field (BACKWARD_TRANSITIVE compatible)..."
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo

# 13. Show all versions again
echo "Checking final versions registered for $TEST_SUBJECT:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 14. Clean up (optional) - delete the test subject
echo "Cleaning up - deleting test subject..."
echo "> # curl -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT"
echo "NOTE: Uncomment the line above to actually delete the subject"
echo

echo "=== Test Completed ==="